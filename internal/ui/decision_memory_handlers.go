package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminMatchDecisionsPage renders the central catalog decision memory management page for administrators.
func (h *UIHandler) AdminMatchDecisionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := (page - 1) * limit

	decisions, total, err := h.catSvc.ListMatchDecisions(ctx, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions", "error", err)
	}

	data := pages.AdminMatchDecisionsData{
		Decisions: decisions,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Search:    search,
	}

	_ = pages.AdminMatchDecisionsPage(lang, dir, data).Render(ctx, w)
}

// AdminMatchDecisionDeleteSubmit handles deleting a single match decision from the central cache.
func (h *UIHandler) AdminMatchDecisionDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", "معرف القرار غير صالح.")
		return
	}

	if err := h.catSvc.DeleteMatchDecision(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", "حدث خطأ أثناء حذف القرار.")
		return
	}

	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", "تم حذف القرار من ذاكرة المطابقة بنجاح.")
}

// AdminMatchDecisionsClearSubmit purges the entire match decision cache.
func (h *UIHandler) AdminMatchDecisionsClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.catSvc.ClearMatchDecisions(ctx); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions", "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", "حدث خطأ أثناء مسح ذاكرة القرارات.")
		return
	}

	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", "تم مسح كافة قرارات الذاكرة بنجاح.")
}

// CustomerDecisionMemoryPage renders the decision memory list for pharmacy customer organizations.
func (h *UIHandler) CustomerDecisionMemoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := (page - 1) * limit

	decisions, total, err := h.catSvc.ListMatchDecisions(ctx, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions for customer", "error", err)
	}

	data := pages.CustomerDecisionMemoryData{
		Decisions: decisions,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Search:    search,
	}

	_ = pages.CustomerDecisionMemoryPage(lang, dir, data).Render(ctx, w)
}

// CustomerDecisionMemoryDeleteSubmit deletes a single customer saved match decision.
func (h *UIHandler) CustomerDecisionMemoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", "معرف القرار غير صالح.")
		return
	}

	if err := h.catSvc.DeleteMatchDecision(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", "حدث خطأ أثناء حذف القرار.")
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", "تم حذف القرار من ذاكرة المطابقة بنجاح.")
}

// CustomerDecisionMemoryClearSubmit clears all match decisions from the memory.
func (h *UIHandler) CustomerDecisionMemoryClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	if err := h.catSvc.ClearMatchDecisions(ctx); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", "حدث خطأ أثناء مسح الذاكرة.")
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", "تم مسح ذاكرة قرارات المطابقة بنجاح.")
}
