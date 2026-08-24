package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type Repository struct {
	db *database.DB
}

// NewRepository creates a new PostgreSQL repository for platform_admin.documents.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, doc *attachments.Document) (*attachments.Document, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.documents (
				organization_id, user_id, document_type, file_url, original_name,
				mime_type, size_bytes, status, review_notes, meta
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		metaBytes := doc.MetaJSON()
		return tx.QueryRow(txCtx, query,
			doc.OrganizationID,
			doc.UserID,
			string(doc.DocumentType),
			doc.FileURL,
			doc.OriginalName,
			doc.MimeType,
			doc.SizeBytes,
			string(doc.Status),
			doc.ReviewNotes,
			metaBytes,
		).Scan(&doc.ID, &doc.PublicID, &doc.CreatedAt, &doc.UpdatedAt)
	})

	if err != nil {
		return nil, fmt.Errorf("documents.Create: %w", err)
	}
	return doc, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*attachments.Document, error) {
	var doc attachments.Document
	var docType, status string
	var metaBytes []byte

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, document_type, file_url,
			       original_name, mime_type, size_bytes, status, review_notes,
			       reviewed_by, reviewed_at, meta, created_at, updated_at, deleted_at
			FROM platform_admin.documents
			WHERE id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&doc.ID, &doc.PublicID, &doc.OrganizationID, &doc.UserID, &docType, &doc.FileURL,
			&doc.OriginalName, &doc.MimeType, &doc.SizeBytes, &status, &doc.ReviewNotes,
			&doc.ReviewedBy, &doc.ReviewedAt, &metaBytes, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
		)
	})

	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("document")
		}
		return nil, fmt.Errorf("documents.GetByID: %w", err)
	}

	doc.DocumentType = attachments.DocumentType(docType)
	doc.Status = attachments.DocumentStatus(status)
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &doc.Meta)
	}
	return &doc, nil
}

func (r *Repository) GetByPublicID(ctx context.Context, publicID uuid.UUID) (*attachments.Document, error) {
	var doc attachments.Document
	var docType, status string
	var metaBytes []byte

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, document_type, file_url,
			       original_name, mime_type, size_bytes, status, review_notes,
			       reviewed_by, reviewed_at, meta, created_at, updated_at, deleted_at
			FROM platform_admin.documents
			WHERE public_id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, publicID).Scan(
			&doc.ID, &doc.PublicID, &doc.OrganizationID, &doc.UserID, &docType, &doc.FileURL,
			&doc.OriginalName, &doc.MimeType, &doc.SizeBytes, &status, &doc.ReviewNotes,
			&doc.ReviewedBy, &doc.ReviewedAt, &metaBytes, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
		)
	})

	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("document")
		}
		return nil, fmt.Errorf("documents.GetByPublicID: %w", err)
	}

	doc.DocumentType = attachments.DocumentType(docType)
	doc.Status = attachments.DocumentStatus(status)
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &doc.Meta)
	}
	return &doc, nil
}

func (r *Repository) ListByOrganization(ctx context.Context, orgID int64) ([]*attachments.Document, error) {
	var list []*attachments.Document
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, document_type, file_url,
			       original_name, mime_type, size_bytes, status, review_notes,
			       reviewed_by, reviewed_at, meta, created_at, updated_at, deleted_at
			FROM platform_admin.documents
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var doc attachments.Document
			var docType, status string
			var metaBytes []byte
			if err := rows.Scan(
				&doc.ID, &doc.PublicID, &doc.OrganizationID, &doc.UserID, &docType, &doc.FileURL,
				&doc.OriginalName, &doc.MimeType, &doc.SizeBytes, &status, &doc.ReviewNotes,
				&doc.ReviewedBy, &doc.ReviewedAt, &metaBytes, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
			); err != nil {
				return err
			}
			doc.DocumentType = attachments.DocumentType(docType)
			doc.Status = attachments.DocumentStatus(status)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &doc.Meta)
			}
			list = append(list, &doc)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListByUser(ctx context.Context, userID int64) ([]*attachments.Document, error) {
	var list []*attachments.Document
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, document_type, file_url,
			       original_name, mime_type, size_bytes, status, review_notes,
			       reviewed_by, reviewed_at, meta, created_at, updated_at, deleted_at
			FROM platform_admin.documents
			WHERE user_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var doc attachments.Document
			var docType, status string
			var metaBytes []byte
			if err := rows.Scan(
				&doc.ID, &doc.PublicID, &doc.OrganizationID, &doc.UserID, &docType, &doc.FileURL,
				&doc.OriginalName, &doc.MimeType, &doc.SizeBytes, &status, &doc.ReviewNotes,
				&doc.ReviewedBy, &doc.ReviewedAt, &metaBytes, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
			); err != nil {
				return err
			}
			doc.DocumentType = attachments.DocumentType(docType)
			doc.Status = attachments.DocumentStatus(status)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &doc.Meta)
			}
			list = append(list, &doc)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListAll(ctx context.Context, filter attachments.DocumentFilter) ([]*attachments.Document, int, error) {
	var list []*attachments.Document
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := ` FROM platform_admin.documents WHERE deleted_at IS NULL `
		args := []interface{}{}
		argIdx := 1

		if filter.OrganizationID != nil {
			baseQuery += fmt.Sprintf(" AND organization_id = $%d", argIdx)
			args = append(args, *filter.OrganizationID)
			argIdx++
		}
		if filter.UserID != nil {
			baseQuery += fmt.Sprintf(" AND user_id = $%d", argIdx)
			args = append(args, *filter.UserID)
			argIdx++
		}
		if filter.DocumentType != nil {
			baseQuery += fmt.Sprintf(" AND document_type = $%d", argIdx)
			args = append(args, string(*filter.DocumentType))
			argIdx++
		}
		if filter.Status != nil {
			baseQuery += fmt.Sprintf(" AND status = $%d", argIdx)
			args = append(args, string(*filter.Status))
			argIdx++
		}
		if filter.Search != "" {
			baseQuery += fmt.Sprintf(" AND (original_name ILIKE $%d OR file_url ILIKE $%d)", argIdx, argIdx)
			args = append(args, "%"+filter.Search+"%")
			argIdx++
		}

		countQuery := "SELECT COUNT(*)" + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		selectQuery := `
			SELECT id, public_id, organization_id, user_id, document_type, file_url,
			       original_name, mime_type, size_bytes, status, review_notes,
			       reviewed_by, reviewed_at, meta, created_at, updated_at, deleted_at
		` + baseQuery + fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d;", limit, offset)

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var doc attachments.Document
			var docType, status string
			var metaBytes []byte
			if err := rows.Scan(
				&doc.ID, &doc.PublicID, &doc.OrganizationID, &doc.UserID, &docType, &doc.FileURL,
				&doc.OriginalName, &doc.MimeType, &doc.SizeBytes, &status, &doc.ReviewNotes,
				&doc.ReviewedBy, &doc.ReviewedAt, &metaBytes, &doc.CreatedAt, &doc.UpdatedAt, &doc.DeletedAt,
			); err != nil {
				return err
			}
			doc.DocumentType = attachments.DocumentType(docType)
			doc.Status = attachments.DocumentStatus(status)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &doc.Meta)
			}
			list = append(list, &doc)
		}
		return rows.Err()
	})

	return list, total, err
}

func (r *Repository) UpdateStatus(ctx context.Context, id int64, status attachments.DocumentStatus, notes string, reviewedBy *int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.documents
			SET status = $1, review_notes = $2, reviewed_by = $3, reviewed_at = now(), updated_at = now()
			WHERE id = $4 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query, string(status), notes, reviewedBy, id)
		if err != nil {
			return fmt.Errorf("documents.UpdateStatus: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("document")
		}
		return nil
	})
}

func (r *Repository) UpdateTypeAndStatus(ctx context.Context, id int64, docType attachments.DocumentType, status attachments.DocumentStatus, notes string, reviewedBy *int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.documents
			SET document_type = $1, status = $2, review_notes = $3, reviewed_by = $4, reviewed_at = now(), updated_at = now()
			WHERE id = $5 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query, string(docType), string(status), notes, reviewedBy, id)
		if err != nil {
			return fmt.Errorf("documents.UpdateTypeAndStatus: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("document")
		}
		return nil
	})
}

func (r *Repository) SoftDelete(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.documents SET deleted_at = now(), updated_at = now() WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

func (r *Repository) HardDelete(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM platform_admin.documents WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

func (r *Repository) CreateDocumentRequest(ctx context.Context, req *attachments.DocumentRequest) (*attachments.DocumentRequest, error) {
	var out attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.document_requests (
				organization_id, requested_by, document_type, title, description, deadline_at, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
			RETURNING id, organization_id, requested_by, document_type, title, description, deadline_at, status, submitted_doc_id, created_at, updated_at;
		`
		var st string
		if req.Status == "" {
			st = string(attachments.DocReqPending)
		} else {
			st = string(req.Status)
		}
		var dt string = string(req.DocumentType)
		var subDocID *int64
		err := tx.QueryRow(txCtx, query,
			req.OrganizationID, req.RequestedBy, dt, req.Title, req.Description, req.DeadlineAt, st,
		).Scan(
			&out.ID, &out.OrganizationID, &out.RequestedBy, &dt, &out.Title, &out.Description, &out.DeadlineAt, &st, &subDocID, &out.CreatedAt, &out.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("document_requests.Create: %w", err)
		}
		out.DocumentType = attachments.DocumentType(dt)
		out.Status = attachments.DocumentRequestStatus(st)
		out.SubmittedDocID = subDocID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Repository) ListRequestsByOrg(ctx context.Context, orgID int64) ([]*attachments.DocumentRequest, error) {
	var list []*attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT dr.id, dr.organization_id, COALESCE(o.legal_name, ''), dr.requested_by, dr.document_type,
			       dr.title, dr.description, dr.deadline_at, dr.status, dr.submitted_doc_id, dr.created_at, dr.updated_at
			FROM platform_admin.document_requests dr
			LEFT JOIN org.organizations o ON o.id = dr.organization_id
			WHERE dr.organization_id = $1
			ORDER BY dr.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return fmt.Errorf("document_requests.ListByOrg: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item attachments.DocumentRequest
			var dt, st string
			var subDocID *int64
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.OrgName, &item.RequestedBy, &dt,
				&item.Title, &item.Description, &item.DeadlineAt, &st, &subDocID, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return fmt.Errorf("document_requests.Scan: %w", err)
			}
			item.DocumentType = attachments.DocumentType(dt)
			item.Status = attachments.DocumentRequestStatus(st)
			item.SubmittedDocID = subDocID
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListAllRequests(ctx context.Context) ([]*attachments.DocumentRequest, error) {
	var list []*attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT dr.id, dr.organization_id, COALESCE(o.legal_name, ''), dr.requested_by, dr.document_type,
			       dr.title, dr.description, dr.deadline_at, dr.status, dr.submitted_doc_id, dr.created_at, dr.updated_at
			FROM platform_admin.document_requests dr
			LEFT JOIN org.organizations o ON o.id = dr.organization_id
			ORDER BY dr.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return fmt.Errorf("document_requests.ListAll: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item attachments.DocumentRequest
			var dt, st string
			var subDocID *int64
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.OrgName, &item.RequestedBy, &dt,
				&item.Title, &item.Description, &item.DeadlineAt, &st, &subDocID, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return fmt.Errorf("document_requests.Scan: %w", err)
			}
			item.DocumentType = attachments.DocumentType(dt)
			item.Status = attachments.DocumentRequestStatus(st)
			item.SubmittedDocID = subDocID
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) UpdateRequestStatus(ctx context.Context, id int64, status attachments.DocumentRequestStatus, submittedDocID *int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.document_requests
			SET status = $1, submitted_doc_id = COALESCE($2, submitted_doc_id), updated_at = now()
			WHERE id = $3;
		`
		tag, err := tx.Exec(txCtx, query, string(status), submittedDocID, id)
		if err != nil {
			return fmt.Errorf("document_requests.UpdateStatus: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("document_request")
		}
		return nil
	})
}

func (r *Repository) FulfillRequestByDoc(ctx context.Context, orgID int64, docType attachments.DocumentType, docID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.document_requests
			SET status = 'fulfilled', submitted_doc_id = $1, updated_at = now()
			WHERE organization_id = $2
			  AND (document_type = $3 OR document_type = '' OR document_type = 'other')
			  AND status IN ('pending', 'submitted');
		`
		_, err := tx.Exec(txCtx, query, docID, orgID, string(docType))
		return err
	})
}
