package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
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

// SaveLearnedMapping records a buyer correction (FR-016, FR-040).
func (r *Repository) SaveLearnedMapping(ctx context.Context, orgID int64, rawName string, productID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, raw_name, product_id, source, status
			) VALUES ($1, $1, $2, $3, 'manual', 'processed')
			ON CONFLICT DO NOTHING;`, orgID, rawName, productID)
		return err
	})
}

// LookupDecisions reads the adjudication cache for a batch of keys.
func (r *Repository) LookupDecisions(ctx context.Context, keys []string) (map[string]smartorder.CachedDecision, error) {
	if len(keys) == 0 {
		return map[string]smartorder.CachedDecision{}, nil
	}
	out := make(map[string]smartorder.CachedDecision, len(keys))

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			UPDATE catalog.match_decisions
			SET hit_count = hit_count + 1, last_used_at = now()
			WHERE decision_key = ANY($1::text[])
			RETURNING decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version;`,
			keys)
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

// SaveDecisions writes adjudication results to the shared cache.
func (r *Repository) SaveDecisions(ctx context.Context, decisions []smartorder.CachedDecision) error {
	if len(decisions) == 0 {
		return nil
	}
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const cols = 6
		values := make([]string, 0, len(decisions))
		args := make([]any, 0, len(decisions)*cols)
		for i, d := range decisions {
			base := i * cols
			ph := make([]string, cols)
			for j := 0; j < cols; j++ {
				ph[j] = "$" + strconv.Itoa(base+j+1)
			}
			values = append(values, "("+strings.Join(ph, ",")+")")
			args = append(args, d.Key, d.NormName, d.ChosenProductID,
				d.Confidence, d.Reason, d.PromptVersion)
		}
		_, err := tx.Exec(txCtx, `
			INSERT INTO catalog.match_decisions (
				decision_key, norm_name, chosen_product_id, confidence, reason, prompt_version
			) VALUES `+strings.Join(values, ",")+`
			ON CONFLICT (decision_key) DO NOTHING;`, args...)
		return err
	})
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
