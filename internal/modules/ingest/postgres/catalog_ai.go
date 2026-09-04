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
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

	const cols = 5
	values := make([]string, 0, len(matches))
	args := make([]any, 0, len(matches)*cols+1)
	args = append(args, importID)
	for i, m := range matches {
		base := i*cols + 1
		values = append(values, fmt.Sprintf("($%d::int,$%d::bigint,$%d::numeric,$%d::text,$%d::text)",
			base+1, base+2, base+3, base+4, base+5))
		args = append(args, m.SourceRow, m.ProductID, clampScore(m.Score),
			trimTo(m.Reason, 500), matchLevel(m.Level))
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows AS t
			SET product_id  = v.product_id,
			    match_level = v.level,
			    match_score = v.score,
			    message     = v.reason
			FROM (VALUES `+strings.Join(values, ",")+`) AS v(source_row, product_id, score, reason, level)
			WHERE t.import_id = $1
			  AND t.source_row = v.source_row
			  AND NOT t.is_manually_matched;`, args...)
		if err != nil {
			return fmt.Errorf("ingest postgres: apply AI matches: %w", err)
		}
		return nil
	})
}

// LookupDecisions reads the tenant-scoped decision cache for a batch of keys.
func (r *Repository) LookupDecisions(
	ctx context.Context, keys []string,
) (map[string]ingest.CachedDecision, error) {
	if len(keys) == 0 {
		return map[string]ingest.CachedDecision{}, nil
	}
	out := make(map[string]ingest.CachedDecision, len(keys))

	var orgID int64
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		orgID = actor.OrganizationID
	} else if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		orgID = tid
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if !isDecisionMemoryEnabled(txCtx, tx) {
			return nil // Global kill switch is OFF
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

// SaveDecisions writes what the model decided to the tenant-scoped cache.
func (r *Repository) SaveDecisions(ctx context.Context, decisions []ingest.CachedDecision) error {
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

// matchLevel defaults an unset level to an acceptance.
//
// The stage always sets one now, and the default is here so a caller written
// before it did — or a test constructing an AIMatch by hand — writes the level
// the row had rather than an empty string the review screen cannot read.
func matchLevel(level string) string {
	if level == "" {
		return "strong"
	}
	return level
}
