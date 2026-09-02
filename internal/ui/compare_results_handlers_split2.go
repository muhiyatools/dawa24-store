package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareMarketIntelligencePage renders the market intelligence dashboard.
func (h *UIHandler) CompareMarketIntelligencePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-intelligence", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.intel.vendors_only"))
		return
	}
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureMarketDiscounts)
		if err != nil || !allowed {
			h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", i18n.T(lang, "compare.intel.upgrade_required"))
			return
		}
	}

	var report *compare.MarketIntelligenceReport
	if h.compareSvc != nil {
		rep, err := h.compareSvc.GetMarketIntelligenceReport(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to get market intelligence report", "error", err)
		} else {
			report = rep
		}
	}

	pageData := pages.MarketIntelligencePageData{
		Report:     report,
		IsCustomer: actor.IsCustomer(),
	}

	h.renderPage(ctx, w, "render market intelligence page", pages.CompareMarketIntelligencePage(lang, dir, pageData))
}

// MarketDiscountsPage renders market-wide approved discounts across all suppliers and warehouses.
func (h *UIHandler) MarketDiscountsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/market-discounts", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.discounts.vendors_only"))
		return
	}
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureMarketDiscounts)
		if err != nil || !allowed {
			h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", i18n.T(lang, "compare.discounts.upgrade_required"))
			return
		}
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	supplier := strings.TrimSpace(r.URL.Query().Get("supplier"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortBy == "" {
		sortBy = "discount_desc"
	}
	currentView := strings.TrimSpace(r.URL.Query().Get("view"))
	if currentView != "grid" {
		currentView = "list"
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	limit := 24
	if lStr := strings.TrimSpace(r.URL.Query().Get("limit")); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && (l == 24 || l == 48 || l == 96) {
			limit = l
		}
	}

	var minPricePtr, maxPricePtr *float64
	if minPStr := strings.TrimSpace(r.URL.Query().Get("min_price")); minPStr != "" {
		if val, err := strconv.ParseFloat(minPStr, 64); err == nil && val >= 0 {
			minPricePtr = &val
		}
	}
	if maxPStr := strings.TrimSpace(r.URL.Query().Get("max_price")); maxPStr != "" {
		if val, err := strconv.ParseFloat(maxPStr, 64); err == nil && val >= 0 {
			maxPricePtr = &val
		}
	}

	var minDiscPtr, maxDiscPtr *float64
	if minDStr := strings.TrimSpace(r.URL.Query().Get("min_discount")); minDStr != "" {
		if val, err := strconv.ParseFloat(minDStr, 64); err == nil && val >= 0 {
			minDiscPtr = &val
		}
	}
	if maxDStr := strings.TrimSpace(r.URL.Query().Get("max_discount")); maxDStr != "" {
		if val, err := strconv.ParseFloat(maxDStr, 64); err == nil && val >= 0 {
			maxDiscPtr = &val
		}
	}

	var orgIDPtr *int64
	if actor.OrganizationID > 0 {
		orgIDPtr = &actor.OrganizationID
	}

	filter := compare.MarketDiscountsFilter{
		Query:          query,
		Supplier:       supplier,
		OrganizationID: orgIDPtr,
		MinPrice:       minPricePtr,
		MaxPrice:       maxPricePtr,
		MinDiscount:    minDiscPtr,
		MaxDiscount:    maxDiscPtr,
		SortBy:         sortBy,
		Page:           page,
		Limit:          limit,
	}

	var result *compare.MarketDiscountsResult
	if h.compareSvc != nil {
		res, err := h.compareSvc.ListMarketDiscounts(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "list market discounts error", "error", err)
		} else {
			result = res
		}
	}

	if result == nil {
		result = &compare.MarketDiscountsResult{
			Items:              make([]*compare.MarketDiscountRow, 0),
			Page:               page,
			Limit:              limit,
			TotalPages:         1,
			AvailableSuppliers: make([]string, 0),
		}
	}

	h.renderPage(ctx, w, "render market discounts", pages.MarketDiscountsPage(lang, dir, actor, result, filter, currentView))
}
