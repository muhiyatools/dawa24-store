package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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

// OrganizationDocumentsPage renders the customer/vendor legal documents & licensing screen.
func (h *UIHandler) OrganizationDocumentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}

	data := &pages.OrganizationDocumentsData{}
	if h.attSvc != nil && orgID > 0 {
		sysCtx := database.AsSystem(ctx)
		docs, err := h.attSvc.ListByOrganization(sysCtx, orgID)
		reqs, _ := h.attSvc.ListDocumentRequests(sysCtx, actor, &orgID)
		if err != nil {
			h.log.ErrorContext(ctx, "load organization documents", "organization_id", orgID, "error", err)
			data.Error = "تعذر تحميل المستندات، حاول مجدداً."
		} else {
			data = pages.BuildOrganizationDocumentsData(docs, reqs, actor.IsVendor())
		}
	}

	h.renderPage(ctx, w, "render organization documents", pages.OrganizationDocuments(lang, dir, data, actor.Permissions))
}

// OrganizationDocumentsUploadSubmit stores a freshly uploaded file and
// registers it as a pending document on the actor's organization.
func (h *UIHandler) OrganizationDocumentsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || (orgID <= 0 && !actor.IsPlatformAdmin()) {
		h.documentsRedirect(w, r, "error", "يجب تسجيل الدخول لحساب منشأة معتمدة.")
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", "خدمة المستندات غير متاحة حالياً.")
		return
	}

	docTypeStr := strings.TrimSpace(r.PostFormValue("document_type"))
	if docTypeStr == "" {
		docTypeStr = strings.TrimSpace(r.FormValue("document_type"))
	}
	docType := attachments.DocumentType(docTypeStr)
	if docType == "" {
		h.documentsRedirect(w, r, "error", "يرجى تحديد نوع المستند المراد رفعه.")
		return
	}

	// Try standard field "file", then fallback to "document_file" or "file_upload"
	formKeys := []string{"file", "document_file", "file_upload", "doc"}
	var fileURL, originalName string
	var uploadErr error

	for _, k := range formKeys {
		fileURL, originalName, uploadErr = saveUploadedFileMeta(r, k, "documents")
		if uploadErr == nil && fileURL != "" {
			break
		}
	}

	if fileURL == "" {
		errMsg := "يجب اختيار ملف المستند للرفع."
		if uploadErr != nil {
			errMsg = "فشل رفع الملف: " + uploadErr.Error()
		}
		h.documentsRedirect(w, r, "error", errMsg)
		return
	}

	sysCtx := database.AsSystem(ctx)
	uploadedDoc, err := h.attSvc.RegisterUpload(sysCtx, actor, docType, fileURL, originalName)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to register document upload", "error", err)
		h.documentsRedirect(w, r, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// If replacement reason is provided, attach it as a note for platform admins
	replacementReason := strings.TrimSpace(r.PostFormValue("replacement_reason"))
	if replacementReason != "" && uploadedDoc != nil {
		adminActor := authctx.Actor{IsStaff: true, Role: "admin"}
		_ = h.attSvc.VerifyDocument(sysCtx, adminActor, uploadedDoc.ID, attachments.StatusPending, fmt.Sprintf("سبب استبدال المستند: %s", replacementReason))
	}

	// If linked to an administrative document request, fulfill or submit it
	reqIDStr := strings.TrimSpace(r.PostFormValue("request_id"))
	if reqIDStr != "" && uploadedDoc != nil {
		if reqID, err := strconv.ParseInt(reqIDStr, 10, 64); err == nil && reqID > 0 {
			_ = h.attSvc.SubmitDocumentForRequest(sysCtx, reqID, uploadedDoc.ID)
		}
	}

	if replacementReason != "" {
		h.documentsRedirect(w, r, "success", "تم استبدال وتحديث المستند بنجاح، وهو الآن قيد تدقيق واعتماد إدارة المنصة.")
	} else {
		h.documentsRedirect(w, r, "success", "تم رفع المستند بنجاح وهو الآن قيد تدقيق إدارة المنصة.")
	}
}

// OrganizationDocumentDeleteSubmit removes a document, restricted strictly to platform administrators.
func (h *UIHandler) OrganizationDocumentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsPlatformAdmin() {
		h.documentsRedirect(w, r, "error", "عفواً، لا يمكن حذف المستندات الرسمية المرفوعة إلا من خلال إدارة المنصة حصراً.")
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

	sysCtx := database.AsSystem(ctx)
	if err := h.attSvc.Delete(sysCtx, actor, id); err != nil {
		h.documentsRedirect(w, r, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.documentsRedirect(w, r, "success", "تم حذف المستند بنجاح.")
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
