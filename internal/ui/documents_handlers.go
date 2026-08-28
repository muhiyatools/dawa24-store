package ui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// documentsRedirect returns the actor to their documents screen after a form
// action, carrying the notice.
func (h *UIHandler) documentsRedirect(w http.ResponseWriter, r *http.Request, kind, message string) {
	target := "/customer/documents"
	if actor, ok := authctx.From(r.Context()); ok && actor.IsVendor() {
		target = "/vendor/documents"
	}
	h.redirectWithNotice(w, r, target, kind, message)
}

// OrganizationDocumentsPage renders the shared customer/vendor documents
// screen (Rebuild V2 §4.2): every document on the organization with its
// verification status and reviewer note, grouped by requirement. The shell is
// chosen by the actor's audience.
func (h *UIHandler) OrganizationDocumentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	data := &pages.OrganizationDocumentsData{}
	if h.attSvc != nil && actor.OrganizationID > 0 {
		docs, err := h.attSvc.ListByOrganization(ctx, actor.OrganizationID)
		reqs, _ := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID)
		if err != nil {
			h.log.ErrorContext(ctx, "load organization documents", "organization_id", actor.OrganizationID, "error", err)
			data.Error = "تعذر تحميل المستندات، حاول مجدداً."
		} else {
			data = pages.BuildOrganizationDocumentsData(docs, reqs, actor.IsVendor())
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.OrganizationDocuments(lang, dir, data, actor.Permissions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render organization documents", "error", err)
	}
}

// OrganizationDocumentsUploadSubmit stores a freshly uploaded file and
// registers it as a pending document on the actor's organization.
func (h *UIHandler) OrganizationDocumentsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		h.documentsRedirect(w, r, "error", "يجب تسجيل الدخول لمنشأة.")
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", "خدمة المستندات غير متاحة حالياً.")
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	if docType == "" {
		h.documentsRedirect(w, r, "error", "نوع المستند مطلوب.")
		return
	}

	url, originalName, err := saveUploadedFileMeta(r, "file", "documents")
	if err != nil {
		h.documentsRedirect(w, r, "error", "فشل رفع الملف: "+err.Error())
		return
	}
	if url == "" {
		h.documentsRedirect(w, r, "error", "يجب اختيار ملف للرفع.")
		return
	}

	uploadedDoc, err := h.attSvc.RegisterUpload(ctx, actor, docType, url, originalName)
	if err != nil {
		h.documentsRedirect(w, r, "error", h.safeMessage(err, langOf(r)))
		return
	}

	if reqIDStr := r.PostFormValue("request_id"); reqIDStr != "" && uploadedDoc != nil {
		if reqID, err := strconv.ParseInt(reqIDStr, 10, 64); err == nil && reqID > 0 {
			_ = h.attSvc.SubmitDocumentForRequest(ctx, reqID, uploadedDoc.ID)
		}
	}

	h.documentsRedirect(w, r, "success", "تم رفع المستند، وهو قيد تدقيق إدارة المنصة.")
}

// OrganizationDocumentDeleteSubmit removes a document the org owns, but only
// while it is pending — a reviewed document stays on record.
func (h *UIHandler) OrganizationDocumentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		h.documentsRedirect(w, r, "error", "يجب تسجيل الدخول.")
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", "خدمة المستندات غير متاحة حالياً.")
		return
	}

	id, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("id")), 10, 64)
	if err != nil || id <= 0 {
		h.documentsRedirect(w, r, "error", "معرف المستند غير صالح.")
		return
	}

	docs, err := h.attSvc.ListByOrganization(ctx, actor.OrganizationID)
	if err != nil {
		h.documentsRedirect(w, r, "error", "تعذر الوصول للمستند.")
		return
	}
	for _, doc := range docs {
		if doc == nil || doc.ID != id {
			continue
		}
		if doc.Status != attachments.StatusPending {
			h.documentsRedirect(w, r, "error", "لا يمكن حذف مستند سبق تدقيقه.")
			return
		}
		if err := h.attSvc.Delete(ctx, actor, id); err != nil {
			h.documentsRedirect(w, r, "error", h.safeMessage(err, langOf(r)))
			return
		}
		h.documentsRedirect(w, r, "success", "تم حذف المستند.")
		return
	}

	h.documentsRedirect(w, r, "error", "المستند غير موجود لهذه المنشأة.")
}

// saveUploadedFileMeta saves a multipart file and returns its public URL and
// the client-side file name (for the documents registry).
func saveUploadedFileMeta(r *http.Request, formKey, category string) (url, name string, err error) {
	url, err = saveUploadedFile(r, formKey, category)
	if err != nil || url == "" {
		return url, "", err
	}
	if file, header, ferr := r.FormFile(formKey); ferr == nil {
		defer file.Close()
		name = header.Filename
	}
	return url, name, nil
}

// DocumentViewHandler streams or redirects to the document file for in-browser viewing / preview.
func (h *UIHandler) DocumentViewHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, false)
}

// DocumentDownloadHandler forces downloading the document file.
func (h *UIHandler) DocumentDownloadHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, true)
}

// serveDocumentFile safely finds, verifies access to, and streams a document file.
func (h *UIHandler) serveDocumentFile(w http.ResponseWriter, r *http.Request, download bool) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "يجب تسجيل الدخول لعرض المستند", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "معرف المستند غير صالح", http.StatusBadRequest)
		return
	}

	if h.attSvc == nil {
		http.Error(w, "خدمة المستندات غير متاحة", http.StatusServiceUnavailable)
		return
	}

	doc, err := h.attSvc.GetDocumentByID(ctx, actor, id)
	if err != nil {
		if actor.IsPlatformAdmin() || actor.IsStaff {
			doc, err = h.attSvc.GetByIDAdmin(ctx, id)
		}
		if err != nil || doc == nil {
			http.Error(w, "المستند غير موجود أو ليس لديك صلاحية لعرضه", http.StatusForbidden)
			return
		}
	}

	fileURL := strings.TrimSpace(doc.FileURL)
	if fileURL == "" {
		http.Error(w, "ملف المستند غير موجود", http.StatusNotFound)
		return
	}

	// 1. If it's a remote URL (http:// or https://), redirect directly
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") {
		http.Redirect(w, r, fileURL, http.StatusTemporaryRedirect)
		return
	}

	// 2. Check local disk locations
	cleanPath := strings.TrimPrefix(fileURL, "/uploads/")
	candidates := []string{
		filepath.Join("data", "uploads", cleanPath),
		filepath.Join(UploadBaseDir, cleanPath),
		filepath.Join("data", fileURL),
		fileURL,
		filepath.Join(UploadBaseDir, "documents", filepath.Base(fileURL)),
		filepath.Join(UploadBaseDir, "licenses", filepath.Base(fileURL)),
	}

	for _, cand := range candidates {
		info, statErr := os.Stat(cand)
		if statErr == nil && !info.IsDir() {
			f, openErr := os.Open(cand)
			if openErr == nil {
				defer f.Close()

				// Determine mime type
				mimeType := doc.MimeType
				if mimeType == "" {
					ext := strings.ToLower(filepath.Ext(cand))
					switch ext {
					case ".pdf":
						mimeType = "application/pdf"
					case ".png":
						mimeType = "image/png"
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".webp":
						mimeType = "image/webp"
					default:
						mimeType = "application/octet-stream"
					}
				}

				filename := doc.OriginalName
				if filename == "" {
					filename = filepath.Base(cand)
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
				w.Header().Set("Cache-Control", "private, max-age=3600")
				http.ServeContent(w, r, filename, info.ModTime(), f)
				return
			}
		}
	}

	// 3. If storage client (S3/MinIO) is configured, get presigned URL or redirect
	if h.storage != nil {
		presigned, presignErr := h.storage.PresignGet(ctx, fileURL, 60*time.Minute)
		if presignErr == nil && presigned != "" {
			http.Redirect(w, r, presigned, http.StatusTemporaryRedirect)
			return
		}
	}

	http.Error(w, "تعذر العثور على ملف المستند على الخادم", http.StatusNotFound)
}
