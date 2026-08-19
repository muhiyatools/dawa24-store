package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminChatTreePage redirects / renders chat decision tree (finder).
func (h *UIHandler) AdminChatTreePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/finder", http.StatusSeeOther)
}

// AdminChatHistoryPage renders audit list of all platform conversations.
func (h *UIHandler) AdminChatHistoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var convs []*chat.Conversation
	if h.chatSvc != nil {
		// AsSystem justified: platform administrator audit oversight across peer conversations
		convs, _ = h.chatSvc.ListConversations(database.AsSystem(ctx), 0, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminChatHistoryPage(convs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin chat history", "error", err)
	}
}

// AdminChatHistoryDetailPage renders messages within a specific conversation.
func (h *UIHandler) AdminChatHistoryDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Redirect(w, r, "/admin/chat/history", http.StatusSeeOther)
		return
	}

	var msgs []*chat.Message
	if h.chatSvc != nil {
		msgs, _ = h.chatSvc.GetThread(database.AsSystem(ctx), convID, 100)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminChatHistoryDetailPage(convID, msgs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin chat detail", "error", err)
	}
}
