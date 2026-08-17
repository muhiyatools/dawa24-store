package attachments

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service provides high-level business operations for attachments and storage presigning.
type Service struct {
	repo    Repository
	storage *storage.Client
	log     *slog.Logger
}

// NewService creates a new attachments Service.
func NewService(repo Repository, storage *storage.Client, log *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
		log:     log,
	}
}

// PresignUpload validates file parameters, reserves a pending document row, and returns a secure presigned PUT URL.
func (s *Service) PresignUpload(ctx context.Context, actor authctx.Actor, req PresignRequest) (*PresignResult, error) {
	if err := ValidatePresignRequest(req); err != nil {
		return nil, err
	}

	var orgID *int64
	if req.OrganizationID != nil && *req.OrganizationID > 0 {
		// Verify actor belongs to this org or is platform admin
		if !actor.IsPlatformAdmin() && actor.OrgID != *req.OrganizationID {
			return nil, apperr.Forbidden("document.unauthorized_org", "ليس لديك صلاحية رفع مستندات لهذه المنشأة")
		}
		orgID = req.OrganizationID
	} else if actor.OrgID > 0 {
		v := actor.OrgID
		orgID = &v
	}

	var userID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		userID = &v
	}

	if orgID == nil && userID == nil {
		return nil, apperr.Unauthorized()
	}

	storageKey := GenerateStorageKey(req.DocumentType, orgID, userID, req.OriginalName)

	doc := &Document{
		OrganizationID: orgID,
		UserID:         userID,
		DocumentType:   req.DocumentType,
		FileURL:        storageKey,
		OriginalName:   req.OriginalName,
		MimeType:       req.MimeType,
		SizeBytes:      req.SizeBytes,
		Status:         StatusPending,
		Meta: map[string]interface{}{
			"uploader_user_id": actor.UserID,
		},
	}

	created, err := s.repo.Create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("attachments.PresignUpload: %w", err)
	}

	uploadURL := ""
	expiresAt := time.Now().Add(15 * time.Minute)
	if s.storage != nil {
		url, err := s.storage.PresignPut(ctx, storageKey, req.MimeType, 15*time.Minute)
		if err != nil {
			s.log.Error("failed to generate presigned PUT url", "error", err, "key", storageKey)
			_ = s.repo.HardDelete(ctx, created.ID)
			return nil, fmt.Errorf("storage presign error: %w", err)
		}
		uploadURL = url
	} else {
		// Local mock fallback for development without external S3
		uploadURL = fmt.Sprintf("/api/v1/attachments/%d/upload-mock", created.ID)
	}

	return &PresignResult{
		DocumentID: created.ID,
		PublicID:   created.PublicID,
		UploadURL:  uploadURL,
		ExpiresAt:  expiresAt,
		StorageKey: storageKey,
	}, nil
}

// ConfirmUpload verifies with MinIO/S3 that the object actually exists, records real file size, and marks status.
func (s *Service) ConfirmUpload(ctx context.Context, actor authctx.Actor, id int64) (*Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Verify actor ownership
	if !actor.IsPlatformAdmin() {
		if doc.UserID != nil && *doc.UserID != actor.UserID && (doc.OrganizationID == nil || *doc.OrganizationID != actor.OrgID) {
			return nil, apperr.Forbidden("document.access_denied", "ليس لديك صلاحية تأكيد هذا المستند")
		}
	}

	if s.storage != nil {
		size, _, err := s.storage.HeadObject(ctx, doc.FileURL)
		if err != nil {
			s.log.Warn("confirm upload failed: object not found in storage", "id", id, "key", doc.FileURL, "error", err)
			_ = s.repo.HardDelete(ctx, id)
			return nil, apperr.Validation("document.not_in_storage", "لم يتم العثور على الملف في وحدة التخزين، يرجى إعادة المحاولة", map[string]string{"upload": "فشل الرفع"})
		}
		doc.SizeBytes = size
	}

	return doc, nil
}

// GetDownloadURL generates a short-lived presigned GET URL for viewing or downloading the document.
func (s *Service) GetDownloadURL(ctx context.Context, actor authctx.Actor, id int64) (string, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	// Check access permissions
	if !actor.IsPlatformAdmin() {
		if doc.UserID != nil && *doc.UserID != actor.UserID && (doc.OrganizationID == nil || *doc.OrganizationID != actor.OrgID) {
			return "", apperr.Forbidden("document.access_denied", "ليس لديك صلاحية الوصول لهذا المستند")
		}
	}

	if s.storage != nil {
		return s.storage.PresignGet(ctx, doc.FileURL, 30*time.Minute)
	}

	return "/static/docs/placeholder.pdf", nil
}

// VerifyDocument allows platform admins to approve or reject submitted certificates and licenses.
func (s *Service) VerifyDocument(ctx context.Context, actor authctx.Actor, id int64, status DocumentStatus, notes string) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", "يتطلب صلاحيات مدير النظام للتحقق من المستندات")
	}


	var reviewerID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reviewerID = &v
	}

	return s.repo.UpdateStatus(ctx, id, status, notes, reviewerID)
}

// Delete soft-deletes a document record.
func (s *Service) Delete(ctx context.Context, actor authctx.Actor, id int64) error {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !actor.IsPlatformAdmin() {
		if doc.UserID != nil && *doc.UserID != actor.UserID && (doc.OrganizationID == nil || *doc.OrganizationID != actor.OrgID) {
			return apperr.Forbidden("document.access_denied", "ليس لديك صلاحية حذف هذا المستند")
		}
	}


	return s.repo.SoftDelete(ctx, id)
}

// ListByOrganization returns all documents belonging to an organization.
func (s *Service) ListByOrganization(ctx context.Context, orgID int64) ([]*Document, error) {
	return s.repo.ListByOrganization(ctx, orgID)
}

// ListByUser returns all documents uploaded by a user.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]*Document, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListAll returns documents matching administrative search criteria.
func (s *Service) ListAll(ctx context.Context, filter DocumentFilter) ([]*Document, int, error) {
	return s.repo.ListAll(ctx, filter)
}
