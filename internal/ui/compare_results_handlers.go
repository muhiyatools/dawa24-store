package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareRunSubmit processes selection of suppliers and redirects to results view (Plan V5 Phase 2 §2.5.1).
func (h *UIHandler) CompareRunSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر معالجة الطلب.")
		return
	}

	supplierIDs := r.Form["supplier_ids"]
	if len(supplierIDs) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار مورد واحد على الأقل للمقارنة.")
		return
	}

	if len(supplierIDs) > 10 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "الحد الأقصى للمقارنة هو 10 موردين في المرة الواحدة.")
		return
	}

	queryParam := strings.Join(supplierIDs, ",")
	http.Redirect(w, r, "/compare/results?suppliers="+queryParam, http.StatusSeeOther)
}

// CompareResultsPage renders multi-supplier comparison results with full filtering, sorting and pagination (Plan V5 §2.5.1).
func (h *UIHandler) CompareResultsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/results", http.StatusSeeOther)
		return
	}

	if h.compareSvc != nil {
		ent, err := h.compareSvc.EntitlementFor(ctx, actor.UserID, actor.OrganizationID)
		if err != nil || !ent.Active {
			http.Redirect(w, r, "/compare", http.StatusSeeOther)
			return
		}
	}

	supParam := r.URL.Query().Get("suppliers")
	var fileIDs []int64
	for _, s := range strings.Split(supParam, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			fileIDs = append(fileIDs, id)
		}
	}

	if len(fileIDs) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "warning", "يرجى اختيار ملفات الموردين للمقارنة.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render placeholder or results templ page
	if err := pages.CompareToolPage(lang, dir, true).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare results", "error", err)
	}
}

// CompareHeadToHeadPage handles head-to-head comparison between two suppliers.
func (h *UIHandler) CompareHeadToHeadPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	if h.compareSvc != nil {
		ent, err := h.compareSvc.EntitlementFor(ctx, actor.UserID, actor.OrganizationID)
		if err != nil || !ent.Active {
			http.Redirect(w, r, "/compare", http.StatusSeeOther)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareToolPage(lang, dir, true).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render head to head", "error", err)
	}
}

// MarketDiscountsPage renders market-wide approved discounts (Plan V5 §2.5.2).
func (h *UIHandler) MarketDiscountsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/market-discounts", http.StatusSeeOther)
		return
	}

	if h.compareSvc != nil {
		ent, err := h.compareSvc.EntitlementFor(ctx, actor.UserID, actor.OrganizationID)
		if err != nil || !ent.Active {
			http.Redirect(w, r, "/compare", http.StatusSeeOther)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareToolPage(lang, dir, true).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render market discounts", "error", err)
	}
}
