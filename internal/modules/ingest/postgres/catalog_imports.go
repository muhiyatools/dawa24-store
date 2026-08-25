package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Persistence for the vendor catalogue import.
//
// The uploaded file lives on the session row and the parsed rows do not live
// anywhere: they are streamed out of the file when the run happens. That is the
// difference between this and the importer it replaces, which wrote nine
// thousand JSON documents into Postgres before it knew what any of them were.

// importColumns is the projection every read shares.
const importColumns = `
	id, public_id, organization_id, created_by, filename, file_size_bytes,
	phase, source, overrides, settings, mapping, stats, findings,
	total_rows, inserted_rows, updated_rows, skipped_rows, error_rows,
	matched_rows, review_rows, unmatched_rows, created_products,
	progress_percent, progress_note, error_message,
	started_at, completed_at, created_at, updated_at, expires_at`

// Create opens an import and stores the uploaded bytes with it.
func (r *Repository) Create(ctx context.Context, s *ingest.Session, file []byte) error {
	docs, err := encodeImport(s)
	if err != nil {
		return err
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO ingest.catalog_imports (
				organization_id, created_by, filename, file_size_bytes, source_file,
				phase, source, overrides, settings, total_rows
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at, expires_at`,
			s.OrganizationID, s.CreatedBy, s.Filename, s.FileSizeBytes, file,
			string(s.Phase), docs.source, docs.overrides, docs.settings, s.TotalRows,
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt)
	})
}

// Get loads an import by its public id.
func (r *Repository) Get(ctx context.Context, publicID string) (*ingest.Session, error) {
	var s ingest.Session
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+importColumns+` FROM ingest.catalog_imports WHERE public_id = $1`, publicID)
		return scanImport(row, &s)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("catalog_import")
		}
		return nil, err
	}
	return &s, nil
}

// File returns the stored upload.
func (r *Repository) File(ctx context.Context, id int64) ([]byte, error) {
	var out []byte
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT source_file FROM ingest.catalog_imports WHERE id = $1`, id).Scan(&out)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("catalog_import")
		}
		return nil, err
	}
	if len(out) == 0 {
		return nil, apperr.NotFound("catalog_import_file")
	}
	return out, nil
}

// SaveDraft persists the vendor's corrections and settings and moves the phase.
func (r *Repository) SaveDraft(ctx context.Context, s *ingest.Session) error {
	docs, err := encodeImport(s)
	if err != nil {
		return err
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = $2, source = $3, overrides = $4, settings = $5, mapping = $6,
			    total_rows = $7
			WHERE id = $1 AND phase IN ('mapping','settings','confirm')`,
			s.ID, string(s.Phase), docs.source, docs.overrides, docs.settings,
			docs.mapping, s.TotalRows)
		if err != nil {
			return fmt.Errorf("ingest postgres: save import draft: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_open", "لم يعد بالإمكان تعديل هذه الجلسة.")
		}
		return nil
	})
}

// Begin marks the run started and clears any previous outcome.
func (r *Repository) Begin(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'processing', started_at = now(), completed_at = NULL,
			    progress_percent = 0, progress_note = '', error_message = '',
			    inserted_rows = 0, updated_rows = 0, skipped_rows = 0, error_rows = 0,
			    matched_rows = 0, review_rows = 0, unmatched_rows = 0, created_products = 0,
			    findings = '[]'::JSONB, stats = '{}'::JSONB
			WHERE id = $1 AND phase IN ('mapping','settings','confirm','failed')`, id)
		if err != nil {
			return fmt.Errorf("ingest postgres: begin import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.already_running", "هذه الجلسة قيد التنفيذ أو منتهية بالفعل.")
		}
		// A re-run must not show the previous run's rows beside the new ones.
		if _, err := tx.Exec(txCtx,
			`DELETE FROM ingest.catalog_import_rows WHERE import_id = $1`, id); err != nil {
			return fmt.Errorf("ingest postgres: clear import rows: %w", err)
		}
		return nil
	})
}

// Progress records how far a run has reached.
func (r *Repository) Progress(ctx context.Context, id int64, percent int, note string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET progress_percent = LEAST(GREATEST($2, 0), 100), progress_note = $3
			WHERE id = $1`, id, percent, note)
		if err != nil {
			return fmt.Errorf("ingest postgres: import progress: %w", err)
		}
		return nil
	})
}

// Finish records the outcome of a completed run.
func (r *Repository) Finish(ctx context.Context, s *ingest.Session) error {
	stats, err := json.Marshal(s.Stats)
	if err != nil {
		return fmt.Errorf("ingest postgres: encode import stats: %w", err)
	}
	findings, err := json.Marshal(s.Findings)
	if err != nil {
		return fmt.Errorf("ingest postgres: encode import findings: %w", err)
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'completed', completed_at = now(), progress_percent = 100,
			    progress_note = '', stats = $2, findings = $3,
			    total_rows = $4, inserted_rows = $5, updated_rows = $6,
			    skipped_rows = $7, error_rows = $8, matched_rows = $9,
			    review_rows = $10, unmatched_rows = $11, created_products = $12
			WHERE id = $1`,
			s.ID, stats, findings, s.TotalRows, s.InsertedRows, s.UpdatedRows,
			s.SkippedRows, s.ErrorRows, s.MatchedRows, s.ReviewRows,
			s.UnmatchedRows, s.CreatedProducts)
		if err != nil {
			return fmt.Errorf("ingest postgres: finish import: %w", err)
		}
		return nil
	})
}

// Fail records a run that stopped on an error.
func (r *Repository) Fail(ctx context.Context, id int64, message string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'failed', completed_at = now(), error_message = $2
			WHERE id = $1`, id, message)
		if err != nil {
			return fmt.Errorf("ingest postgres: fail import: %w", err)
		}
		return nil
	})
}

// Cancel discards an import without touching the catalogue.
func (r *Repository) Cancel(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'cancelled', completed_at = now(), source_file = ''::BYTEA
			WHERE id = $1 AND phase IN ('mapping','settings','confirm')`, id)
		if err != nil {
			return fmt.Errorf("ingest postgres: cancel import: %w", err)
		}
		return nil
	})
}

// List backs the history panel on the upload screen.
func (r *Repository) List(ctx context.Context, orgID int64, limit int) ([]*ingest.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var out []*ingest.Session
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `SELECT `+importColumns+`
			FROM ingest.catalog_imports
			WHERE organization_id = $1
			ORDER BY created_at DESC
			LIMIT $2`, orgID, limit)
		if err != nil {
			return fmt.Errorf("ingest postgres: list imports: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s ingest.Session
			if err := scanImport(rows, &s); err != nil {
				return err
			}
			out = append(out, &s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Sweep collects abandoned imports and the files they hold.
func (r *Repository) Sweep(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The bytes go first and unconditionally: an abandoned review holds a
		// copy of a vendor's whole catalogue, and that is the part worth
		// reclaiming even where the record itself is kept for the history panel.
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET source_file = ''::BYTEA
			WHERE expires_at < now() AND octet_length(source_file) > 0`); err != nil {
			return fmt.Errorf("ingest postgres: sweep import files: %w", err)
		}
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'cancelled', completed_at = now()
			WHERE expires_at < now() AND phase IN ('mapping','settings','confirm')`); err != nil {
			return fmt.Errorf("ingest postgres: sweep imports: %w", err)
		}
		return nil
	})
}
