package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ContactPage renders the public contact form.
func (h *UIHandler) ContactPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ContactPage(lang, dir, false).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render contact page", "error", err)
	}
}

// ContactSubmit records a public contact inquiry and re-renders the form with a
// success state that keeps the visitor on the page.
func (h *UIHandler) ContactSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	_ = r.ParseForm()

	submitted := false
	if h.adminSvc != nil {
		err := h.adminSvc.SubmitContactMessage(database.AsSystem(ctx), &platformadmin.ContactMessage{
			Name:    strings.TrimSpace(r.FormValue("name")),
			Email:   strings.TrimSpace(r.FormValue("email")),
			Phone:   strings.TrimSpace(r.FormValue("phone")),
			Subject: strings.TrimSpace(r.FormValue("subject")),
			Message: strings.TrimSpace(r.FormValue("message")),
			Status:  "unread",
		})
		if err != nil {
			h.log.WarnContext(ctx, "contact submit: failed to record inquiry", "error", err)
		} else {
			submitted = true
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ContactPage(lang, dir, submitted).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render contact page after submit", "error", err)
	}
}

// AdminMessagesPage renders the platform contact-message inbox.
func (h *UIHandler) AdminMessagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var messages []*platformadmin.ContactMessage
	if h.adminSvc != nil {
		messages, _ = h.adminSvc.ListContactMessages(database.AsSystem(ctx), "", 200, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminMessages(lang, dir, messages).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin messages", "error", err)
	}
}

// AdminMessageToggleSubmit toggles the status of a contact message between unread and read.
func (h *UIHandler) AdminMessageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.adminSvc != nil {
		msgs, err := h.adminSvc.ListContactMessages(database.AsSystem(ctx), "", 500, 0)
		if err == nil {
			for _, m := range msgs {
				if m.ID == id {
					newStatus := "read"
					if m.Status == "read" {
						newStatus = "unread"
					}
					_ = h.adminSvc.UpdateContactMessageStatus(database.AsSystem(ctx), id, newStatus)
					break
				}
			}
		}
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", "تم تحديث حالة الرسالة بنجاح.")
}

// AdminMessageDeleteSubmit deletes a contact message from the inbox.
func (h *UIHandler) AdminMessageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.adminSvc != nil {
		_ = h.adminSvc.DeleteContactMessage(database.AsSystem(ctx), id)
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", "تم حذف الرسالة بنجاح.")
}
