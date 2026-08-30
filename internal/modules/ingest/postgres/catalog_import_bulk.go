package postgres

// Bulk review actions.
//
// The review screen's unit of work is not a row, it is a decision repeated over
// a page of rows: "these forty suggestions are right, that one is not". Doing
// that one form post at a time is forty page loads, and the vendor with nine
// thousand rows does not do it at all — which is how a review queue turns into
// either an unreviewed import or an abandoned one.
//
// So every action here takes a list of row ids and settles them in one
// statement. The list always comes from the server-rendered page the vendor was
// looking at, and every statement is scoped by import_id as well as by id, so a
// forged row id from another import matches nothing.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

// refreshCounts recomputes an import's three match counters.
//
// It is one statement rather than three because the review screen reads all
// three together, and it is here rather than repeated at each call site because
// the three predicates and the tab filters in Rows() must agree — a screen
// whose counters disagree with its own tabs is one nobody trusts.
const refreshCounts = `
	UPDATE ingest.catalog_imports SET
	  matched_rows = (
	    SELECT COUNT(*) FROM ingest.catalog_import_rows
	    WHERE import_id = $1
	      AND (is_manually_matched OR match_level IN ('barcode','code','exact','strong'))
	      AND product_id IS NOT NULL AND product_id > 0),
	  review_rows = (
	    SELECT COUNT(*) FROM ingest.catalog_import_rows
	    WHERE import_id = $1 AND NOT is_manually_matched
	      AND match_level IN ('review','ambiguous')),
	  unmatched_rows = (
	    SELECT COUNT(*) FROM ingest.catalog_import_rows
	    WHERE import_id = $1 AND NOT is_manually_matched
	      AND (product_id IS NULL OR product_id = 0
	           OR match_level IN ('none','unmatched',''))
	      AND match_level NOT IN ('review','ambiguous','barcode','code','exact','strong'))
	WHERE id = $1;`

// ConfirmRowMatches promotes the engine's suggestion on each row to a decision
// the vendor made.
//
// It confirms what is already there rather than choosing anything: the product
// on the row is the one the review screen showed. A row with no product is
// skipped and counted as skipped rather than silently ignored, because "I
// selected forty and thirty-one were confirmed" is the only way the vendor
// learns that nine of them still need a product chosen by hand.
//
// The score is left as the engine measured it and match_level is not touched;
// is_manually_matched is what carries the decision, so the review screen can go
// on showing what the engine thought and what the vendor did about it.
func (r *Repository) ConfirmRowMatches(
	ctx context.Context, importID int64, rowIDs []int64,
) (confirmed int, err error) {
	if len(rowIDs) == 0 {
		return 0, nil
	}
	err = r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET is_manually_matched = true,
			    is_excluded = false,
			    outcome = 'staged',
			    message = 'مطابقة معتمدة من المستخدم بعد المراجعة'
			WHERE import_id = $1
			  AND id = ANY($2::bigint[])
			  AND product_id IS NOT NULL AND product_id > 0
			  AND NOT is_manually_matched;`, importID, rowIDs)
		if err != nil {
			return err
		}
		confirmed = int(tag.RowsAffected())
		_, err = tx.Exec(txCtx, refreshCounts, importID)
		return err
	})
	return confirmed, err
}

// SetRowsExcluded includes or excludes a list of rows in one statement.
func (r *Repository) SetRowsExcluded(
	ctx context.Context, importID int64, rowIDs []int64, excluded bool,
) (int, error) {
	if len(rowIDs) == 0 {
		return 0, nil
	}
	var n int
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET is_excluded = $3
			WHERE import_id = $1 AND id = ANY($2::bigint[]);`,
			importID, rowIDs, excluded)
		if err != nil {
			return err
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

// ClearRowMatches unlinks a list of rows, returning them to "not matched".
//
// The counterpart of ConfirmRowMatches, and the reason it exists is the same:
// a vendor who has just selected a page of suggestions and seen that they are
// wrong needs one action, not forty.
func (r *Repository) ClearRowMatches(
	ctx context.Context, importID int64, rowIDs []int64,
) (int, error) {
	if len(rowIDs) == 0 {
		return 0, nil
	}
	var n int
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET product_id = NULL,
			    match_level = 'none',
			    match_score = 0.0000,
			    is_manually_matched = false,
			    outcome = 'staged',
			    message = 'تم إلغاء الربط بالكتالوج'
			WHERE import_id = $1 AND id = ANY($2::bigint[]);`, importID, rowIDs)
		if err != nil {
			return err
		}
		n = int(tag.RowsAffected())
		_, err = tx.Exec(txCtx, refreshCounts, importID)
		return err
	})
	return n, err
}

// PendingRowIDs lists the rows a commit would refuse, newest decision first.
//
// The commit screen needs the count, and a vendor about to lose ninety rows
// deserves to be told before the button is pressed rather than after. Returning
// ids rather than a bare count lets the same call drive the "select every row
// still awaiting a decision" action on the review screen.
func (r *Repository) PendingRowIDs(ctx context.Context, importID int64, limit int) ([]int64, int, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	var ids []int64
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const pending = `
			FROM ingest.catalog_import_rows
			WHERE import_id = $1 AND NOT is_excluded AND NOT is_manually_matched
			  AND match_level NOT IN ('barcode','code','exact','strong')`
		if err := tx.QueryRow(txCtx, `SELECT count(*) `+pending, importID).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(txCtx,
			`SELECT id `+pending+` ORDER BY match_score DESC, source_row LIMIT $2`,
			importID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, total, err
}

// rowsWhere builds the review table's filter clause.
//
// Extracted because the bulk actions must select exactly the rows the vendor
// was looking at: "apply this to everything matching the current filter" is
// only safe if the filter means the same thing to the page and to the action.
// Two copies of these predicates would eventually mean two different things.
func rowsWhere(importID int64, filter ingest.RowFilter) (string, []any) {
	where := []string{"r.import_id = $1"}
	args := []any{importID}
	if filter.Outcome != "" {
		args = append(args, filter.Outcome)
		where = append(where, fmt.Sprintf("r.outcome = $%d", len(args)))
	}
	switch filter.MatchLevel {
	case "matched":
		where = append(where, "((r.is_manually_matched = true OR r.match_level IN ('barcode', 'code', 'exact', 'strong')) AND r.product_id IS NOT NULL AND r.product_id > 0)")
	case "review":
		where = append(where, "(NOT r.is_manually_matched AND r.match_level IN ('review', 'ambiguous'))")
	case "unmatched":
		where = append(where, "(NOT r.is_manually_matched AND (r.product_id IS NULL OR r.product_id = 0 OR r.match_level IN ('none', 'unmatched', '')) AND r.match_level NOT IN ('review', 'ambiguous', 'barcode', 'code', 'exact', 'strong'))")
	case "":
		// no match filter
	default:
		args = append(args, filter.MatchLevel)
		where = append(where, fmt.Sprintf("r.match_level = $%d", len(args)))
	}
	if q := strings.TrimSpace(filter.Search); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("(r.display_name ILIKE $%d OR r.source_code ILIKE $%d OR r.custom_variant_name ILIKE $%d OR COALESCE(p.name->>'ar', p.name->>'en', '') ILIKE $%d OR p.sku ILIKE $%d)", len(args), len(args), len(args), len(args), len(args)))
	}
	return strings.Join(where, " AND "), args
}

// RowIDsForFilter lists the ids of every row the current filter selects.
//
// It is what makes "select everything matching this filter" mean the same thing
// as the page the vendor is reading, rather than approximately the same thing.
// Capped, because a selection larger than the cap is not a selection — it is an
// import setting, and the wizard has one of those.
func (r *Repository) RowIDsForFilter(
	ctx context.Context, importID int64, filter ingest.RowFilter, limit int,
) ([]int64, error) {
	if limit <= 0 || limit > 20000 {
		limit = 20000
	}
	clause, args := rowsWhere(importID, filter)
	var out []int64
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT r.id
			FROM ingest.catalog_import_rows r
			LEFT JOIN catalog.products p ON p.id = r.product_id
			WHERE `+clause+`
			ORDER BY r.match_score DESC, r.source_row
			LIMIT $`+strconv.Itoa(len(args)+1), append(args, limit)...)
		if err != nil {
			return fmt.Errorf("ingest postgres: list import row ids: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}
