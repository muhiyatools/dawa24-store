package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareRunSubmit processes selection of suppliers and redirects to results view (Plan V5 Phase 2 §2.5.1).
func (h *UIHandler) CompareRunSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.run.vendors_only"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.run.request_failed"))
		return
	}

	supplierIDs := r.Form["supplier_ids"]
	if len(supplierIDs) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.run.select_at_least_one"))
		return
	}

	if len(supplierIDs) > 10 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.run.max_suppliers_exceeded"))
		return
	}

	// Validate all selected files are ready (have mapping applied)
	if h.compareSvc != nil {
		for _, idStr := range supplierIDs {
			if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil && id > 0 {
				file, errGet := h.compareSvc.GetFile(ctx, id)
				if errGet == nil && file != nil && file.Status != compare.FileReady {
					// A failed file is not an unmapped one. Telling the user to
					// finish the mapping when parsing actually broke sends them
					// to a screen that cannot fix anything.
					msg := fmt.Sprintf(i18n.T(lang, "compare.run.mapping_needed"), file.SupplierName)
					if file.Status == compare.FileFailed {
						reason := strings.TrimSpace(file.ErrorMessage)
						if reason == "" {
							reason = i18n.T(lang, "compare.run.read_content_failed")
						}
						msg = fmt.Sprintf(i18n.T(lang, "compare.run.file_failed_format"), file.SupplierName, reason)
					}
					h.redirectWithNotice(w, r, "/compare/tool", "error", msg)
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

	supParam := r.URL.Query().Get("suppliers")
	var fileIDs []int64
	for _, s := range strings.Split(supParam, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			fileIDs = append(fileIDs, id)
		}
	}

	if len(fileIDs) == 0 && h.compareSvc != nil {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		allF, _ := h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
		if len(allF) == 0 {
			allF, _ = h.compareSvc.ListAllFiles(ctx, "", nil)
		}
		for _, f := range allF {
			if f.Status == compare.FileReady && f.RowCount > 0 {
				fileIDs = append(fileIDs, f.ID)
			}
		}
		if len(fileIDs) == 0 && len(allF) > 0 {
			for _, f := range allF {
				fileIDs = append(fileIDs, f.ID)
			}
		}
	}

	if len(fileIDs) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "warning", i18n.T(lang, "compare.results.select_files_warning"))
		return
	}

	var result *compare.ComparisonResultSet
	if h.compareSvc != nil {
		res, err := h.compareSvc.RunMultiSupplierComparison(ctx, fileIDs)
		if err == nil {
			result = res
		} else {
			h.redirectWithNotice(w, r, "/compare/tool", "error", fmt.Sprintf(i18n.T(lang, "compare.results.process_error_prefix"), h.safeMessage(err, lang)))
			return
		}
	}

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}

	h.renderPage(ctx, w, "render compare results", pages.CompareResultsPage(lang, dir, result, filter))
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
		if len(files) == 0 {
			files, _ = h.compareSvc.ListAllFiles(ctx, "", nil)
		}
	}

	sourceID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("source")), 10, 64)
	targetID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("target")), 10, 64)

	// Default selection to first two files if not specified and at least 2 files exist
	if sourceID <= 0 && len(files) >= 1 {
		sourceID = files[0].ID
	}
	if targetID <= 0 && len(files) >= 2 {
		targetID = files[1].ID
	} else if targetID <= 0 && len(files) == 1 {
		targetID = files[0].ID
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

	h.renderPage(ctx, w, "render head to head page", pages.CompareHeadToHeadPage(lang, dir, pageData))
}

// CompareMarketBenchmarkPage compares one of the caller's lists against the
// whole public market.
func (h *UIHandler) CompareMarketBenchmarkPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-benchmark", http.StatusSeeOther)
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	// The list selector offers the caller's own files. Falling back to every
	// file on the platform, as this used to, meant a supplier with no uploads
	// was silently shown somebody else's list as "قائمتي".
	var files []*compare.CompareFile
	if h.compareSvc != nil {
		files, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
		if len(files) == 0 && actor.IsStaff {
			files, _ = h.compareSvc.ListAllFiles(ctx, "", nil)
		}
	}

	fileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("file")), 10, 64)
	if fileID <= 0 && len(files) > 0 {
		fileID = files[0].ID
	}

	filter := compare.BenchmarkFilter{
		FileID:         fileID,
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		Tab:            strings.TrimSpace(r.URL.Query().Get("tab")),
		Sort:           strings.TrimSpace(r.URL.Query().Get("sort")),
		OrganizationID: orgPtr,
		MinPrice:       optionalFloat(r.URL.Query().Get("min_price")),
		MaxPrice:       optionalFloat(r.URL.Query().Get("max_price")),
		MinDiscount:    optionalFloat(r.URL.Query().Get("min_discount")),
		MaxDiscount:    optionalFloat(r.URL.Query().Get("max_discount")),
	}
	if filter.Tab == "" {
		filter.Tab = "all"
	}

	var (
		result *compare.BenchmarkResult
		failed bool
	)
	if h.compareSvc != nil && fileID > 0 {
		res, err := h.compareSvc.RunMarketBenchmark(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to run market benchmark", "error", err, "file", fileID)
			failed = true
		} else {
			result = res
		}
	}

	pageData := pages.MarketBenchmarkPageData{
		Result:      result,
		Failed:      failed,
		Files:       files,
		FileID:      fileID,
		Query:       filter.Query,
		MinPrice:    strings.TrimSpace(r.URL.Query().Get("min_price")),
		MaxPrice:    strings.TrimSpace(r.URL.Query().Get("max_price")),
		MinDiscount: strings.TrimSpace(r.URL.Query().Get("min_discount")),
		MaxDiscount: strings.TrimSpace(r.URL.Query().Get("max_discount")),
		ActiveTab:   filter.Tab,
		Sort:        filter.Sort,
		IsCustomer:  actor.IsCustomer(),
	}

	h.renderPage(ctx, w, "render market benchmark page", pages.CompareMarketBenchmarkPage(lang, dir, pageData))
}

// optionalFloat reads a query parameter that may legitimately be absent.
func optionalFloat(raw string) *float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < 0 {
		return nil
	}
	return &v
}
