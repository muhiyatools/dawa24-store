package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Staging persistence for the reviewed catalogue import.
//
// The staging table holds a whole uploaded file — nine thousand rows is the
// ordinary case — between the moment it is parsed and the moment the admin
// confirms it. Everything here is therefore set-based or streamed; a per-row
// round trip at this size is minutes of latency in a screen the admin is
// waiting on.

const sessionColumns = `
	s.id, s.public_id, s.organization_id, s.created_by,
	s.filename, s.file_size_bytes, s.source_format, s.sheet_name, s.delimiter,
	s.status, s.import_mode, s.options, s.layout_overrides,
	s.total_rows, s.parsed_rows, s.insert_rows, s.update_rows, s.skip_rows,
	s.error_rows, s.warning_rows, s.block_count, s.new_brands, s.new_categories,
	s.ai_calls, s.ai_applied, s.ai_matched, s.ai_note, s.ai_fallback,
	s.error_message, s.created_at, s.updated_at, s.committed_at, s.expires_at,
	s.structure, s.progress_phase, s.progress_current, s.progress_total, s.progress_at`

// CreateImportSession opens a review session and collects abandoned ones.
//
// The uploaded bytes are kept on the row so the admin can correct the column
// mapping and re-run the same file without uploading it again.
func (r *Repository) CreateImportSession(ctx context.Context, s *catalog.ImportSession, sourceFile []byte) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// An abandoned review holds a copy of the admin's entire file. Reaping
		// on open keeps the staging tables bounded without a scheduled job,
		// and the work is proportional to what actually expired.
		if _, err := tx.Exec(txCtx, `
			DELETE FROM catalog.import_sessions
			WHERE status IN ('draft','ready') AND expires_at < now()
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap expired import sessions: %w", err)
		}

		// A finished session keeps its counts as history but has no further use
		// for its rows. Without this they are never collected — the expiry sweep
		// above only looks at sessions still under review — and a few hundred
		// imports leave millions of staging rows behind.
		if _, err := tx.Exec(txCtx, `
			DELETE FROM catalog.import_staging_rows r
			USING catalog.import_sessions s
			WHERE r.session_id = s.id
			  AND s.status IN ('committed','cancelled','failed')
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap finished staging rows: %w", err)
		}

		// A preparation run that never came back was interrupted the same way,
		// and leaves a worse mess: 'processing' is not reviewable, so the admin
		// gets a screen that polls for ever and can be neither cancelled nor
		// corrected. The run itself is bounded at thirty minutes, so an hour
		// without a heartbeat means no goroutine is coming back for it.
		if _, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET status = 'failed',
			    progress_phase = 'failed',
			    error_message = 'توقفت معالجة الملف قبل اكتمالها. يمكنك تصحيح ربط الأعمدة وإعادة المعالجة.'
			WHERE status = 'processing' AND updated_at < now() - INTERVAL '1 hour'
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap stale processing sessions: %w", err)
		}

		// A session claimed by a commit that never came back was interrupted —
		// a crash, a deploy, a lost connection. Its claim must not stand
		// forever: the admin would see a review screen that can neither be
		// committed nor cancelled. Two hours is several times the longest sane
		// commit, so anything older than that failed.
		if _, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET status = 'failed',
			    error_message = 'توقفت عملية الحفظ قبل اكتمالها. يرجى بدء العملية من جديد.'
			WHERE status = 'committing' AND updated_at < now() - INTERVAL '2 hours'
		`); err != nil {
			return fmt.Errorf("catalog postgres: reap stale committing sessions: %w", err)
		}

		options, err := json.Marshal(s.Options)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode import options: %w", err)
		}
		overrides, err := json.Marshal(s.Overrides)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode layout overrides: %w", err)
		}

		structure, err := json.Marshal(s.Structure)
		if err != nil {
			return fmt.Errorf("catalog postgres: encode import structure: %w", err)
		}

		// The analysis counts are written here rather than left for the
		// preparation pass. Omitting them is why a session sitting on the
		// mapping step reported nought rows and nought blocks for a file of
		// nine thousand: the screen was reading a row nobody had filled in.
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.import_sessions (
				organization_id, created_by, filename, file_size_bytes,
				source_format, sheet_name, delimiter, status, import_mode,
				options, layout_overrides, source_file,
				total_rows, block_count, structure
			) VALUES (
				$1,
				-- Attribution must never be the reason an import fails. A user id
				-- that no longer resolves — a deleted account, a stale session —
				-- records as NULL, which is what the column's ON DELETE SET NULL
				-- would have done anyway.
				(SELECT u.id FROM identity.users u WHERE u.id = $2),
				$3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12,
				$13, $14, $15::jsonb)
			RETURNING id, public_id, created_at, updated_at, expires_at
		`,
			s.OrganizationID, s.CreatedBy, s.Filename, s.FileSizeBytes,
			s.SourceFormat, s.SheetName, s.Delimiter, string(s.Status), string(s.Mode),
			string(options), string(overrides), sourceFile,
			s.TotalRows, s.BlockCount, string(structure),
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt, &s.ExpiresAt)
	})
}

// GetImportSession loads a session by its public id.
func (r *Repository) GetImportSession(ctx context.Context, publicID string) (*catalog.ImportSession, error) {
	var out *catalog.ImportSession
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+sessionColumns+` FROM catalog.import_sessions s WHERE s.public_id = $1`, publicID)
		s, err := scanImportSession(row)
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ClearStagingRows drops a finished session's rows.
//
// Called the moment a session is committed or cancelled: the rows have done
// their job, the session row keeps the counts that history needs, and holding
// nine thousand of them per import indefinitely is how the staging table grows
// without bound.
func (r *Repository) ClearStagingRows(ctx context.Context, sessionID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`DELETE FROM catalog.import_staging_rows WHERE session_id = $1`, sessionID); err != nil {
			return fmt.Errorf("catalog postgres: clear finished staging rows: %w", err)
		}
		return nil
	})
}

// ImportSourceFile reads back the bytes a session was opened with.
func (r *Repository) ImportSourceFile(ctx context.Context, sessionID int64) ([]byte, error) {
	var content []byte
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx,
			`SELECT source_file FROM catalog.import_sessions WHERE id = $1`, sessionID).Scan(&content)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("import_session")
		}
		if err != nil {
			return fmt.Errorf("catalog postgres: read import source file: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return content, nil
}

// ReleaseImportSourceFile drops the stored bytes once a session is finished
// with, so a committed import does not keep a copy of the file indefinitely.
func (r *Repository) ReleaseImportSourceFile(ctx context.Context, sessionID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`UPDATE catalog.import_sessions SET source_file = ''::bytea WHERE id = $1`, sessionID)
		if err != nil {
			return fmt.Errorf("catalog postgres: release import source file: %w", err)
		}
		return nil
	})
}

// UpdateImportSession writes back the counts, status and AI account.
//
// The variadic fromStatuses turns the write into a guarded transition: given
// any, the UPDATE carries `AND status = ANY(...)`, so a background prepare
// cannot resurrect a session an admin cancelled while it ran — the previous
// unconditional overwrite did exactly that. With none, the write is a plain
// save of the row the caller already owns.
func (r *Repository) UpdateImportSession(
	ctx context.Context, s *catalog.ImportSession, fromStatuses ...catalog.SessionStatus,
) error {
	options, err := json.Marshal(s.Options)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode import options: %w", err)
	}
	overrides, err := json.Marshal(s.Overrides)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode layout overrides: %w", err)
	}
	brands, err := json.Marshal(nonNilStrings(s.NewBrands))
	if err != nil {
		return fmt.Errorf("catalog postgres: encode new brands: %w", err)
	}
	categories, err := json.Marshal(nonNilStrings(s.NewCategories))
	if err != nil {
		return fmt.Errorf("catalog postgres: encode new categories: %w", err)
	}
	structure, err := json.Marshal(s.Structure)
	if err != nil {
		return fmt.Errorf("catalog postgres: encode import structure: %w", err)
	}

	query := `
		UPDATE catalog.import_sessions SET
			status = $2, import_mode = $3, options = $4::jsonb, layout_overrides = $5::jsonb,
			sheet_name = $6, source_format = $7, delimiter = $8,
			total_rows = $9, parsed_rows = $10, insert_rows = $11, update_rows = $12,
			skip_rows = $13, error_rows = $14, warning_rows = $15, block_count = $16,
			new_brands = $17::jsonb, new_categories = $18::jsonb,
			ai_calls = $19, ai_applied = $20, ai_matched = $21, ai_note = $22, ai_fallback = $23,
			error_message = $24, committed_at = $25, structure = $26::jsonb,
			progress_phase = $27, updated_at = now()
		WHERE id = $1`
	args := []any{
		s.ID, string(s.Status), string(s.Mode), string(options), string(overrides),
		s.SheetName, s.SourceFormat, s.Delimiter,
		s.TotalRows, s.ParsedRows, s.InsertRows, s.UpdateRows,
		s.SkipRows, s.ErrorRows, s.WarningRows, s.BlockCount,
		string(brands), string(categories),
		s.AICalls, s.AIApplied, s.AIMatched, s.AINote, s.AIFallback,
		s.ErrorMessage, s.CommittedAt, string(structure), string(s.Progress.Phase),
	}

	if len(fromStatuses) > 0 {
		statuses := make([]string, len(fromStatuses))
		for i, st := range fromStatuses {
			statuses[i] = string(st)
		}
		query += ` AND status = ANY($28::text[])`
		args = append(args, statuses)
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: update import session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("catalog.import_state_changed",
				i18n.TDefault("w4_mod.w4str_122_122"))
		}
		return nil
	})
}

// SaveImportProgress records a background run's phase on the session row.
//
// It writes only the progress columns and only while the session is still the
// running one. The alternative — reusing the full save — would have the
// background goroutine overwrite counts, options and status it does not own,
// which is a race that only ever shows up on a big file.
func (r *Repository) SaveImportProgress(
	ctx context.Context, publicID string, p catalog.ImportProgress,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE catalog.import_sessions
			SET progress_phase = $2, progress_current = $3, progress_total = $4,
			    progress_at = now()
			WHERE public_id = $1 AND status = 'processing'
		`, publicID, string(p.Phase), p.Current, p.Total)
		if err != nil {
			return fmt.Errorf("catalog postgres: save import progress: %w", err)
		}
		return nil
	})
}

// ClaimImportSessionForCommit atomically takes ownership of a reviewable
// session for committing, flipping it to 'committing' in the same statement
// that reads it.
//
// The claim is what makes concurrent commits safe across processes: two
// requests racing on one session cannot both pass, whatever in-process guards
// say, because only one of them finds a reviewable row to flip. A claimed
// session is not reviewable, so cancel and re-prepare are refused until the
// commit finishes or the reaper records it failed.
func (r *Repository) ClaimImportSessionForCommit(ctx context.Context, publicID string) (*catalog.ImportSession, error) {
	var out *catalog.ImportSession
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// The alias is not decoration: sessionColumns qualifies every column
		// with "s.", and an UPDATE's RETURNING can only see the target table
		// under its own name unless one is given. Without it every commit
		// failed with `missing FROM-clause entry for table "s"` — after the
		// admin had reviewed the whole file and pressed save.
		row := tx.QueryRow(txCtx, `
			UPDATE catalog.import_sessions AS s
			SET status = 'committing', updated_at = now()
			WHERE s.public_id = $1 AND s.status = 'ready'
			RETURNING `+sessionColumns, publicID)
		s, err := scanImportSession(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("catalog.import_not_committable",
				i18n.TDefault("w4_mod.w4str_123_123"))
		}
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceStagingRows swaps in a freshly parsed set of rows.
//
// Reprocessing is normal — the admin corrects a column mapping and runs it
// again — so this is a replace rather than an append, and it uses COPY because
// the alternative at nine thousand rows is nine thousand round trips.
func (r *Repository) ReplaceStagingRows(ctx context.Context, sessionID int64, rows []*catalog.StagingRow) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`DELETE FROM catalog.import_staging_rows WHERE session_id = $1`, sessionID); err != nil {
			return fmt.Errorf("catalog postgres: clear staging rows: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}

		source, err := newStagingCopySource(sessionID, rows)
		if err != nil {
			return err
		}

		_, err = tx.CopyFrom(txCtx,
			pgx.Identifier{"catalog", "import_staging_rows"},
			[]string{
				"session_id", "source_row", "block_index", "action", "included",
				"matched_product_id", "match_reason", "payload", "issues", "ai_changes",
				"search_name", "search_code", "has_error", "has_warning", "has_ai",
			},
			source)
		if err != nil {
			return fmt.Errorf("catalog postgres: copy staging rows: %w", err)
		}
		return nil
	})
}

// newStagingCopySource encodes the rows once, up front, so a JSON failure
// surfaces as an ordinary error rather than aborting a COPY midstream.
func newStagingCopySource(sessionID int64, rows []*catalog.StagingRow) (pgx.CopyFromSource, error) {
	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		payload, err := json.Marshal(row.Product)
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged product (row %d): %w", row.SourceRow, err)
		}
		issues, err := json.Marshal(nonNilIssues(row.Issues))
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged issues (row %d): %w", row.SourceRow, err)
		}
		changes, err := json.Marshal(nonNilChanges(row.AIChanges))
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged ai changes (row %d): %w", row.SourceRow, err)
		}

		hasError, hasWarning := false, false
		for _, issue := range row.Issues {
			if issue.Severity == catalog.SeverityError {
				hasError = true
			} else {
				hasWarning = true
			}
		}

		values = append(values, []any{
			sessionID, row.SourceRow, row.Block, string(row.Action), row.Included,
			row.MatchedProductID, string(row.MatchReason),
			string(payload), string(issues), string(changes),
			row.DisplayName(), stagingSearchCode(row),
			hasError, hasWarning, len(row.AIChanges) > 0,
		})
	}
	return pgx.CopyFromRows(values), nil
}

func stagingSearchCode(row *catalog.StagingRow) string {
	if row.Product == nil {
		return ""
	}
	return strings.TrimSpace(row.Product.SKU + " " + row.Product.Barcode)
}

// ListStagingRows reads a page of the review table.
func (r *Repository) ListStagingRows(
	ctx context.Context, sessionID int64, filter catalog.StagingFilter,
) ([]*catalog.StagingRow, int, error) {
	where := []string{"session_id = $1"}
	args := []any{sessionID}

	if filter.Action != "" {
		args = append(args, string(filter.Action))
		where = append(where, fmt.Sprintf("action = $%d", len(args)))
	}
	if filter.OnlyIssues {
		where = append(where, "(has_error OR has_warning)")
	}
	if filter.OnlyAI {
		where = append(where, "has_ai")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(search_name ILIKE $%d OR search_code ILIKE $%d)", len(args), len(args)))
	}
	clause := strings.Join(where, " AND ")

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, max(filter.Offset, 0))

	var rows []*catalog.StagingRow
	var total int

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM catalog.import_staging_rows WHERE `+clause,
			args[:len(args)-2]...).Scan(&total); err != nil {
			return fmt.Errorf("catalog postgres: count staging rows: %w", err)
		}

		// Least certain first.
		//
		// The default used to be the row's position in the administrator's
		// file, which is an order that tells the reviewer nothing: the rows
		// needing a decision are scattered evenly among the ones that do not,
		// so finding them means reading all of them — and this is the import
		// whose mistakes overwrite the entry every pharmacy reads.
		//
		// There is no score column on this table, so the ordering is by how the
		// row was decided, worst class first: anything that failed, then rows a
		// model settled, then a similarity judgement, then a name, and last the
		// identifier hits that are facts rather than opinions.
		query := fmt.Sprintf(stagingRowSelect+`
			WHERE %s
			ORDER BY
			  CASE
			    WHEN r.has_error            THEN 0
			    WHEN r.matched_product_id IS NULL THEN 1
			    WHEN r.match_reason = 'ai'  THEN 2
			    WHEN r.has_warning          THEN 3
			    WHEN r.match_reason = 'similar' THEN 4
			    WHEN r.match_reason = 'name'    THEN 5
			    ELSE 6
			  END,
			  r.source_row
			LIMIT $%d OFFSET $%d`, qualify(clause), len(args)-1, len(args))

		cursor, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: list staging rows: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			row, err := scanStagingRow(cursor)
			if err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetStagingRow returns one staged row.
func (r *Repository) GetStagingRow(ctx context.Context, sessionID, rowID int64) (*catalog.StagingRow, error) {
	var row *catalog.StagingRow
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx, stagingRowSelect+`
			WHERE r.id = $2 AND r.session_id = $1`, sessionID, rowID)
		if err != nil {
			return fmt.Errorf("catalog postgres: get staging row: %w", err)
		}
		defer cursor.Close()

		if !cursor.Next() {
			return apperr.NotFound("import_row")
		}
		row, err = scanStagingRow(cursor)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// SetRowIncluded flips one row's inclusion switch.
func (r *Repository) SetRowIncluded(ctx context.Context, sessionID, rowID int64, included bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.import_staging_rows
			SET included = $3
			WHERE id = $2 AND session_id = $1
		`, sessionID, rowID, included)
		if err != nil {
			return fmt.Errorf("catalog postgres: set staging row inclusion: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("import_row")
		}
		return nil
	})
}

// SetRowsIncludedByAction flips every row sharing an action, for the review
// table's "select all" controls. An empty action covers the whole session.
func (r *Repository) SetRowsIncludedByAction(
	ctx context.Context, sessionID int64, action catalog.RowAction, included bool,
) (int64, error) {
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.import_staging_rows SET included = $2 WHERE session_id = $1`
		args := []any{sessionID, included}
		if action != "" {
			query += ` AND action = $3`
			args = append(args, string(action))
		}
		tag, err := tx.Exec(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: bulk set staging inclusion: %w", err)
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

// CountStagingActions tallies the rows the admin has left selected, which is
// what the confirmation screen promises will happen.
func (r *Repository) CountStagingActions(ctx context.Context, sessionID int64) (catalog.StagingCounts, error) {
	var counts catalog.StagingCounts
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT
				count(*) FILTER (WHERE included AND action = 'insert'),
				count(*) FILTER (WHERE included AND action = 'update'),
				count(*) FILTER (WHERE NOT included OR action = 'skip'),
				count(*) FILTER (WHERE has_error),
				count(*) FILTER (WHERE has_warning),
				count(*) FILTER (WHERE has_ai),
				count(*)
			FROM catalog.import_staging_rows WHERE session_id = $1
		`, sessionID).Scan(
			&counts.Insert, &counts.Update, &counts.Skip,
			&counts.Errors, &counts.Warnings, &counts.AIChanged, &counts.Total)
	})
	if err != nil {
		return catalog.StagingCounts{}, fmt.Errorf("catalog postgres: count staging actions: %w", err)
	}
	return counts, nil
}

// LoadCommittableRows reads back the rows the admin left selected.
//
// The action is re-read from the table rather than recomputed, so what commits
// is exactly what the confirmation screen showed.
func (r *Repository) LoadCommittableRows(ctx context.Context, sessionID int64) ([]*catalog.StagingRow, error) {
	var rows []*catalog.StagingRow
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx, stagingRowSelect+`
			WHERE r.session_id = $1 AND r.included
			  AND r.action <> 'skip' AND NOT r.has_error
			ORDER BY r.source_row`, sessionID)
		if err != nil {
			return fmt.Errorf("catalog postgres: load committable rows: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			row, err := scanStagingRow(cursor)
			if err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ArchiveAllProducts soft-deletes the whole catalogue for the clear-and-add
// mode, returning how many rows it retired.
//
// Soft, never hard: orders, variants and saved lists reference these ids, and a
// physical delete would cascade through all of them. An archived product can be
// brought back; a deleted one takes an order's history with it.
func (r *Repository) ArchiveAllProducts(ctx context.Context, orgID int64) (int64, error) {
	var archived int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.products
			SET deleted_at = now(), updated_at = now()
			WHERE organization_id = $1 AND deleted_at IS NULL
		`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: archive catalogue: %w", err)
		}
		archived = tag.RowsAffected()
		return nil
	})
	return archived, err
}

// ImportVocabulary reads the taxonomy an enrichment pass must choose within.
//
// Handing the model the platform's own categories and brands by id is what stops
// an import from fragmenting the taxonomy: it classifies into what exists rather
// than inventing a fifth spelling of i18n.TDefault("w4_mod.s_267_267").
func (r *Repository) ImportVocabulary(ctx context.Context, orgID int64) (catalog.EnrichVocabulary, error) {
	var vocab catalog.EnrichVocabulary

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		if vocab.Categories, err = loadCategoryOptions(txCtx, tx); err != nil {
			return err
		}
		if vocab.Brands, err = loadBrandOptions(txCtx, tx); err != nil {
			return err
		}
		vocab.DosageForms, err = loadDosageForms(txCtx, tx)
		return err
	})
	if err != nil {
		return catalog.EnrichVocabulary{}, err
	}
	return vocab, nil
}

func loadCategoryOptions(ctx context.Context, tx pgx.Tx) ([]catalog.TaxonomyOption, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, COALESCE(name->>'ar', name->>'en', '')
		FROM catalog.categories
		WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load category vocabulary: %w", err)
	}
	defer rows.Close()

	var out []catalog.TaxonomyOption
	for rows.Next() {
		var opt catalog.TaxonomyOption
		if err := rows.Scan(&opt.ID, &opt.Name); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan category: %w", err)
		}
		if opt.Name != "" {
			out = append(out, opt)
		}
	}
	return out, rows.Err()
}

// loadBrandOptions orders brands by how much of the catalogue already uses
// them, so the truncation that keeps the prompt bounded drops the long tail
// rather than an arbitrary slice.
func loadBrandOptions(ctx context.Context, tx pgx.Tx) ([]catalog.TaxonomyOption, error) {
	rows, err := tx.Query(ctx, `
		SELECT b.id, COALESCE(b.name->>'ar', b.name->>'en', ''), count(p.id) AS uses
		FROM catalog.brands b
		LEFT JOIN catalog.products p ON p.brand_id = b.id AND p.deleted_at IS NULL
		WHERE b.deleted_at IS NULL
		GROUP BY b.id, b.name
		ORDER BY uses DESC, b.id
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load brand vocabulary: %w", err)
	}
	defer rows.Close()

	var out []catalog.TaxonomyOption
	for rows.Next() {
		var opt catalog.TaxonomyOption
		var uses int64
		if err := rows.Scan(&opt.ID, &opt.Name, &uses); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan brand: %w", err)
		}
		if opt.Name != "" {
			out = append(out, opt)
		}
	}
	return out, rows.Err()
}

func loadDosageForms(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT dosage_form
		FROM catalog.products
		WHERE deleted_at IS NULL AND btrim(dosage_form) <> ''
		ORDER BY dosage_form
		LIMIT 100
	`)
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: load dosage form vocabulary: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var form string
		if err := rows.Scan(&form); err != nil {
			return nil, fmt.Errorf("catalog postgres: scan dosage form: %w", err)
		}
		out = append(out, form)
	}
	return out, rows.Err()
}

// ListRecentImportSessions backs the import history panel.
func (r *Repository) ListRecentImportSessions(ctx context.Context, orgID int64, limit int) ([]*catalog.ImportSession, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var out []*catalog.ImportSession

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx,
			`SELECT `+sessionColumns+`
			 FROM catalog.import_sessions s
			 WHERE s.organization_id = $1
			 ORDER BY s.created_at DESC
			 LIMIT $2`, orgID, limit)
		if err != nil {
			return fmt.Errorf("catalog postgres: list import sessions: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			s, err := scanImportSession(cursor)
			if err != nil {
				return err
			}
			out = append(out, s)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface{ Scan(dest ...any) error }

func scanImportSession(row rowScanner) (*catalog.ImportSession, error) {
	var s catalog.ImportSession
	var status, mode, phase string
	var options, overrides, brands, categories, structure []byte
	var progressAt *time.Time

	err := row.Scan(
		&s.ID, &s.PublicID, &s.OrganizationID, &s.CreatedBy,
		&s.Filename, &s.FileSizeBytes, &s.SourceFormat, &s.SheetName, &s.Delimiter,
		&status, &mode, &options, &overrides,
		&s.TotalRows, &s.ParsedRows, &s.InsertRows, &s.UpdateRows, &s.SkipRows,
		&s.ErrorRows, &s.WarningRows, &s.BlockCount, &brands, &categories,
		&s.AICalls, &s.AIApplied, &s.AIMatched, &s.AINote, &s.AIFallback,
		&s.ErrorMessage, &s.CreatedAt, &s.UpdatedAt, &s.CommittedAt, &s.ExpiresAt,
		&structure, &phase, &s.Progress.Current, &s.Progress.Total, &progressAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("import_session")
	}
	if err != nil {
		return nil, fmt.Errorf("catalog postgres: scan import session: %w", err)
	}

	s.Status = catalog.SessionStatus(status)
	s.Mode = catalog.ImportMode(mode)
	if err := json.Unmarshal(options, &s.Options); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode import options: %w", err)
	}
	if err := json.Unmarshal(overrides, &s.Overrides); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode layout overrides: %w", err)
	}
	if err := json.Unmarshal(brands, &s.NewBrands); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode new brands: %w", err)
	}
	if err := json.Unmarshal(categories, &s.NewCategories); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode new categories: %w", err)
	}
	// The structure document is advisory: a session written before the column
	// existed decodes to the zero value, and the screens fall back to
	// re-reading the file rather than refusing to render.
	if len(structure) > 0 {
		if err := json.Unmarshal(structure, &s.Structure); err != nil {
			return nil, fmt.Errorf("catalog postgres: decode import structure: %w", err)
		}
	}
	s.Progress.Phase = catalog.ImportPhase(phase)
	s.Progress.Message = s.Progress.Phase.Label()
	if progressAt != nil {
		s.Progress.UpdatedAt = *progressAt
		s.Progress.StartedAt = *progressAt
	}
	return &s, nil
}

// stagingRowSelect is the one column list every staging-row read uses.
//
// It is shared rather than repeated because it drifted the moment it was not:
// two of the three queries grew the matched-product join and the third did not,
// and the mismatch surfaced only at commit time as "number of field
// descriptions must equal number of destinations, got 11 and 13" — after the
// admin had reviewed nine thousand rows and pressed save. scanStagingRow below
// is the other half of this contract; the two change together or not at all.
//
// The matched product's name comes from the join rather than from a copy taken
// at staging time. An import is reviewed minutes or hours after it was
// prepared, and a cached name would disagree with the product the admin opens
// to check it against — which is worse than showing no name.
const stagingRowSelect = `
	SELECT r.id, r.session_id, r.source_row, r.block_index, r.action, r.included,
	       r.matched_product_id, r.match_reason, r.payload, r.issues, r.ai_changes,
	       COALESCE(NULLIF(p.name->>'ar', ''), NULLIF(p.name->>'en', ''), '') AS matched_name,
	       COALESCE(p.sku, '') AS matched_sku
	FROM catalog.import_staging_rows r
	LEFT JOIN catalog.products p ON p.id = r.matched_product_id
`

func scanStagingRow(cursor pgx.Rows) (*catalog.StagingRow, error) {
	var row catalog.StagingRow
	var action, matchReason string
	var payload, issues, changes []byte

	if err := cursor.Scan(
		&row.ID, &row.SessionID, &row.SourceRow, &row.Block, &action, &row.Included,
		&row.MatchedProductID, &matchReason, &payload, &issues, &changes,
		&row.MatchedProductName, &row.MatchedProductSKU,
	); err != nil {
		return nil, fmt.Errorf("catalog postgres: scan staging row: %w", err)
	}

	row.Action = catalog.RowAction(action)
	row.MatchReason = catalog.MatchReason(matchReason)

	var product catalog.Product
	if err := json.Unmarshal(payload, &product); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged product (row %d): %w", row.SourceRow, err)
	}
	// A product decoded from JSON has a nil Name map when the file gave no name
	// at all; downstream code writes into it, so it must never be nil.
	if product.Name == nil {
		product.Name = i18n.Text{}
	}
	row.Product = &product

	if err := json.Unmarshal(issues, &row.Issues); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged issues (row %d): %w", row.SourceRow, err)
	}
	if err := json.Unmarshal(changes, &row.AIChanges); err != nil {
		return nil, fmt.Errorf("catalog postgres: decode staged ai changes (row %d): %w", row.SourceRow, err)
	}
	return &row, nil
}

// qualify prefixes the staging-row filter clause with the table alias the
// listing query joins under. The clause is built from a fixed set of column
// names in ListStagingRows, never from input.
func qualify(clause string) string {
	for _, col := range []string{
		"session_id", "action", "has_error", "has_warning", "has_ai",
		"search_name", "search_code",
	} {
		clause = strings.ReplaceAll(clause, col, "r."+col)
	}
	return clause
}

// The JSONB columns are NOT NULL, so a nil Go slice must encode as [] and not
// as the JSON literal null.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilIssues(in []catalog.RowIssue) []catalog.RowIssue {
	if in == nil {
		return []catalog.RowIssue{}
	}
	return in
}

func nonNilChanges(in []catalog.AIChange) []catalog.AIChange {
	if in == nil {
		return []catalog.AIChange{}
	}
	return in
}
