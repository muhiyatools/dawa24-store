package postgres

// The two database halves of the AI enhancement stage: writing what it decided,
// and remembering it.
//
// Both are deliberately bulk. The stage's whole cost argument is that a file is
// a fixed handful of model requests however long it is; putting a round trip
// behind each of two thousand accepted matches, or each of two thousand cache
// lookups, would move the bottleneck rather than remove it.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ApplyAIMatches folds every accepted answer onto its staged row in one
// statement.
//
// A row the vendor has already matched by hand is left alone: their decision
// outranks the model's, and an import that silently overwrote it would be
// unusable. The match_level is written as 'strong' so the row falls into the
// matched bucket by the same rule every other matched row does, and the message
// says the model decided it — which is what the review screen shows and what an
// operator reads when a match looks wrong.
func (r *Repository) ApplyAIMatches(
	ctx context.Context, importID int64, matches []ingest.AIMatch,
) error {
	if len(matches) == 0 {
		return nil
	}

	const cols = 4
	values := make([]string, 0, len(matches))
	args := make([]any, 0, len(matches)*cols+1)
	args = append(args, importID)
	for i, m := range matches {
		base := i*cols + 1
		values = append(values, fmt.Sprintf("($%d::int,$%d::bigint,$%d::numeric,$%d::text)",
			base+1, base+2, base+3, base+4))
		args = append(args, m.SourceRow, m.ProductID, clampScore(m.Score), trimTo(m.Reason, 500))
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows AS t
			SET product_id  = v.product_id,
			    match_level = 'strong',
			    match_score = v.score,
			    message     = v.reason
			FROM (VALUES `+strings.Join(values, ",")+`) AS v(source_row, product_id, score, reason)
			WHERE t.import_id = $1
			  AND t.source_row = v.source_row
			  AND NOT t.is_manually_matched;`, args...)
		if err != nil {
			return fmt.Errorf("ingest postgres: apply AI matches: %w", err)
		}
		return nil
	})
}

// LookupDecisions reads the shared decision cache for a batch of keys.
//
// The same table the smart order reads and writes. That is the point: the two
// features ask an identical question through an identical prompt, so an answer
// bought by either is free to the other, and a vendor's weekly re-upload of a
// price list they changed twelve rows of costs twelve rows' worth of model
// time.
//
// AsSystem, because the cache is knowledge about the shared catalogue rather
// than about any one tenant — and cross-tenant reuse is most of its value.
func (r *Repository) LookupDecisions(
	ctx context.Context, keys []string,
) (map[string]ingest.CachedDecision, error) {
	if len(keys) == 0 {
		return map[string]ingest.CachedDecision{}, nil
	}
	out := make(map[string]ingest.CachedDecision, len(keys))

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
			var d ingest.CachedDecision
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

// SaveDecisions writes what the model decided to the shared cache.
//
// ON CONFLICT DO NOTHING rather than an update: the key already contains the
// prompt version and the shortlist, so a collision means the identical question
// was answered concurrently, and either answer is as good as the other.
func (r *Repository) SaveDecisions(ctx context.Context, decisions []ingest.CachedDecision) error {
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

// SaveAlias records a confirmed name for a catalogue product.
//
// Written with source 'ai_confirmed', which the deterministic alias tier
// deliberately excludes. The row exists so an operator can see what the model
// has been deciding and promote what is right — not so the next import trusts
// it. One confident mistake propagating silently to every vendor is exactly
// what that exclusion guards against.
func (r *Repository) SaveAlias(
	ctx context.Context, productID int64, alias, source string, confidence float64,
) error {
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
