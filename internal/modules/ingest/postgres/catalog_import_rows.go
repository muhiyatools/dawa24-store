package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The per-row outcome ledger, and the encoding both halves of the import share.

// AppendRows records a batch of row outcomes.
//
// CopyFrom rather than an insert per row: the ledger is written once per
// spreadsheet line and a nine-thousand-row file would otherwise spend longer
// recording what it did than doing it.
func (r *Repository) AppendRows(
	ctx context.Context, importID, orgID int64, rows []ingest.RowOutcome,
) error {
	if len(rows) == 0 {
		return nil
	}
	records := make([][]any, 0, len(rows))
	for _, row := range rows {
		payload, err := json.Marshal(row.Payload)
		if err != nil {
			payload = []byte(`{}`)
		}
		candidates, err := json.Marshal(row.Candidates)
		if err != nil || row.Candidates == nil {
			candidates = []byte(`[]`)
		}
		issues, err := json.Marshal(row.Issues)
		if err != nil || row.Issues == nil {
			issues = []byte(`[]`)
		}
		records = append(records, []any{
			importID, orgID, row.SourceRow, row.Outcome, row.MatchLevel,
			row.MatchScore, row.ProductID, row.VariantID,
			trimTo(row.DisplayName, 300), trimTo(row.SourceCode, 100),
			payload, candidates, issues, trimTo(row.Message, 500),
		})
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.CopyFrom(txCtx,
			pgx.Identifier{"ingest", "catalog_import_rows"},
			[]string{
				"import_id", "organization_id", "source_row", "outcome", "match_level",
				"match_score", "product_id", "variant_id", "display_name",
				"source_code", "payload", "candidates", "issues", "message",
			},
			pgx.CopyFromRows(records))
		if err != nil {
			return fmt.Errorf("ingest postgres: append import rows: %w", err)
		}
		return nil
	})
}

// Rows reads a page of the results table.
func (r *Repository) Rows(
	ctx context.Context, importID int64, filter ingest.RowFilter,
) ([]*ingest.RowOutcome, int, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 50
	}
	where := []string{"import_id = $1"}
	args := []any{importID}
	if filter.Outcome != "" {
		args = append(args, filter.Outcome)
		where = append(where, fmt.Sprintf("outcome = $%d", len(args)))
	}
	if filter.MatchLevel != "" {
		args = append(args, filter.MatchLevel)
		where = append(where, fmt.Sprintf("match_level = $%d", len(args)))
	}
	if q := strings.TrimSpace(filter.Search); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("(display_name ILIKE $%d OR source_code ILIKE $%d)", len(args), len(args)))
	}
	clause := strings.Join(where, " AND ")

	var out []*ingest.RowOutcome
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM ingest.catalog_import_rows WHERE `+clause, args...,
		).Scan(&total); err != nil {
			return fmt.Errorf("ingest postgres: count import rows: %w", err)
		}

		paged := append(append([]any{}, args...), filter.Limit, filter.Offset)
		rows, err := tx.Query(txCtx, fmt.Sprintf(`
			SELECT id, source_row, outcome, match_level, match_score, product_id,
			       variant_id, display_name, source_code, payload, candidates, issues, message
			FROM ingest.catalog_import_rows
			WHERE %s
			ORDER BY source_row
			LIMIT $%d OFFSET $%d`, clause, len(args)+1, len(args)+2), paged...)
		if err != nil {
			return fmt.Errorf("ingest postgres: list import rows: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o ingest.RowOutcome
			var payload, candidates, issues []byte
			if err := rows.Scan(&o.ID, &o.SourceRow, &o.Outcome, &o.MatchLevel,
				&o.MatchScore, &o.ProductID, &o.VariantID, &o.DisplayName,
				&o.SourceCode, &payload, &candidates, &issues, &o.Message); err != nil {
				return fmt.Errorf("ingest postgres: scan import row: %w", err)
			}
			_ = json.Unmarshal(payload, &o.Payload)
			_ = json.Unmarshal(candidates, &o.Candidates)
			_ = json.Unmarshal(issues, &o.Issues)
			out = append(out, &o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// RowCounts tallies the ledger by outcome, for the results screen's tabs.
func (r *Repository) RowCounts(ctx context.Context, importID int64) (map[string]int, error) {
	counts := map[string]int{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT outcome, count(*) FROM ingest.catalog_import_rows
			WHERE import_id = $1 GROUP BY outcome`, importID)
		if err != nil {
			return fmt.Errorf("ingest postgres: count import outcomes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				return err
			}
			counts[key] = n
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}

// importDocs are the JSON columns of a session, encoded together so a failure
// to marshal one never leaves a half-written row.
type importDocs struct {
	source    []byte
	overrides []byte
	settings  []byte
	mapping   []byte
}

func encodeImport(s *ingest.Session) (importDocs, error) {
	var docs importDocs
	var err error
	if docs.source, err = json.Marshal(s.Source); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import source: %w", err)
	}
	// A map keyed by int marshals to an object of numeric strings, which is
	// exactly what the column override map needs and what decodeOverrides reads
	// back.
	if docs.overrides, err = json.Marshal(s.Overrides); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import overrides: %w", err)
	}
	if docs.settings, err = json.Marshal(s.Settings); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import settings: %w", err)
	}
	if s.Mapping == nil {
		docs.mapping = []byte(`{}`)
	} else if docs.mapping, err = json.Marshal(s.Mapping); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import mapping: %w", err)
	}
	return docs, nil
}

// scanner is the shape pgx.Row and pgx.Rows share.
type scanner interface {
	Scan(dest ...any) error
}

func scanImport(row scanner, s *ingest.Session) error {
	var phase string
	var source, overrides, settings, mapping, stats, findings []byte
	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrganizationID, &s.CreatedBy, &s.Filename,
		&s.FileSizeBytes, &phase, &source, &overrides, &settings, &mapping,
		&stats, &findings,
		&s.TotalRows, &s.InsertedRows, &s.UpdatedRows, &s.SkippedRows, &s.ErrorRows,
		&s.MatchedRows, &s.ReviewRows, &s.UnmatchedRows, &s.CreatedProducts,
		&s.ProgressPercent, &s.ProgressNote, &s.ErrorMessage,
		&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return err
	}
	s.Phase = ingest.Phase(phase)
	// A malformed document must not take the whole session down with it: the
	// vendor can still see what happened and start again.
	_ = json.Unmarshal(source, &s.Source)
	_ = json.Unmarshal(overrides, &s.Overrides)
	_ = json.Unmarshal(settings, &s.Settings)
	_ = json.Unmarshal(stats, &s.Stats)
	_ = json.Unmarshal(findings, &s.Findings)
	if len(mapping) > 2 {
		var snap ingest.MappingSnapshot
		if json.Unmarshal(mapping, &snap) == nil {
			s.Mapping = &snap
		}
	}
	s.Settings = s.Settings.Normalize()
	return nil
}

func trimTo(s string, limit int) string {
	s = sheet.CleanCell(s)
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}
