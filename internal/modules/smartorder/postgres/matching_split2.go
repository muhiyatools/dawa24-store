package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ResolveByFuzzyDB uses PostgreSQL's pg_trgm extension to find catalogue
// products whose names are similar to the unresolved lines — catching
// transliteration variants (i18n.TDefault("w4_mod.s_203_203") vs i18n.TDefault("w4_mod.s_204_204")) and typos that share no
// whole word but plenty of character sequences.
//
// This tier runs AFTER exact name matching and BEFORE the in-memory scorer,
// handling the middle ground: names that are not identical but obviously refer
// to the same product when the character overlap is measured.
//
// Only unambiguous, high-similarity matches are accepted (similarity > 0.45).
// The query is batched to avoid a per-line round trip.
func (r *Repository) ResolveByFuzzyDB(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	const batchSize = 500
	out := make(map[string]int64, len(names))
	ambiguous := make(map[string]bool)

	nameExpr := "platform.normalize_arabic(lower(trim(p.name->>'ar')))"
	if matchLang == "en" {
		nameExpr = "lower(trim(p.name->>'en'))"
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(names); start += batchSize {
			end := start + batchSize
			if end > len(names) {
				end = len(names)
			}
			batch := lowerAll(names[start:end])
			if len(batch) == 0 {
				continue
			}

			rows, err := tx.Query(txCtx, `
				SELECT DISTINCT ON (query) query, p.id AS product_id
				FROM unnest($1::text[]) AS query
				JOIN catalog.products p
				  ON p.deleted_at IS NULL AND p.status = 'active'
				 AND similarity(query, `+nameExpr+`) > 0.45
				ORDER BY query, similarity(query, `+nameExpr+`) DESC;`,
				batch)
			if err != nil {
				return err
			}

			for rows.Next() {
				var key string
				var productID int64
				if err := rows.Scan(&key, &productID); err != nil {
					rows.Close()
					return err
				}
				if existing, seen := out[key]; seen && existing != productID {
					ambiguous[key] = true
				} else {
					out[key] = productID
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// ResolveByContains matches lines where the line's name is contained within
// the catalogue name (or vice versa), provided the name is sufficiently
// specific (at least 6 characters) and matches uniquely to one product.
func (r *Repository) ResolveByContains(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	var filtered []string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if len([]rune(n)) >= 6 {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return map[string]int64{}, nil
	}

	const batchSize = 250
	out := make(map[string]int64, len(filtered))
	ambiguous := make(map[string]bool)

	nameExpr := "platform.normalize_arabic(lower(trim(p.name->>'ar')))"
	if matchLang == "en" {
		nameExpr = "lower(trim(p.name->>'en'))"
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(filtered); start += batchSize {
			end := start + batchSize
			if end > len(filtered) {
				end = len(filtered)
			}
			batch := filtered[start:end]

			rows, err := tx.Query(txCtx, `
				SELECT query, p.id AS product_id
				FROM unnest($1::text[]) AS query
				JOIN catalog.products p
				  ON p.deleted_at IS NULL AND p.status = 'active'
				 AND (
					(`+nameExpr+` LIKE '%' || query || '%') OR
					(query LIKE '%' || `+nameExpr+` || '%' AND length(`+nameExpr+`) >= 6)
				 );`,
				batch)
			if err != nil {
				return err
			}

			for rows.Next() {
				var key string
				var productID int64
				if err := rows.Scan(&key, &productID); err != nil {
					rows.Close()
					return err
				}
				if existing, seen := out[key]; seen && existing != productID {
					ambiguous[key] = true
				} else {
					out[key] = productID
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// SaveLearnedMapping records a buyer correction (FR-016, FR-040) into customer_product_mappings AND match_decisions.
func (r *Repository) SaveLearnedMapping(ctx context.Context, orgID int64, rawName string, productID int64) error {
	normName := strings.ToLower(strings.TrimSpace(rawName))
	decKey := "manual:" + normName
	var userID *int64
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		userID = &actor.UserID
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil
		}
		// 1. Insert into catalog.customer_product_mappings
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, raw_name, product_id, source, status, is_active, created_at, updated_at
			) VALUES ($1, $1, $2, $3, 'manual', 'processed', true, now(), now())
			ON CONFLICT DO NOTHING;`, orgID, rawName, productID)
		if err != nil {
			return err
		}

		// 2. Also record in catalog.match_decisions
		if normName != "" && productID > 0 {
			_, err = tx.Exec(txCtx, `
				INSERT INTO catalog.match_decisions (
					organization_id, user_id, decision_key, norm_name, chosen_product_id,
					confidence, reason, prompt_version, hit_count, created_at, last_used_at
				) VALUES (
					$1, $2, $3, $4, $5,
					1.000, 'قرار تصحيح يدوي من الطلب الذكي', 'manual:v1', 1, now(), now()
				)
				ON CONFLICT (COALESCE(organization_id, 0), decision_key)
				DO UPDATE SET
					chosen_product_id = EXCLUDED.chosen_product_id,
					confidence = 1.000,
					user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
					hit_count = catalog.match_decisions.hit_count + 1,
					last_used_at = now();
			`, orgID, userID, decKey, normName, productID)
		}
		return err
	})
}

// LookupDecisions reads the adjudication cache for a batch of keys scoped strictly to the tenant organization.
func (r *Repository) LookupDecisions(ctx context.Context, keys []string) (map[string]smartorder.CachedDecision, error) {
	if len(keys) == 0 {
		return map[string]smartorder.CachedDecision{}, nil
	}
	out := make(map[string]smartorder.CachedDecision, len(keys))

	var orgID int64
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		orgID = actor.OrganizationID
	} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		orgID = tid
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil // Global kill-switch is OFF
		}

		var rows pgx.Rows
		var err error
		if orgID > 0 {
			rows, err = tx.Query(txCtx, `
				UPDATE catalog.match_decisions
				SET hit_count = hit_count + 1, last_used_at = now()
				WHERE organization_id = $1 AND decision_key = ANY($2::text[])
				RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version;`,
				orgID, keys)
		} else {
			rows, err = tx.Query(txCtx, `
				UPDATE catalog.match_decisions
				SET hit_count = hit_count + 1, last_used_at = now()
				WHERE organization_id IS NULL AND decision_key = ANY($1::text[])
				RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version;`,
				keys)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d smartorder.CachedDecision
			if err := rows.Scan(&d.Key, &d.NormName, &d.ChosenProductID,
				&d.Confidence, &d.Reason, &d.PromptVersion); err != nil {
				return err
			}
			out[d.Key] = d
		}
		return rows.Err()
	})
	return out, err
}

// SaveDecisions writes adjudication results to the tenant-scoped cache.
func (r *Repository) SaveDecisions(ctx context.Context, decisions []smartorder.CachedDecision) error {
	if len(decisions) == 0 {
		return nil
	}
	var orgID int64
	var userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrganizationID
		userID = actor.UserID
	} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		orgID = tid
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil // Global switch is OFF
		}

		const cols = 8
		values := make([]string, 0, len(decisions))
		args := make([]any, 0, len(decisions)*cols)
		for i, d := range decisions {
			base := i * cols
			ph := make([]string, cols)
			for j := 0; j < cols; j++ {
				ph[j] = "$" + strconv.Itoa(base+j+1)
			}
			values = append(values, "("+strings.Join(ph, ",")+")")
			args = append(args,
				sqlNullOrgID(orgID), sqlNullOrgID(userID),
				d.Key, d.NormName, d.ChosenProductID,
				d.Confidence, d.Reason, d.PromptVersion)
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.match_decisions (
				organization_id, user_id, decision_key, norm_name, chosen_product_id,
				confidence, reason, prompt_version
			) VALUES `+strings.Join(values, ",")+`
			ON CONFLICT (COALESCE(organization_id, 0), decision_key)
			DO UPDATE SET
				chosen_product_id = EXCLUDED.chosen_product_id,
				confidence = EXCLUDED.confidence,
				reason = EXCLUDED.reason,
				user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
				hit_count = catalog.match_decisions.hit_count + 1,
				last_used_at = now();`, args...)
		return err
	})
}

func sqlNullOrgID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func isDecisionMemoryEnabled(ctx context.Context, tx pgx.Tx) bool {
	var val any
	err := tx.QueryRow(ctx, `SELECT value FROM platform_admin.system_settings WHERE key = 'decision_memory_enabled' LIMIT 1;`).Scan(&val)
	if err != nil || val == nil {
		return true // default enabled
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	case []byte:
		s := string(v)
		return strings.Contains(s, "true") || strings.Contains(s, "1")
	default:
		return true
	}
}
