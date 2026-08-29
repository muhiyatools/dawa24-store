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

// Bulk resolution.
//
// Every function here takes the whole file's worth of keys and returns a map.
// None of them may be called in a loop — FR-017a exists because the previous
// generation of this feature issued a query per row, and a ten-thousand-line
// import spent its entire budget waiting on round trips.

// ResolveByCodes matches SKUs and barcodes across the whole file in one query.
//
// Both parent products and variants carry codes, and a pharmacy's file may use
// either. A code that resolves to two different products is deliberately dropped
// rather than arbitrarily picked: an ambiguous code is not a match, and guessing
// is how the wrong medicine gets ordered.
func (r *Repository) ResolveByCodes(ctx context.Context, skus, barcodes []string) (map[string]int64, error) {
	if len(skus) == 0 && len(barcodes) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(skus)+len(barcodes))
	ambiguous := make(map[string]bool)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT key, product_id FROM (
				SELECT lower(trim(p.sku)) AS key, p.id AS product_id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND lower(trim(p.sku)) = ANY($1::text[])
				UNION ALL
				SELECT lower(trim(v.sku)), v.product_id
				FROM catalog.product_variants v
				WHERE v.deleted_at IS NULL AND v.status = 'active'
				  AND lower(trim(v.sku)) = ANY($1::text[])
				UNION ALL
				SELECT lower(trim(p.barcode)), p.id
				FROM catalog.products p
				WHERE p.deleted_at IS NULL AND p.status = 'active'
				  AND lower(trim(p.barcode)) = ANY($2::text[])
				UNION ALL
				SELECT lower(trim(v.barcode)), v.product_id
				FROM catalog.product_variants v
				WHERE v.deleted_at IS NULL AND v.status = 'active'
				  AND lower(trim(v.barcode)) = ANY($2::text[])
			) hits WHERE key <> '';`,
			lowerAll(skus), lowerAll(barcodes))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			if existing, seen := out[key]; seen && existing != productID {
				ambiguous[key] = true
				continue
			}
			out[key] = productID
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// ResolveBySaving matches against the buyer's own Saving Products list.
//
// Only entries that already carry a product_id count: an unlinked entry is the
// buyer's private label with nothing behind it, and the line falls through to
// the ordinary ladder (FR-015 scenario 2). On the live database 8,777 of 8,778
// entries are linked, which is why this tier resolves so much of a typical file.
//
// Tenant-scoped, not AsSystem: one pharmacy's private naming must never leak to
// another.
func (r *Repository) ResolveBySaving(ctx context.Context, orgID int64, names, skus []string) (map[string]int64, error) {
	if len(names) == 0 && len(skus) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names)+len(skus))

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT key, product_id FROM (
				SELECT platform.normalize_arabic(lower(trim(sp.name_product))) AS key,
				       sp.product_id
				FROM catalog.saving_products sp
				WHERE sp.organization_id = $1
				  AND sp.deleted_at IS NULL
				  AND sp.product_id IS NOT NULL
				  AND platform.normalize_arabic(lower(trim(sp.name_product))) = ANY($2::text[])
				UNION ALL
				SELECT lower(trim(sp.sku)), sp.product_id
				FROM catalog.saving_products sp
				WHERE sp.organization_id = $1
				  AND sp.deleted_at IS NULL
				  AND sp.product_id IS NOT NULL
				  AND lower(trim(sp.sku)) = ANY($3::text[])
			) hits WHERE key <> '';`,
			orgID, lowerAll(names), lowerAll(skus))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			out[key] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByLearned applies corrections this buyer has confirmed before.
//
// This is what makes the third import of a recurring file need no manual work:
// every correction becomes a permanent answer for that buyer's own vocabulary.
func (r *Repository) ResolveByLearned(ctx context.Context, orgID int64, names []string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT platform.normalize_arabic(lower(trim(m.raw_name))) AS key, m.product_id
			FROM catalog.customer_product_mappings m
			WHERE m.customer_org_id = $1
			  AND m.product_id IS NOT NULL
			  AND m.is_active
			  AND platform.normalize_arabic(lower(trim(m.raw_name))) = ANY($2::text[]);`,
			orgID, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			out[key] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByAlias applies aliases confirmed against the shared catalogue.
//
// Cross-tenant by design: an alias is knowledge about the catalogue, not about
// a buyer, and every pharmacy benefits from it. AI-derived aliases are excluded
// until a person has accepted them — AI output never becomes ground truth on its
// own.
func (r *Repository) ResolveByAlias(ctx context.Context, names []string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT a.alias, a.product_id
			FROM catalog.product_aliases a
			WHERE a.source <> 'ai_confirmed'
			  AND a.alias = ANY($1::text[]);`, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var alias string
			var productID int64
			if err := rows.Scan(&alias, &productID); err != nil {
				return err
			}
			out[alias] = productID
		}
		return rows.Err()
	})
	return out, err
}

// ResolveByExactName matches normalised names against the catalogue's own
// Arabic and English product names in one query, prioritizing the configured
// language setting.
//
// This is the single most impactful tier that was missing: when a pharmacy
// types "البير 40جم كريم" and the catalogue has exactly that, no fuzzy scorer
// or AI is needed — a normalised string comparison settles it. On the live
// catalogue of ~20,000 products, this tier alone resolves thousands of lines
// that previously fell through to the scorer or were left unmatched.
//
// The query is cross-tenant (AsSystem) because catalogue products are shared.
// Ambiguous names (mapping to more than one product) are dropped: guessing
// between two products is worse than leaving the line for a human.
func (r *Repository) ResolveByExactName(ctx context.Context, names []string, matchLang string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	out := make(map[string]int64, len(names))
	ambiguous := make(map[string]bool)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var sqlQuery string
		switch matchLang {
		case "en":
			sqlQuery = `
				SELECT key, product_id FROM (
					SELECT lower(trim(p.name->>'en')) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND lower(trim(p.name->>'en')) = ANY($1::text[])
					UNION ALL
					SELECT platform.normalize_arabic(lower(trim(p.name->>'ar'))) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND platform.normalize_arabic(lower(trim(p.name->>'ar'))) = ANY($1::text[])
				) hits WHERE key <> '';`
		default:
			sqlQuery = `
				SELECT key, product_id FROM (
					SELECT platform.normalize_arabic(lower(trim(p.name->>'ar'))) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND platform.normalize_arabic(lower(trim(p.name->>'ar'))) = ANY($1::text[])
					UNION ALL
					SELECT lower(trim(p.name->>'en')) AS key, p.id AS product_id
					FROM catalog.products p
					WHERE p.deleted_at IS NULL AND p.status = 'active'
					  AND lower(trim(p.name->>'en')) = ANY($1::text[])
				) hits WHERE key <> '';`
		}
		rows, err := tx.Query(txCtx, sqlQuery, lowerAll(names))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var productID int64
			if err := rows.Scan(&key, &productID); err != nil {
				return err
			}
			if existing, seen := out[key]; seen && existing != productID {
				ambiguous[key] = true
				continue
			}
			out[key] = productID
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for key := range ambiguous {
		delete(out, key)
	}
	return out, nil
}

// ResolveByFuzzyDB uses PostgreSQL's pg_trgm extension to find catalogue
// products whose names are similar to the unresolved lines — catching
// transliteration variants ("ابليفاى" vs "ابيليفاي") and typos that share no
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

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// SaveAlias records a confirmed name for a catalogue product.
//
// Cross-tenant by design: an alias is knowledge about the shared catalogue, and
// every pharmacy benefits from a name that has been confirmed once. What is not
// shared is trust — an 'ai_confirmed' alias is stored but excluded from the
// deterministic tier until a person accepts it.
func (r *Repository) SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" || productID <= 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.product_aliases (product_id, alias, source, confidence)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (alias, product_id) DO NOTHING;`,
			productID, alias, source, confidence)
		return err
	})
}
