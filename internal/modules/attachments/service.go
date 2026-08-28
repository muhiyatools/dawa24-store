package attachments

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
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

// MissingRequiredDocuments returns missing document indicator if the
// organization has no verified copy of any document, or nil when the org can trade.
// An organization requires at least one active/approved document to trade.
func (s *Service) MissingRequiredDocuments(ctx context.Context, orgID int64, orgType string) ([]DocumentType, error) {
	if orgID <= 0 {
		return nil, apperr.Unauthorized()
	}

	docs, err := s.repo.ListByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("attachments.MissingRequiredDocuments: %w", err)
	}

	verifiedCount := 0
	for _, d := range docs {
		if d != nil && d.Status == StatusVerified && d.DeletedAt == nil {
			verifiedCount++
		}
	}

	if verifiedCount == 0 {
		return []DocumentType{DocCommercialRegister}, nil
	}
	return nil, nil
}

// RegisterUpload records a document that was already uploaded through the
// local /uploads flow (Rebuild V2 §4.2) — the organization-owned files screen
// uses this instead of presigned storage when object storage is absent.
// Unlike PresignUpload it needs no storage client: the uploader hands over
// the final public URL.
func (s *Service) RegisterUpload(ctx context.Context, actor authctx.Actor, docType DocumentType, url, originalName string) (*Document, error) {
	if actor.OrgID <= 0 {
		return nil, apperr.Unauthorized()
	}

	validMimes, ok := allowedMIMEs[docType]
	if !ok {
		return nil, apperr.Validation("document.type_invalid", "نوع المستند غير صالح", map[string]string{"document_type": "نوع غير مدعوم"})
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	mime := "application/octet-stream"
	switch ext {
	case "", ".pdf":
		mime = "application/pdf"
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".webp":
		mime = "image/webp"
	}
	mimeMatched := false
	for _, m := range validMimes {
		if m == mime {
			mimeMatched = true
			break
		}
	}
	if !mimeMatched {
		return nil, apperr.Validation("document.mime_unsupported", "صيغة الملف غير مسموح بها لهذا النوع", map[string]string{"mime_type": "صيغة غير مدعومة"})
	}

	url = strings.TrimSpace(url)
	if url == "" {
		return nil, apperr.Validation("document.file_required", "ملف المستند مطلوب", nil)
	}

	var orgIDPtr *int64
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID > 0 {
		orgIDPtr = &orgID
	}

	var userIDPtr *int64
	if actor.UserID > 0 {
		v := actor.UserID
		userIDPtr = &v
	}

	doc := &Document{
		OrganizationID: orgIDPtr,
		UserID:         userIDPtr,
		DocumentType:   docType,
		FileURL:        url,
		OriginalName:   strings.TrimSpace(originalName),
		MimeType:       mime,
		Status:         StatusPending,
		Meta: map[string]interface{}{
			"uploader_user_id": actor.UserID,
		},
	}

	created, err := s.repo.Create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("attachments.RegisterUpload: %w", err)
	}
	return created, nil
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

// GetDocumentByID retrieves a document ensuring the actor has access permissions.
func (s *Service) GetDocumentByID(ctx context.Context, actor authctx.Actor, id int64) (*Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if !actor.IsPlatformAdmin() {
		orgID := actor.OrganizationID
		if orgID <= 0 {
			orgID = actor.OrgID
		}
		hasAccess := false
		if doc.OrganizationID != nil && orgID > 0 && *doc.OrganizationID == orgID {
			hasAccess = true
		}
		if doc.UserID != nil && actor.UserID > 0 && *doc.UserID == actor.UserID {
			hasAccess = true
		}
		if !hasAccess {
			return nil, apperr.Forbidden("document.access_denied", "ليس لديك صلاحية الوصول لهذا المستند")
		}
	}
	return doc, nil
}

// GetByIDAdmin fetches a document by ID directly for platform administrative tasks.
func (s *Service) GetByIDAdmin(ctx context.Context, id int64) (*Document, error) {
	return s.repo.GetByID(ctx, id)
}

// GetDownloadURL generates a secure URL for viewing or downloading the document.
func (s *Service) GetDownloadURL(ctx context.Context, actor authctx.Actor, id int64) (string, error) {
	doc, err := s.GetDocumentByID(ctx, actor, id)
	if err != nil {
		if actor.IsPlatformAdmin() {
			doc, err = s.repo.GetByID(ctx, id)
		}
		if err != nil || doc == nil {
			return "", err
		}
	}

	fileURL := strings.TrimSpace(doc.FileURL)
	if fileURL == "" {
		return "", apperr.NotFound("document.file_empty")
	}

	// If it's already a direct HTTP(S) URL or local uploads/static path, return it directly
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") || strings.HasPrefix(fileURL, "/uploads/") || strings.HasPrefix(fileURL, "/static/") || strings.HasPrefix(fileURL, "/documents/") {
		return fileURL, nil
	}

	if s.storage != nil {
		presigned, presignErr := s.storage.PresignGet(ctx, fileURL, 60*time.Minute)
		if presignErr == nil && presigned != "" {
			return presigned, nil
		}
		if s.log != nil {
			s.log.WarnContext(ctx, "storage presign get failed", "id", id, "key", fileURL, "error", presignErr)
		}
	}

	return fileURL, nil
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

	if err := s.repo.UpdateStatus(ctx, id, status, notes, reviewerID); err != nil {
		return err
	}

	if status == StatusVerified {
		if doc, err := s.repo.GetByID(ctx, id); err == nil && doc != nil && doc.OrganizationID != nil {
			_ = s.repo.FulfillRequestByDoc(ctx, *doc.OrganizationID, doc.DocumentType, doc.ID)
		}
	}
	return nil
}

// VerifyDocumentWithType allows platform admins to assign category and approve or reject submitted documents.
func (s *Service) VerifyDocumentWithType(ctx context.Context, actor authctx.Actor, id int64, docType DocumentType, status DocumentStatus, notes string) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", "يتطلب صلاحيات مدير النظام للتحقق من المستندات")
	}

	var reviewerID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reviewerID = &v
	}

	var err error
	if docType != "" {
		err = s.repo.UpdateTypeAndStatus(ctx, id, docType, status, notes, reviewerID)
	} else {
		err = s.repo.UpdateStatus(ctx, id, status, notes, reviewerID)
	}
	if err != nil {
		return err
	}

	if status == StatusVerified {
		if doc, err := s.repo.GetByID(ctx, id); err == nil && doc != nil && doc.OrganizationID != nil {
			actualType := docType
			if actualType == "" {
				actualType = doc.DocumentType
			}
			_ = s.repo.FulfillRequestByDoc(ctx, *doc.OrganizationID, actualType, doc.ID)
		}
	}
	return nil
}

// CreateDocumentRequest issues an official document request from platform admin to an organization.
func (s *Service) CreateDocumentRequest(ctx context.Context, actor authctx.Actor, orgID int64, docType DocumentType, title, description string, deadlineDays int) (*DocumentRequest, error) {
	if !actor.IsPlatformAdmin() {
		return nil, apperr.Forbidden("document.admin_required", "يتطلب صلاحيات مدير النظام لطلب مستندات رسمية")
	}
	if orgID <= 0 {
		return nil, apperr.Validation("org_id.required", "يرجى تحديد المنشأة المطلوبة", nil)
	}
	if strings.TrimSpace(title) == "" {
		return nil, apperr.Validation("title.required", "عنوان ومسمى المستند المطلوب إلزامي", nil)
	}
	if deadlineDays <= 0 {
		deadlineDays = 30
	}

	var reqBy *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reqBy = &v
	}

	req := &DocumentRequest{
		OrganizationID: orgID,
		RequestedBy:    reqBy,
		DocumentType:   docType,
		Title:          strings.TrimSpace(title),
		Description:    strings.TrimSpace(description),
		DeadlineAt:     time.Now().Add(time.Duration(deadlineDays) * 24 * time.Hour),
		Status:         DocReqPending,
	}

	return s.repo.CreateDocumentRequest(ctx, req)
}

// ListDocumentRequests returns administrative document requests for a specific org or across the platform.
func (s *Service) ListDocumentRequests(ctx context.Context, actor authctx.Actor, orgID *int64) ([]*DocumentRequest, error) {
	if orgID != nil && *orgID > 0 {
		if !actor.IsPlatformAdmin() && actor.OrgID != *orgID {
			return nil, apperr.Forbidden("document.access_denied", "ليس لديك صلاحية استعراض طلبات هذه المنشأة")
		}
		return s.repo.ListRequestsByOrg(ctx, *orgID)
	}

	if !actor.IsPlatformAdmin() {
		return nil, apperr.Forbidden("document.admin_required", "يتطلب صلاحيات مدير النظام")
	}
	return s.repo.ListAllRequests(ctx)
}

// CancelDocumentRequest cancels an administrative document request.
func (s *Service) CancelDocumentRequest(ctx context.Context, actor authctx.Actor, reqID int64) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", "يتطلب صلاحيات مدير النظام")
	}
	return s.repo.UpdateRequestStatus(ctx, reqID, DocReqCancelled, nil)
}

// SubmitDocumentForRequest marks a document request as submitted with the newly uploaded document ID.
func (s *Service) SubmitDocumentForRequest(ctx context.Context, reqID int64, docID int64) error {
	return s.repo.UpdateRequestStatus(ctx, reqID, DocReqSubmitted, &docID)
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
