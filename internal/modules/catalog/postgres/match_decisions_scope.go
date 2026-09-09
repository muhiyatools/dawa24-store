package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListMatchDecisionsFiltered returns paginated decision memories matching all specified filters.
func (r *Repository) ListMatchDecisionsFiltered(ctx context.Context, f catalog.DecisionMemoryFilter) ([]*catalog.MatchDecisionView, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	var out []*catalog.MatchDecisionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"1=1"}
		var args []any

		if search := strings.TrimSpace(f.Search); search != "" {
			args = append(args, "%"+search+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+p+" OR m.reason ILIKE "+p+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+p+" OR COALESCE(p.sku, '') ILIKE "+p+")")
		}
		if f.OrganizationID != nil && *f.OrganizationID > 0 {
			args = append(args, *f.OrganizationID)
			where = append(where, "m.organization_id = $"+strconv.Itoa(len(args)))
		}
		if f.UserID != nil && *f.UserID > 0 {
			args = append(args, *f.UserID)
			where = append(where, "m.user_id = $"+strconv.Itoa(len(args)))
		}
		if f.Scope != "" {
			args = append(args, f.Scope)
			where = append(where, "m.scope = $"+strconv.Itoa(len(args)))
		}
		if f.Source != "" {
			args = append(args, f.Source)
			where = append(where, "m.source = $"+strconv.Itoa(len(args)))
		}
		if f.MinConfidence != nil {
			args = append(args, *f.MinConfidence)
			where = append(where, "m.confidence >= $"+strconv.Itoa(len(args)))
		}
		if f.MaxConfidence != nil {
			args = append(args, *f.MaxConfidence)
			where = append(where, "m.confidence <= $"+strconv.Itoa(len(args)))
		}
		if f.OnlyUnlinked {
			where = append(where, "m.chosen_product_id IS NULL")
		}
		if f.CreatedFrom != nil {
			args = append(args, *f.CreatedFrom)
			where = append(where, "m.created_at >= $"+strconv.Itoa(len(args)))
		}
		if f.CreatedTo != nil {
			args = append(args, *f.CreatedTo)
			where = append(where, "m.created_at <= $"+strconv.Itoa(len(args)))
		}
		if f.LastUsedFrom != nil {
			args = append(args, *f.LastUsedFrom)
			where = append(where, "m.last_used_at >= $"+strconv.Itoa(len(args)))
		}
		if f.LastUsedTo != nil {
			args = append(args, *f.LastUsedTo)
			where = append(where, "m.last_used_at <= $"+strconv.Itoa(len(args)))
		}
		if f.MinHitCount != nil {
			args = append(args, *f.MinHitCount)
			where = append(where, "m.hit_count >= $"+strconv.Itoa(len(args)))
		}
		if strings.TrimSpace(f.PromptVersion) != "" {
			args = append(args, strings.TrimSpace(f.PromptVersion))
			where = append(where, "m.prompt_version = $"+strconv.Itoa(len(args)))
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, f.Limit, f.Offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.confidence, COALESCE(m.reason, ''), m.prompt_version, m.hit_count,
			       m.scope, m.source, m.promoted_by, m.promoted_at,
			       m.created_at, m.last_used_at,
			       COALESCE(o.name->>'ar', o.name->>'en', ''), COALESCE(u.name->>'ar', u.name->>'en', u.email, '')
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
			LEFT JOIN org.organizations o ON o.id = m.organization_id
			LEFT JOIN identity.users u ON u.id = m.user_id
			WHERE ` + clause + `
			ORDER BY m.last_used_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID, orgID, userID, promBy *int64
			var reason, orgName, userName string
			if err := rows.Scan(
				&v.ID, &orgID, &userID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.Scope, &v.Source, &promBy, &v.PromotedAt,
				&v.CreatedAt, &v.LastUsedAt,
				&orgName, &userName,
			); err != nil {
				return err
			}
			v.OrganizationID = orgID
			v.UserID = userID
			v.ChosenProductID = chosenID
			v.Reason = reason
			v.PromotedBy = promBy
			v.OrganizationName = orgName
			v.UserName = userName
			out = append(out, &v)
		}
		return rows.Err()
	})
	return out, total, err
}

// ListMatchDecisionsForOrgWithPlatform returns decisions visible to an organization (own + platform-inherited).
// On key collision, the organization's own decision overrides the platform row.
func (r *Repository) ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	pref, err := r.GetDecisionMemoryPreference(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	if !pref {
		return r.ListMatchDecisionsForOrg(ctx, orgID, search, limit, offset)
	}

	var out []*catalog.MatchDecisionView
	var total int

	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"(m.organization_id = $1 OR m.scope = 'platform')"}
		args := []any{orgID}

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+p+" OR m.reason ILIKE "+p+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+p+" OR COALESCE(p.sku, '') ILIKE "+p+")")
		}
		clause := strings.Join(where, " AND ")

		countQuery := fmt.Sprintf(`
			WITH filtered AS (
				SELECT m.id,
				       ROW_NUMBER() OVER (
				           PARTITION BY m.decision_key
				           ORDER BY CASE WHEN m.organization_id = $1 THEN 0 ELSE 1 END, m.last_used_at DESC
				       ) as rn
				FROM catalog.match_decisions m
				LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
				WHERE %s
			)
			SELECT count(*) FROM filtered WHERE rn = 1;
		`, clause)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := fmt.Sprintf(`
			WITH ranked AS (
				SELECT m.id, m.organization_id, m.user_id, m.decision_key, m.norm_name, m.chosen_product_id,
				       COALESCE(p.name->>'ar', p.name->>'en', '') AS prod_name,
				       COALESCE(p.sku, '') AS prod_sku,
				       m.confidence, COALESCE(m.reason, '') AS reason, m.prompt_version, m.hit_count,
				       m.scope, m.source, m.promoted_by, m.promoted_at,
				       m.created_at, m.last_used_at,
				       (m.organization_id IS NULL OR m.organization_id <> $1) AS is_inherited,
				       ROW_NUMBER() OVER (
				           PARTITION BY m.decision_key
				           ORDER BY CASE WHEN m.organization_id = $1 THEN 0 ELSE 1 END, m.last_used_at DESC
				       ) as rn
				FROM catalog.match_decisions m
				LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
				WHERE %s
			)
			SELECT id, organization_id, user_id, decision_key, norm_name, chosen_product_id,
			       prod_name, prod_sku, confidence, reason, prompt_version, hit_count,
			       scope, source, promoted_by, promoted_at, created_at, last_used_at, is_inherited
			FROM ranked
			WHERE rn = 1
			ORDER BY last_used_at DESC, id DESC
			LIMIT %s OFFSET %s;
		`, clause, limParam, offParam)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.MatchDecisionView
			var chosenID, dbOrgID, dbUserID, promBy *int64
			var reason string
			if err := rows.Scan(
				&v.ID, &dbOrgID, &dbUserID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.Scope, &v.Source, &promBy, &v.PromotedAt,
				&v.CreatedAt, &v.LastUsedAt, &v.IsPlatformInherited,
			); err != nil {
				return err
			}
			v.OrganizationID = dbOrgID
			v.UserID = dbUserID
			v.ChosenProductID = chosenID
			v.Reason = reason
			v.PromotedBy = promBy
			out = append(out, &v)
		}
		return rows.Err()
	})
	return out, total, err
}

// GetDecisionMemoryPreference returns whether the organization uses platform decision memory. Defaults to true.
func (r *Repository) GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error) {
	if orgID <= 0 {
		return true, nil
	}
	var usePlatform bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT use_platform_memory
			FROM catalog.decision_memory_preferences
			WHERE organization_id = $1;
		`, orgID).Scan(&usePlatform)
	})
	if err == pgx.ErrNoRows {
		return true, nil
	}
	return usePlatform, err
}

// SetDecisionMemoryPreference updates the organization's preference for using platform memory.
func (r *Repository) SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error {
	if orgID <= 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.decision_memory_preferences (
				organization_id, use_platform_memory, updated_by, updated_at
			) VALUES ($1, $2, NULLIF($3, 0), now())
			ON CONFLICT (organization_id) DO UPDATE SET
				use_platform_memory = EXCLUDED.use_platform_memory,
				updated_by = EXCLUDED.updated_by,
				updated_at = now();
		`, orgID, usePlatform, updatedBy)
		return err
	})
}

// PromoteMatchDecision elevates an organization's decision to platform-wide scope.
func (r *Repository) PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'platform', promoted_by = NULLIF($2, 0), promoted_at = now()
			WHERE id = $1;
		`, id, adminUserID)
		return err
	})
}

// DemoteMatchDecision reverts a decision back to organization-only scope.
func (r *Repository) DemoteMatchDecision(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'org', promoted_by = NULL, promoted_at = NULL
			WHERE id = $1;
		`, id)
		return err
	})
}

// BulkPromoteMatchDecisions promotes multiple decisions to platform scope.
func (r *Repository) BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			UPDATE catalog.match_decisions
			SET scope = 'platform', promoted_by = NULLIF($2, 0), promoted_at = now()
			WHERE id = ANY($1::bigint[]);
		`, ids, adminUserID)
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	return affected, err
}

// BulkDeleteMatchDecisions removes multiple decisions from the cache.
func (r *Repository) BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `
			DELETE FROM catalog.match_decisions
			WHERE id = ANY($1::bigint[]);
		`, ids)
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	return affected, err
}

// RelinkMatchDecision updates the chosen product on a decision record as administrator.
func (r *Repository) RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var normName string
		err := tx.QueryRow(txCtx, `
			UPDATE catalog.match_decisions
			SET chosen_product_id = $2,
			    confidence = 1.000,
			    source = 'admin',
			    user_id = COALESCE(NULLIF($3, 0), user_id),
			    last_used_at = now()
			WHERE id = $1
			RETURNING norm_name;
		`, id, productID, adminUserID).Scan(&normName)
		if err != nil {
			return err
		}
		if productID != nil && *productID > 0 && normName != "" {
			if _, err := tx.Exec(txCtx, `
				INSERT INTO catalog.product_aliases (product_id, alias, source, confidence)
				VALUES ($1, $2, 'manual', 1.000)
				ON CONFLICT (alias, product_id) DO NOTHING;
			`, *productID, normName); err != nil {
				return err
			}
		}
		return nil
	})
}

// RelinkMatchDecisionForOrg updates a decision owned by an organization.
func (r *Repository) RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var normName string
		err := tx.QueryRow(txCtx, `
			UPDATE catalog.match_decisions
			SET chosen_product_id = $3,
			    confidence = 1.000,
			    source = 'manual',
			    user_id = COALESCE(NULLIF($4, 0), user_id),
			    last_used_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING norm_name;
		`, id, orgID, productID, userID).Scan(&normName)
		if err != nil {
			return err
		}
		if productID != nil && *productID > 0 && normName != "" {
			_, _ = tx.Exec(txCtx, `
				INSERT INTO catalog.customer_product_mappings (
					organization_id, customer_org_id, raw_name, product_id,
					source, status, is_active, created_at, updated_at
				) VALUES (
					$1, $1, $2, $3,
					'manual', 'processed', true, now(), now()
				)
				ON CONFLICT DO NOTHING;
			`, orgID, normName, *productID)
		}
		return nil
	})
}
