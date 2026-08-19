package ui

import (
	"net/http"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
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

	submitted := false
	if h.adminSvc != nil {
		err := h.adminSvc.SubmitContactMessage(ctx, &platformadmin.ContactMessage{
			Name:    r.PostFormValue("name"),
			Email:   r.PostFormValue("email"),
			Phone:   r.PostFormValue("phone"),
			Subject: r.PostFormValue("subject"),
			Message: r.PostFormValue("message"),
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
		messages, _ = h.adminSvc.ListContactMessages(ctx, "", 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminMessages(lang, dir, messages).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin messages", "error", err)
	}
}
