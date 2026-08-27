package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareRunSubmit processes selection of suppliers and redirects to results view (Plan V5 Phase 2 §2.5.1).
func (h *UIHandler) CompareRunSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "هذه الأداة مخصصة لحسابات الموردين فقط.")
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

	// Validate all selected files are ready (have mapping applied)
	if h.compareSvc != nil {
		for _, idStr := range supplierIDs {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil && id > 0 {
				file, errGet := h.compareSvc.GetFile(ctx, id)
				if errGet == nil && file != nil && file.Status != compare.FileReady {
					h.redirectWithNotice(w, r, "/compare/tool", "error",
						fmt.Sprintf("الملف '%s' بحاجة إلى تعيين الأعمدة أولاً. الرجاء اكتمال تعيين الأعمدة لجميع الملفات المختارة.", file.SupplierName))
					return
				}
			}
		}
	}

	queryParam := strings.Join(supplierIDs, ",")
	http.Redirect(w, r, "/compare/results?suppliers="+queryParam, http.StatusSeeOther)
}

// CompareResultsPage renders multi-supplier comparison results with full filtering, sorting and metrics.
func (h *UIHandler) CompareResultsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/results", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "هذه الصفحة مخصصة لحسابات الموردين فقط.")
		return
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

	var result *compare.ComparisonResultSet
	if h.compareSvc != nil {
		res, err := h.compareSvc.RunMultiSupplierComparison(ctx, fileIDs)
		if err == nil {
			result = res
		} else {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر معالجة مقارنة الملفات: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareResultsPage(lang, dir, result, filter).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare results", "error", err)
	}
}

// CompareHeadToHeadPage handles head-to-head comparison between two suppliers.
func (h *UIHandler) CompareHeadToHeadPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/head-to-head", http.StatusSeeOther)
		return
	}

	var files []*compare.CompareFile
	if h.compareSvc != nil {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		files, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
	}

	sourceID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("source")), 10, 64)
	targetID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("target")), 10, 64)

	// Default selection to first two files if not specified and at least 2 files exist
	if sourceID <= 0 && len(files) >= 1 {
		sourceID = files[0].ID
	}
	if targetID <= 0 && len(files) >= 2 {
		targetID = files[1].ID
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	minPStr := strings.TrimSpace(r.URL.Query().Get("min_price"))
	maxPStr := strings.TrimSpace(r.URL.Query().Get("max_price"))
	minDStr := strings.TrimSpace(r.URL.Query().Get("min_discount"))
	maxDStr := strings.TrimSpace(r.URL.Query().Get("max_discount"))
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "all"
	}

	var minP, maxP, minD, maxD *float64
	if v, err := strconv.ParseFloat(minPStr, 64); err == nil && v >= 0 {
		minP = &v
	}
	if v, err := strconv.ParseFloat(maxPStr, 64); err == nil && v >= 0 {
		maxP = &v
	}
	if v, err := strconv.ParseFloat(minDStr, 64); err == nil && v >= 0 {
		minD = &v
	}
	if v, err := strconv.ParseFloat(maxDStr, 64); err == nil && v >= 0 {
		maxD = &v
	}

	var outcome *compare.HeadToHeadOutcome
	if tab == "your_better" {
		o := compare.OutcomeYourBetter
		outcome = &o
	} else if tab == "equal" {
		o := compare.OutcomeEqual
		outcome = &o
	} else if tab == "competitor_better" {
		o := compare.OutcomeCompetitorBetter
		outcome = &o
	}

	var result *compare.HeadToHeadComparisonResult
	if h.compareSvc != nil && sourceID > 0 && targetID > 0 {
		res, err := h.compareSvc.RunSupplierVsSupplierDetailed(ctx, compare.HeadToHeadFilter{
			SourceFileID: sourceID,
			TargetFileID: targetID,
			Query:        q,
			MinPrice:     minP,
			MaxPrice:     maxP,
			MinDiscount:  minD,
			MaxDiscount:  maxD,
			Outcome:      outcome,
		})
		if err != nil {
			h.log.ErrorContext(ctx, "failed to run head-to-head comparison", "error", err)
		} else {
			result = res
		}
	}

	pageData := pages.HeadToHeadPageData{
		Result:       result,
		Files:        files,
		SourceFileID: sourceID,
		TargetFileID: targetID,
		Query:        q,
		MinPrice:     minPStr,
		MaxPrice:     maxPStr,
		MinDiscount:  minDStr,
		MaxDiscount:  maxDStr,
		ActiveTab:    tab,
		IsCustomer:   actor.IsCustomer(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareHeadToHeadPage(lang, dir, pageData).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render head to head page", "error", err)
	}
}

// CompareMarketBenchmarkPage handles benchmarking a supplier file against platform market suppliers.
func (h *UIHandler) CompareMarketBenchmarkPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-benchmark", http.StatusSeeOther)
		return
	}

	var files []*compare.CompareFile
	if h.compareSvc != nil {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		files, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
	}

	fileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("file")), 10, 64)
	if fileID <= 0 && len(files) > 0 {
		fileID = files[0].ID
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	minPStr := strings.TrimSpace(r.URL.Query().Get("min_price"))
	maxPStr := strings.TrimSpace(r.URL.Query().Get("max_price"))
	minDStr := strings.TrimSpace(r.URL.Query().Get("min_discount"))
	maxDStr := strings.TrimSpace(r.URL.Query().Get("max_discount"))
	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "all"
	}

	var minP, maxP, minD, maxD *float64
	if v, err := strconv.ParseFloat(minPStr, 64); err == nil && v >= 0 {
		minP = &v
	}
	if v, err := strconv.ParseFloat(maxPStr, 64); err == nil && v >= 0 {
		maxP = &v
	}
	if v, err := strconv.ParseFloat(minDStr, 64); err == nil && v >= 0 {
		minD = &v
	}
	if v, err := strconv.ParseFloat(maxDStr, 64); err == nil && v >= 0 {
		maxD = &v
	}

	var result *compare.MarketBenchmarkResult
	if h.compareSvc != nil && fileID > 0 {
		res, err := h.compareSvc.RunMarketBenchmarkDetailed(ctx, compare.MarketBenchmarkFilter{
			FileID:      fileID,
			Query:       q,
			MinPrice:    minP,
			MaxPrice:    maxP,
			MinDiscount: minD,
			MaxDiscount: maxD,
			Tab:         tab,
		})
		if err != nil {
			h.log.ErrorContext(ctx, "failed to run market benchmark", "error", err)
		} else {
			result = res
		}
	}

	pageData := pages.MarketBenchmarkPageData{
		Result:      result,
		Files:       files,
		FileID:      fileID,
		Query:       q,
		MinPrice:    minPStr,
		MaxPrice:    maxPStr,
		MinDiscount: minDStr,
		MaxDiscount: maxDStr,
		ActiveTab:   tab,
		IsCustomer:  actor.IsCustomer(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareMarketBenchmarkPage(lang, dir, pageData).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render market benchmark page", "error", err)
	}
}

// CompareMarketIntelligencePage renders the market intelligence dashboard.
func (h *UIHandler) CompareMarketIntelligencePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-intelligence", http.StatusSeeOther)
		return
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareMarketIntelligencePage(lang, dir, pageData).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render market intelligence page", "error", err)
	}
}

// MarketDiscountsPage renders market-wide approved discounts matching the Laravel MarketDiscounts module.
func (h *UIHandler) MarketDiscountsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// Authentication check (same as other pharmacy/customer tools)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/market-discounts", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "هذه الصفحة مخصصة لحسابات الموردين فقط.")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	supplier := strings.TrimSpace(r.URL.Query().Get("supplier"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortBy == "" {
		sortBy = "newest"
	}
	currentView := strings.TrimSpace(r.URL.Query().Get("view"))
	if currentView != "grid" {
		currentView = "list"
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
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

	filter := compare.MarketDiscountsFilter{
		Query:       query,
		Supplier:    supplier,
		MinPrice:    minPricePtr,
		MaxPrice:    maxPricePtr,
		MinDiscount: minDiscPtr,
		MaxDiscount: maxDiscPtr,
		SortBy:      sortBy,
		Page:        page,
		Limit:       24,
	}

	var result *compare.MarketDiscountsResult
	if h.compareSvc != nil {
		res, err := h.compareSvc.ListMarketDiscounts(ctx, filter)
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
			Limit:              24,
			TotalPages:         1,
			AvailableSuppliers: make([]string, 0),
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.MarketDiscountsPage(lang, dir, result, filter, currentView).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render market discounts", "error", err)
	}
}
