package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

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
	phase, source, overrides, settings, mapping, stats, findings, ai_stats,
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
			    total_rows = $7, matched_rows = $8, review_rows = $9, unmatched_rows = $10,
			    error_rows = $11, updated_rows = $12, inserted_rows = $13, skipped_rows = $14,
			    ai_stats = $15
			WHERE id = $1 AND phase IN ('mapping','settings','review','confirm')`,
			s.ID, string(s.Phase), docs.source, docs.overrides, docs.settings,
			docs.mapping, s.TotalRows, s.MatchedRows, s.ReviewRows, s.UnmatchedRows,
			s.ErrorRows, s.UpdatedRows, s.InsertedRows, s.SkippedRows, docs.aiStats)
		if err != nil {
			return fmt.Errorf("ingest postgres: save import draft: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_open", i18n.TDefault("w4_mod.w4str_200_200"))
		}
		return nil
	})
}

// FinishStaging moves a run out of 'processing' and records what it produced.
//
// SaveDraft deliberately refuses a session in 'processing' — that guard is what
// stops a vendor editing settings underneath a run that is already reading
// them — so a staging pass cannot use it to publish its own result. Using it
// anyway is a bug this code has already had: the pass set 'processing', worked
// for several minutes, and then wrote its outcome through SaveDraft, which
// matched zero rows and returned "not open". The failure handler then tried the
// same call and was refused for the same reason, so every import wedged in
// 'processing' at 95% with the work complete and nothing to show for it.
//
// This is the transition that owns the other end of Begin.
func (r *Repository) FinishStaging(ctx context.Context, s *ingest.Session) error {
	docs, err := encodeImport(s)
	if err != nil {
		return err
	}
	stats, err := json.Marshal(s.Stats)
	if err != nil {
		return fmt.Errorf("ingest postgres: encode import stats: %w", err)
	}
	findings, err := json.Marshal(s.Findings)
	if err != nil {
		return fmt.Errorf("ingest postgres: encode import findings: %w", err)
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = $2, settings = $3, mapping = $4, stats = $5, findings = $6,
			    total_rows = $7, matched_rows = $8, review_rows = $9, unmatched_rows = $10,
			    error_rows = $11, ai_stats = $12, error_message = $13,
			    progress_percent = CASE WHEN $2 = 'review' THEN 100 ELSE progress_percent END,
			    completed_at = CASE WHEN $2 = 'failed' THEN now() ELSE completed_at END
			WHERE id = $1 AND phase = 'processing'`,
			s.ID, string(s.Phase), docs.settings, docs.mapping, stats, findings,
			s.TotalRows, s.MatchedRows, s.ReviewRows, s.UnmatchedRows,
			s.ErrorRows, docs.aiStats, s.ErrorMessage)
		if err != nil {
			return fmt.Errorf("ingest postgres: finish staging: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_processing",
				i18n.TDefault("w4_mod.w4str_221_221"))
		}
		return nil
	})
}

// CountWedgedRuns reports how many sessions are stuck in 'processing', so an
// operator can see the damage before changing anything.
func (r *Repository) CountWedgedRuns(ctx context.Context) (int, error) {
	var n int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT COUNT(*) FROM ingest.catalog_imports
			WHERE phase = 'processing'
			  AND COALESCE(started_at, updated_at, created_at) < now() - INTERVAL '`+staleRunAfter+`'`).Scan(&n)
	})
	return n, err
}

// RecoverStaleRuns releases sessions wedged in 'processing'.
//
// A detached staging run lives in the web process. A deploy, a crash, an OOM
// kill — or, as happened here, an outcome write the phase predicate refused —
// leaves the session in a phase that Begin, SaveDraft and the progress screen
// all treat as live. The vendor gets a bar that polls for ever against work
// nobody is doing, and the only recovery was editing the row by hand.
//
// It distinguishes two cases, because they deserve different answers:
//
//   - Rows are staged. The matching finished and only the outcome write was
//     lost, so the session is promoted to 'review' with its counters recomputed
//     from the rows themselves. The vendor gets the result they already paid
//     for, including the AI stage.
//   - No rows are staged. Nothing usable exists, so it is failed with a message
//     telling the vendor to upload again.
//
// Counters come from the rows rather than from memory precisely because the
// memory is gone: whatever the run held was lost with the process, and the
// rows are the only surviving record of what it decided.
func (r *Repository) RecoverStaleRuns(ctx context.Context) (int, error) {
	var recovered int
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		promoted, err := tx.Exec(txCtx, `
			WITH stale AS (
				SELECT i.id
				FROM ingest.catalog_imports i
				WHERE i.phase = 'processing'
				  AND COALESCE(i.started_at, i.updated_at, i.created_at) < now() - INTERVAL '`+staleRunAfter+`'
				  AND EXISTS (SELECT 1 FROM ingest.catalog_import_rows r WHERE r.import_id = i.id)
			), tally AS (
				SELECT r.import_id,
				       COUNT(*) FILTER (WHERE r.match_level IN ('barcode','code','exact','strong')) AS matched,
				       COUNT(*) FILTER (WHERE r.match_level IN ('review','ambiguous'))              AS review,
				       COUNT(*) FILTER (WHERE r.match_level = 'none' OR r.match_level IS NULL)      AS unmatched
				FROM ingest.catalog_import_rows r
				JOIN stale ON stale.id = r.import_id
				GROUP BY r.import_id
			)
			UPDATE ingest.catalog_imports i
			SET phase = 'review',
			    progress_percent = 100,
			    progress_note = 'اكتملت المعالجة',
			    matched_rows = t.matched,
			    review_rows = t.review,
			    unmatched_rows = t.unmatched
			FROM tally t
			WHERE i.id = t.import_id`)
		if err != nil {
			return fmt.Errorf("ingest postgres: promote stale runs: %w", err)
		}

		failed, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports i
			SET phase = 'failed',
			    error_message = 'توقفت المعالجة قبل اكتمالها. يرجى إعادة رفع الملف والمحاولة مرة أخرى.',
			    completed_at = now()
			WHERE i.phase = 'processing'
			  AND COALESCE(i.started_at, i.updated_at, i.created_at) < now() - INTERVAL '`+staleRunAfter+`'
			  AND NOT EXISTS (SELECT 1 FROM ingest.catalog_import_rows r WHERE r.import_id = i.id)`)
		if err != nil {
			return fmt.Errorf("ingest postgres: fail stale runs: %w", err)
		}

		recovered = int(promoted.RowsAffected() + failed.RowsAffected())
		return nil
	})
	return recovered, err
}

// staleRunAfter is how long a 'processing' phase is trusted before it is
// treated as a run that died with its process. Begin refuses live runs; past
// this age the claim is stale — a crash or deploy left the session wedged where
// confirm, cancel and sweep all refused it, recoverable only by hand.
const staleRunAfter = "2 hours"

// Begin marks the run started and clears any previous outcome.
//
// The per-organisation advisory lock serialises claims across processes: two
// different sessions for the same vendor confirming at once would otherwise
// both load stale variant indexes and race their writes. The lock lives for
// this transaction only — it makes the claim atomic, not the whole run — but
// the phase predicate does the rest: only one of them finds a claimable row.
//
// A processing phase older than staleRunAfter is reclaimable, so a run killed
// by a crash or a deploy cannot wedge the session forever.
func (r *Repository) Begin(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`SELECT pg_advisory_xact_lock(
				(SELECT organization_id FROM ingest.catalog_imports WHERE id = $1))`, id); err != nil {
			return fmt.Errorf("ingest postgres: lock import organization: %w", err)
		}
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'processing', started_at = now(), completed_at = NULL,
			    progress_percent = 0, progress_note = '', error_message = '',
			    inserted_rows = 0, updated_rows = 0, skipped_rows = 0, error_rows = 0,
			    matched_rows = 0, review_rows = 0, unmatched_rows = 0, created_products = 0,
			    findings = '[]'::JSONB, stats = '{}'::JSONB
			WHERE id = $1 AND (
				phase IN ('mapping','settings','review','confirm','failed')
				OR (phase = 'processing' AND started_at < now() - INTERVAL '`+staleRunAfter+`')
			)`, id)
		if err != nil {
			return fmt.Errorf("ingest postgres: begin import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.already_running", i18n.TDefault("w4_mod.w4str_222_222"))
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
//
// Guarded by phase: without the predicate a slow progress writer racing Finish
// could overwrite a completed import's 100% with a mid-run figure — and leave
// it there permanently.
func (r *Repository) Progress(ctx context.Context, id int64, percent int, note string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET progress_percent = LEAST(GREATEST($2, 0), 100), progress_note = $3
			WHERE id = $1 AND phase IN ('processing','review','confirm')`, id, percent, note)
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
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'completed', completed_at = now(), progress_percent = 100,
			    progress_note = '', stats = $2, findings = $3,
			    total_rows = $4, inserted_rows = $5, updated_rows = $6,
			    skipped_rows = $7, error_rows = $8, matched_rows = $9,
			    review_rows = $10, unmatched_rows = $11, created_products = $12
			WHERE id = $1 AND phase IN ('processing','review','confirm','settings','mapping')`,
			s.ID, stats, findings, s.TotalRows, s.InsertedRows, s.UpdatedRows,
			s.SkippedRows, s.ErrorRows, s.MatchedRows, s.ReviewRows,
			s.UnmatchedRows, s.CreatedProducts)
		if err != nil {
			return fmt.Errorf("ingest postgres: finish import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_processing",
				i18n.TDefault("w4_mod.w4str_223_223"))
		}
		return nil
	})
}

// Fail records a run that stopped on an error.
func (r *Repository) Fail(ctx context.Context, id int64, message string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'failed', completed_at = now(), error_message = $2
			WHERE id = $1 AND phase IN ('processing','review','confirm','settings','mapping')`, id, message)
		if err != nil {
			return fmt.Errorf("ingest postgres: fail import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_processing",
				i18n.TDefault("w4_mod.w4str_224_224"))
		}
		return nil
	})
}

// Cancel discards an import without touching the catalogue. Cancelling a
// session that is no longer open reports the conflict rather than pretending
// the file was purged when it was not.
func (r *Repository) Cancel(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'cancelled', completed_at = now(), source_file = ''::BYTEA
			WHERE id = $1 AND phase IN ('mapping','settings','review','confirm')`, id)
		if err != nil {
			return fmt.Errorf("ingest postgres: cancel import: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import.not_open", i18n.TDefault("w4_mod.w4str_225_225"))
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
			WHERE expires_at < now() AND phase IN ('mapping','settings','review','confirm')`); err != nil {
			return fmt.Errorf("ingest postgres: sweep imports: %w", err)
		}
		// A run wedged in processing past the stale threshold is dead with its
		// process; recording it failed is what lets the vendor re-run it.
		if _, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET phase = 'failed', completed_at = now(),
			    error_message = 'توقفت العملية قبل اكتمالها. يمكن بدء الاستيراد من جديد.'
			WHERE phase = 'processing' AND started_at < now() - INTERVAL '`+staleRunAfter+`'`); err != nil {
			return fmt.Errorf("ingest postgres: sweep stale processing imports: %w", err)
		}
		return nil
	})
}

// importDocs are the JSON columns of a session, encoded together so a failure
// to marshal one never leaves a half-written row.
type importDocs struct {
	source    []byte
	overrides []byte
	settings  []byte
	mapping   []byte
	aiStats   []byte
}

func encodeImport(s *ingest.Session) (importDocs, error) {
	var docs importDocs
	var err error
	if docs.source, err = json.Marshal(s.Source); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import source: %w", err)
	}
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
	if docs.aiStats, err = json.Marshal(s.AI); err != nil {
		return docs, fmt.Errorf("ingest postgres: encode import ai stats: %w", err)
	}
	return docs, nil
}

// scanner is the shape pgx.Row and pgx.Rows share.
type scanner interface {
	Scan(dest ...any) error
}

func scanImport(row scanner, s *ingest.Session) error {
	var phase string
	var source, overrides, settings, mapping, stats, findings, aiStats []byte
	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrganizationID, &s.CreatedBy, &s.Filename,
		&s.FileSizeBytes, &phase, &source, &overrides, &settings, &mapping,
		&stats, &findings, &aiStats,
		&s.TotalRows, &s.InsertedRows, &s.UpdatedRows, &s.SkippedRows, &s.ErrorRows,
		&s.MatchedRows, &s.ReviewRows, &s.UnmatchedRows, &s.CreatedProducts,
		&s.ProgressPercent, &s.ProgressNote, &s.ErrorMessage,
		&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return err
	}
	s.Phase = ingest.Phase(phase)
	_ = json.Unmarshal(source, &s.Source)
	_ = json.Unmarshal(overrides, &s.Overrides)
	_ = json.Unmarshal(settings, &s.Settings)
	_ = json.Unmarshal(stats, &s.Stats)
	_ = json.Unmarshal(findings, &s.Findings)
	_ = json.Unmarshal(aiStats, &s.AI)
	if len(mapping) > 2 {
		var snap ingest.MappingSnapshot
		if json.Unmarshal(mapping, &snap) == nil {
			s.Mapping = &snap
		}
	}
	s.Settings = s.Settings.Normalize()
	return nil
}
