package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminChatHistoryPage renders audit list of all platform conversations.
func (h *UIHandler) AdminChatHistoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "ai"
	}

	var aiConvs []*assistant.ConversationSummary
	if h.assistantRepo != nil {
		aiConvs, _ = h.assistantRepo.ListAllConversations(database.AsSystem(ctx), 100, 0)
	}

	var userConvs []*chat.Conversation
	if h.chatSvc != nil {
		userConvs, _ = h.chatSvc.ListConversations(database.AsSystem(ctx), 0, 100, 0)
	}

	data := pages.AdminChatHistoryData{
		ActiveTab: tab,
		AIConvs:   aiConvs,
		UserConvs: userConvs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminChatHistoryPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin chat history", "error", err)
	}
}

// AdminAIChatDetailPage renders full messages transcript for an AI assistant conversation.
func (h *UIHandler) AdminAIChatDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Redirect(w, r, "/admin/chat/history?tab=ai", http.StatusSeeOther)
		return
	}

	var summary *assistant.ConversationSummary
	var msgs []*assistant.Message
	if h.assistantRepo != nil {
		summary, _ = h.assistantRepo.GetConversationSummary(database.AsSystem(ctx), convID)
		msgs, _ = h.assistantRepo.ListMessages(database.AsSystem(ctx), convID, 200)
	}

	if summary == nil {
		http.Redirect(w, r, "/admin/chat/history?tab=ai", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAIChatDetailPage(summary, msgs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin ai chat detail", "error", err)
	}
}

// AdminChatHistoryDetailPage renders messages within a specific peer conversation.
func (h *UIHandler) AdminChatHistoryDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	convID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || convID <= 0 {
		http.Redirect(w, r, "/admin/chat/history?tab=users", http.StatusSeeOther)
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
