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

// Repository implements ingest.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new ingest PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// DocumentTypeImportFile marks uploaded import files inside
// platform_admin.documents (070 — one attachment table; mirrors
// attachments.DocImportFile without crossing the module boundary).
const DocumentTypeImportFile = "import_file"

// CreateFileUpload records a pointer to a stored import file. Uploads live on
// platform_admin.documents since 070; documents has no RLS, so these run as
// system with the tenant columns filtered explicitly.
func (r *Repository) CreateFileUpload(ctx context.Context, f *ingest.FileUpload) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.documents (
				organization_id, user_id, title, document_type, storage_key,
				original_name, file_url, status, mime_type, size_bytes
			) VALUES ($1, $2, $3, $4, $5, $6, '', 'pending', $7, $8)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			f.OrganizationID, f.UserID, f.Filename, DocumentTypeImportFile,
			f.StorageKey, f.Filename, f.MimeType, f.FileSizeBytes,
		).Scan(&f.ID, &f.PublicID, &f.CreatedAt)
	})
}

// GetFileUploadByID retrieves an upload record by ID (platform_admin.documents
// since 070).
func (r *Repository) GetFileUploadByID(ctx context.Context, id int64) (*ingest.FileUpload, error) {
	var f ingest.FileUpload
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, original_name, storage_key, size_bytes, mime_type, created_at
			FROM platform_admin.documents
			WHERE id = $1 AND document_type = $2;
		`
		return tx.QueryRow(txCtx, query, id, DocumentTypeImportFile).Scan(
			&f.ID, &f.PublicID, &f.OrganizationID, &f.UserID, &f.Filename,
			&f.StorageKey, &f.FileSizeBytes, &f.MimeType, &f.CreatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("file_upload")
		}
		return nil, err
	}
	return &f, nil
}

// CreateImportSession creates a session for tracking import execution.
func (r *Repository) CreateImportSession(ctx context.Context, s *ingest.ImportSession) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		mappingJSON, err := json.Marshal(s.ColumnMapping)
		if err != nil {
			return err
		}

		query := `
			INSERT INTO ingest.import_sessions (
				organization_id, file_upload_id, status, column_mapping, min_similarity_score, total_rows
			) VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			s.OrganizationID, s.FileUploadID, string(s.Status), mappingJSON, s.MinSimilarityScore, s.TotalRows,
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// GetImportSessionByID retrieves an import session.
func (r *Repository) GetImportSessionByID(ctx context.Context, id int64) (*ingest.ImportSession, error) {
	var s ingest.ImportSession
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, file_upload_id, status, column_mapping,
			       min_similarity_score, total_rows, processed_rows, matched_rows,
			       error_message, started_at, completed_at, created_at, updated_at
			FROM ingest.import_sessions
			WHERE id = $1;
		`
		var statusStr string
		var mappingJSON []byte
		var errMsg *string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&s.ID, &s.PublicID, &s.OrganizationID, &s.FileUploadID, &statusStr, &mappingJSON,
			&s.MinSimilarityScore, &s.TotalRows, &s.ProcessedRows, &s.MatchedRows,
			&errMsg, &s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("import_session")
			}
			return err
		}
		s.Status = ingest.SessionStatus(statusStr)
		if errMsg != nil {
			s.ErrorMessage = *errMsg
		}
		if len(mappingJSON) > 0 {
			_ = json.Unmarshal(mappingJSON, &s.ColumnMapping)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateImportSessionProgress updates processed count and lifecycle state.
func (r *Repository) UpdateImportSessionProgress(
	ctx context.Context,
	id int64,
	processed, matched int,
	status ingest.SessionStatus,
	errMsg string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_sessions
			SET processed_rows = $2, matched_rows = $3, status = $4, error_message = $5, updated_at = now()
			WHERE id = $1;
		`
		// error_message is NOT NULL DEFAULT '' as of migration 033; an empty
		// message is stored as '' rather than inverted to NULL.
		_, err := tx.Exec(txCtx, query, id, processed, matched, string(status), errMsg)
		return err
	})
}

// InsertImportRows writes a batch of staged rows.
func (r *Repository) InsertImportRows(ctx context.Context, rows []*ingest.ImportRow) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, row := range rows {
			rawJSON, err := json.Marshal(row.RawData)
			if err != nil {
				return err
			}
			query := `
				INSERT INTO ingest.import_rows (
					session_id, organization_id, row_number, raw_data, normalized_name, status
				) VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id, created_at;
			`
			if err := tx.QueryRow(txCtx, query,
				row.SessionID, row.OrganizationID, row.RowNumber, rawJSON, row.NormalizedName, row.Status,
			).Scan(&row.ID, &row.CreatedAt); err != nil {
				return fmt.Errorf("ingest postgres: insert row %d: %w", row.RowNumber, err)
			}
		}
		return nil
	})
}

// ListImportRows retrieves staged rows for a session.
func (r *Repository) ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	var rowsList []*ingest.ImportRow
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, session_id, organization_id, row_number, raw_data, normalized_name,
			       matched_product_id, similarity_score, status, created_at
			FROM ingest.import_rows
			WHERE session_id = $1 AND ($2 = '' OR status = $2)
			ORDER BY row_number ASC
			LIMIT $3 OFFSET $4;
		`
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		rows, err := tx.Query(txCtx, query, sessionID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ir ingest.ImportRow
			var rawJSON []byte
			var normName *string
			if err := rows.Scan(
				&ir.ID, &ir.SessionID, &ir.OrganizationID, &ir.RowNumber, &rawJSON,
				&normName, &ir.MatchedProductID, &ir.SimilarityScore, &ir.Status, &ir.CreatedAt,
			); err != nil {
				return err
			}
			if normName != nil {
				ir.NormalizedName = *normName
			}
			if len(rawJSON) > 0 {
				_ = json.Unmarshal(rawJSON, &ir.RawData)
			}
			rowsList = append(rowsList, &ir)
		}
		return rows.Err()
	})
	return rowsList, err
}

// GetImportRowByID retrieves a single staged row.
func (r *Repository) GetImportRowByID(ctx context.Context, id int64) (*ingest.ImportRow, error) {
	var ir ingest.ImportRow
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, session_id, organization_id, row_number, raw_data, normalized_name,
			       matched_product_id, similarity_score, status, created_at
			FROM ingest.import_rows
			WHERE id = $1;
		`
		var rawJSON []byte
		var normName *string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&ir.ID, &ir.SessionID, &ir.OrganizationID, &ir.RowNumber, &rawJSON,
			&normName, &ir.MatchedProductID, &ir.SimilarityScore, &ir.Status, &ir.CreatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("import_row")
			}
			return err
		}
		if normName != nil {
			ir.NormalizedName = *normName
		}
		if len(rawJSON) > 0 {
			_ = json.Unmarshal(rawJSON, &ir.RawData)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ir, nil
}

// ListImportSessions returns sessions for an organization.
func (r *Repository) ListImportSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	var list []*ingest.ImportSession
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, file_upload_id, status, column_mapping,
			       min_similarity_score, total_rows, processed_rows, matched_rows,
			       error_message, started_at, completed_at, created_at, updated_at
			FROM ingest.import_sessions
			WHERE organization_id = $1
			ORDER BY id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s ingest.ImportSession
			var statusStr string
			var mappingJSON []byte
			var errMsg *string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.OrganizationID, &s.FileUploadID, &statusStr, &mappingJSON,
				&s.MinSimilarityScore, &s.TotalRows, &s.ProcessedRows, &s.MatchedRows,
				&errMsg, &s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
			); err != nil {
				return err
			}
			s.Status = ingest.SessionStatus(statusStr)
			if errMsg != nil {
				s.ErrorMessage = *errMsg
			}
			if len(mappingJSON) > 0 {
				_ = json.Unmarshal(mappingJSON, &s.ColumnMapping)
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateColumnMapping modifies column definitions.
func (r *Repository) UpdateColumnMapping(ctx context.Context, id int64, mapping map[string]string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		mappingJSON, err := json.Marshal(mapping)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `UPDATE ingest.import_sessions SET column_mapping = $1, updated_at = now() WHERE id = $2;`, mappingJSON, id)
		return err
	})
}

// UpdateSessionStatus changes the status of a session.
func (r *Repository) UpdateSessionStatus(ctx context.Context, id int64, status ingest.SessionStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE ingest.import_sessions SET status = $1, updated_at = now() WHERE id = $2;`, string(status), id)
		return err
	})
}

// UpdateImportRowMatch updates matching product result and similarity score on a staged row.
func (r *Repository) UpdateImportRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_rows
			SET matched_product_id = $2, similarity_score = $3, status = $4
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, rowID, matchedProductID, score, status)
		return err
	})
}
