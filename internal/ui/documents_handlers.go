package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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
			data.Error = i18n.T(lang, "docs.load_failed")
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
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || (orgID <= 0 && !actor.IsPlatformAdmin()) {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.org_auth_required"))
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.service_unavailable"))
		return
	}

	docTypeStr := strings.TrimSpace(r.PostFormValue("document_type"))
	if docTypeStr == "" {
		docTypeStr = strings.TrimSpace(r.FormValue("document_type"))
	}
	docType := attachments.DocumentType(docTypeStr)
	if docType == "" {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.type_required"))
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
		errMsg := i18n.T(lang, "docs.file_required")
		if uploadErr != nil {
			errMsg = fmt.Sprintf(i18n.T(lang, "docs.upload_failed_prefix"), uploadErr.Error())
		}
		h.documentsRedirect(w, r, "error", errMsg)
		return
	}

	sysCtx := database.AsSystem(ctx)
	uploadedDoc, err := h.attSvc.RegisterUpload(sysCtx, actor, docType, fileURL, originalName)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to register document upload", "error", err)
		h.documentsRedirect(w, r, "error", h.safeMessage(err, lang))
		return
	}

	// If replacement reason is provided, attach it as a note for platform admins
	replacementReason := strings.TrimSpace(r.PostFormValue("replacement_reason"))
	if replacementReason != "" && uploadedDoc != nil {
		adminActor := authctx.Actor{IsStaff: true, Role: "admin"}
		_ = h.attSvc.VerifyDocument(sysCtx, adminActor, uploadedDoc.ID, attachments.StatusPending, fmt.Sprintf(i18n.T(lang, "docs.replacement_reason_note"), replacementReason))
	}

	// If linked to an administrative document request, fulfill or submit it
	reqIDStr := strings.TrimSpace(r.PostFormValue("request_id"))
	if reqIDStr != "" && uploadedDoc != nil {
		if reqID, err := strconv.ParseInt(reqIDStr, 10, 64); err == nil && reqID > 0 {
			_ = h.attSvc.SubmitDocumentForRequest(sysCtx, reqID, uploadedDoc.ID)
		}
	}

	if replacementReason != "" {
		h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.replaced_success"))
	} else {
		h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.uploaded_success"))
	}
}

// OrganizationDocumentDeleteSubmit removes a document, restricted strictly to platform administrators.
func (h *UIHandler) OrganizationDocumentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsPlatformAdmin() {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.delete_admin_only"))
		return
	}
	if h.attSvc == nil {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("id")), 10, 64)
	if err != nil || id <= 0 {
		h.documentsRedirect(w, r, "error", i18n.T(lang, "docs.invalid_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.attSvc.Delete(sysCtx, actor, id); err != nil {
		h.documentsRedirect(w, r, "error", h.safeMessage(err, lang))
		return
	}

	h.documentsRedirect(w, r, "success", i18n.T(lang, "docs.deleted_success"))
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
