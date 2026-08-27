package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListMatchDecisions returns paginated decision memories across the platform.
func (r *Repository) ListMatchDecisions(ctx context.Context, search string, limit, offset int) ([]*catalog.MatchDecisionView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []*catalog.MatchDecisionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"1=1"}
		var args []any

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.norm_name ILIKE "+param+" OR m.reason ILIKE "+param+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+param+" OR COALESCE(p.sku, '') ILIKE "+param+")")
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

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.decision_key, m.norm_name, m.chosen_product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.confidence, COALESCE(m.reason, ''), m.prompt_version, m.hit_count,
			       m.created_at, m.last_used_at
			FROM catalog.match_decisions m
			LEFT JOIN catalog.products p ON p.id = m.chosen_product_id
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
			var chosenID *int64
			var reason string
			if err := rows.Scan(
				&v.ID, &v.DecisionKey, &v.NormName, &chosenID,
				&v.ChosenProductName, &v.ChosenProductSKU,
				&v.Confidence, &reason, &v.PromptVersion, &v.HitCount,
				&v.CreatedAt, &v.LastUsedAt,
			); err != nil {
				return err
			}
			v.ChosenProductID = chosenID
			v.Reason = reason
			out = append(out, &v)
		}
		return rows.Err()
	})

	return out, total, err
}

// DeleteMatchDecision removes a single match decision from the system cache.
func (r *Repository) DeleteMatchDecision(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM catalog.match_decisions WHERE id = $1;`, id)
		return err
	})
}

// ClearMatchDecisions purges all cached matching decisions from the system.
func (r *Repository) ClearMatchDecisions(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `TRUNCATE TABLE catalog.match_decisions RESTART IDENTITY;`)
		return err
	})
}

// ListCustomerMappings returns the saved learned matching decisions for a customer/vendor organization.
func (r *Repository) ListCustomerMappings(ctx context.Context, orgID int64, search string, limit, offset int) ([]*catalog.CustomerMappingView, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []*catalog.CustomerMappingView
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"(m.organization_id = $1 OR m.customer_org_id = $1)"}
		args := []any{orgID}

		if search = strings.TrimSpace(search); search != "" {
			args = append(args, "%"+search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(m.raw_name ILIKE "+param+" OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE "+param+" OR COALESCE(p.sku, '') ILIKE "+param+")")
		}

		clause := strings.Join(where, " AND ")

		countQuery := `
			SELECT count(*)
			FROM catalog.customer_product_mappings m
			LEFT JOIN catalog.products p ON p.id = m.product_id
			WHERE ` + clause + `;
		`
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		query := `
			SELECT m.id, m.organization_id, m.raw_name, m.product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       m.source, m.status, m.created_at, m.updated_at
			FROM catalog.customer_product_mappings m
			LEFT JOIN catalog.products p ON p.id = m.product_id
			WHERE ` + clause + `
			ORDER BY m.updated_at DESC, m.id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`
		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.CustomerMappingView
			if err := rows.Scan(
				&v.ID, &v.OrganizationID, &v.RawName, &v.ProductID,
				&v.ProductName, &v.ProductSKU,
				&v.Source, &v.Status, &v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			out = append(out, &v)
		}
		return rows.Err()
	})

	return out, total, err
}

// DeleteCustomerMapping removes a saved product mapping for an organization.
func (r *Repository) DeleteCustomerMapping(ctx context.Context, orgID, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			DELETE FROM catalog.customer_product_mappings
			WHERE id = $1 AND (organization_id = $2 OR customer_org_id = $2);
		`, id, orgID)
		return err
	})
}

// ClearCustomerMappings purges all saved product mappings for an organization.
func (r *Repository) ClearCustomerMappings(ctx context.Context, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			DELETE FROM catalog.customer_product_mappings
			WHERE organization_id = $1 OR customer_org_id = $1;
		`, orgID)
		return err
	})
}
