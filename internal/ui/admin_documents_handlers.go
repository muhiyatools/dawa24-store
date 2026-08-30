package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminDocumentsPage redirects to the unified documents & approvals audit registry.
func (h *UIHandler) AdminDocumentsPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/approvals?tab=documents", http.StatusSeeOther)
}

// AdminCreateDocumentRequestSubmit issues an administrative document request to an organization.
func (h *UIHandler) AdminCreateDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	orgID, err := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "يرجى اختيار منشأة صالحة من القائمة.")
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	title := strings.TrimSpace(r.PostFormValue("title"))
	description := strings.TrimSpace(r.PostFormValue("description"))
	deadlineDays, _ := strconv.Atoi(r.PostFormValue("deadline_days"))
	if deadlineDays <= 0 {
		deadlineDays = 30
	}

	if title == "" {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "عنوان المستند المطلوب إلزامي.")
		return
	}

	if h.attSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "خدمة المستندات غير متاحة.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.attSvc.CreateDocumentRequest(sysCtx, actor, orgID, docType, title, description, deadlineDays); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", "تم إصدار طلب المستند الرسمي للمنشأة مع التنبيه والمهلة المحددة بنجاح.")
}

// AdminCancelDocumentRequestSubmit cancels an active document request.
func (h *UIHandler) AdminCancelDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		_ = h.attSvc.CancelDocumentRequest(sysCtx, actor, id)
	}

	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", "تم إلغاء طلب المستند.")
}

// AdminVerifyUploadedDocSubmit audits, categorizes, and approves/rejects an uploaded document.
func (h *UIHandler) AdminVerifyUploadedDocSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", "معرف المستند غير صالح.")
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	status := attachments.DocumentStatus(strings.TrimSpace(r.PostFormValue("status")))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if status != attachments.StatusVerified && status != attachments.StatusRejected {
		status = attachments.StatusVerified
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if err := h.attSvc.VerifyDocumentWithType(sysCtx, actor, id, docType, status, notes); err != nil {
			h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	msg := "تم اعتماد وتوثيق المستند وتحديث ملف المنشأة بنجاح."
	if status == attachments.StatusRejected {
		msg = "تم رفض المستند وحفظ الملاحظات."
	}
	h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "success", msg)
}
