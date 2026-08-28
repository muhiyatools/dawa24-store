package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SmartOrderProgressPage renders step 3.
//
// Everything shown comes from the database rather than from the request that
// started the run, so a buyer who closed the tab and came back on another device
// sees the true state (FR-027, US8).
func (h *UIHandler) SmartOrderProgressPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	// A finished run has no business showing a spinner.
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale:
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/results", http.StatusSeeOther)
		return
	case smartorder.StatusPlaced:
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/review", http.StatusSeeOther)
		return
	}

	events, _ := h.smartOrderSvc.Events(ctx, run.ID, 0)
	data := pages.SmartOrderProgressData{
		Run:     run,
		Events:  events,
		Failed:  run.Status == smartorder.StatusFailed,
		Percent: smartorder.RunPercent(events),
		Caption: "جارٍ تجهيز الأصناف",
		Message: "جارٍ مطابقة الأصناف والبحث عن الموردين…",
	}
	if data.Failed {
		data.Message = run.FailureReason
		if data.Message == "" {
			data.Message = "حدث خطأ غير متوقع أثناء المعالجة."
		}
	} else if stage := smartorder.CurrentStage(events); stage != "" {
		data.Caption = stage.Label()
		data.AIRunning = stage == smartorder.StageInitialDone ||
			stage == smartorder.StageAIEnhance || stage == smartorder.StageAdjudicate
	}
	// A run that has been claimed but has emitted nothing yet is genuinely at
	// the start, not stalled. Showing a little progress is the honest reading
	// and stops a freshly queued run looking like a dead one.
	if !data.Failed && data.Percent == 0 {
		data.Percent = 2
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderProgressPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order progress page", "error", err)
	}
}

// SmartOrderResultsPage renders step 4: matching and supplier results.
func (h *UIHandler) SmartOrderResultsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	// Default filter is "unmatched" as requested, unless explicitly overridden
	match := strings.TrimSpace(q.Get("match"))
	if match == "" && q.Get("outcome") != "" {
		match = q.Get("outcome")
	} else if match == "" && q.Has("all") {
		match = ""
	} else if match == "" && !q.Has("match") && !q.Has("outcome") {
		match = "unmatched"
	} else if match == "all" {
		match = ""
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 25
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && (l == 10 || l == 25 || l == 50 || l == 100 || l == -1) {
		limit = l
	}

	sortBy := strings.TrimSpace(q.Get("sort"))
	sortOrder := strings.TrimSpace(q.Get("order"))
	search := strings.TrimSpace(q.Get("q"))

	filter := smartorder.LineFilter{
		MatchGroup: match,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		Search:     search,
		Limit:      limit,
		Offset:     (page - 1) * limit,
		All:        limit == -1,
	}

	filterCounts, err := h.smartOrderSvc.FilterCounts(ctx, run.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order filter counts", "run_id", run.ID, "error", err)
		http.Error(w, "تعذّر تحميل فلاتر النتائج", http.StatusInternalServerError)
		return
	}

	lines, total, err := h.smartOrderSvc.Results(ctx, run, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order results", "run_id", run.ID, "error", err)
		http.Error(w, "تعذّر تحميل النتائج", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderResultsPage(lang, dir, pages.SmartOrderResultsData{
		Run:       run,
		Counts:    filterCounts,
		Lines:     lines,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Match:     match,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Search:    search,
	}).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order results page", "error", err)
	}
}

// SmartOrderReviewPage renders step 5: the dedicated review cart.
//
// It reads only smartorder data. The ordinary shopping cart is never touched:
// an abandoned import must not leave items in a cart the buyer believes is
// empty (FR-042).
func (h *UIHandler) SmartOrderReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	orderable, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{
		Outcome: string(smartorder.OutcomeOrdered), All: true,
	})
	if err != nil {
		http.Error(w, "تعذّر تحميل الطلب", http.StatusInternalServerError)
		return
	}

	data := pages.SmartOrderReviewData{
		Run:        run,
		Error:      r.URL.Query().Get("error"),
		BranchName: h.branchName(ctx, run),
		// Shown once, on the render that follows a refused finalisation.
		Stale: h.smartOrderStale.take(run.PublicID),
	}
	byVendor := map[string]*pages.SmartOrderReviewGroup{}

	for _, l := range orderable {
		sel, err := h.smartOrderSvc.Selection(ctx, run.OrganizationID, l.ID)
		if err != nil {
			continue
		}
		candidates, err := h.smartOrderSvc.Candidates(ctx, run.OrganizationID, l.ID)
		if err != nil {
			continue
		}
		chosen, alternatives := splitCandidates(candidates, sel.CandidateID)
		if chosen == nil {
			continue
		}
		vendor := h.vendorName(ctx, chosen.VendorOrgID)

		group, exists := byVendor[vendor]
		if !exists {
			group = &pages.SmartOrderReviewGroup{VendorName: vendor}
			byVendor[vendor] = group
		}
		group.Lines = append(group.Lines, pages.SmartOrderReviewLine{
			Line:           l,
			VendorName:     vendor,
			AvailableStock: chosen.StockQty,
			UnitPrice:      chosen.NetUnitPrice,
			DiscountPct:    float64(chosen.DiscountBps) / 100,
			LineNet:        sel.LineNet,
			DecidedBy:      sel.DecidedBy,
			SkippedName:    h.skippedVendorName(ctx, candidates, sel.SkippedCandidateID),
			SkippedExcess:  derefFloat(sel.SkippedExcessPct),
			Alternatives:   alternatives,
		})
		if sum, err := group.Subtotal.Add(sel.LineNet); err == nil {
			group.Subtotal = sum
		}
	}
	for _, g := range byVendor {
		data.Groups = append(data.Groups, *g)
	}

	// Everything the buyer is not getting, shown separately rather than omitted
	// (FR-045).
	for _, outcome := range []smartorder.Outcome{
		smartorder.OutcomeUnmatched, smartorder.OutcomeNoSupplier,
		smartorder.OutcomeCoverageBlocked, smartorder.OutcomeInstitutionalBlocked,
		smartorder.OutcomeOutOfStock, smartorder.OutcomeBelowMinQty,
		smartorder.OutcomeZeroQty,
	} {
		excluded, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{
			Outcome: string(outcome), Limit: 200,
		})
		if err == nil {
			data.Excluded = append(data.Excluded, excluded...)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderReviewPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order review page", "error", err)
	}
}

// SmartOrderHistoryPage lists previous runs for this organisation.
func (h *UIHandler) SmartOrderHistoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || h.smartOrderSvc == nil {
		http.Redirect(w, r, "/customer/smart-order/new", http.StatusSeeOther)
		return
	}
	_, _ = h.smartOrderSvc.History(ctx, actor.OrganizationID, 50, 0)
	// History shares the import screen for now. A dedicated view is presentation,
	// not behaviour, and the run data it needs is already exposed by the service.
	http.Redirect(w, r, "/customer/smart-order/new", http.StatusSeeOther)
}

// splitCandidates separates the chosen offer from the alternatives the buyer
// could switch to.
func splitCandidates(candidates []smartorder.Candidate, chosenID int64) (*smartorder.Candidate, int) {
	var chosen *smartorder.Candidate
	alternatives := 0
	for i := range candidates {
		if candidates[i].ID == chosenID {
			chosen = &candidates[i]
			continue
		}
		if candidates[i].Eligible {
			alternatives++
		}
	}
	return chosen, alternatives
}

// skippedVendorName names the supplier the tolerance band passed over, so the
// line can say who was skipped rather than just that someone was.
func (h *UIHandler) skippedVendorName(ctx context.Context, candidates []smartorder.Candidate, skippedID *int64) string {
	if skippedID == nil {
		return ""
	}
	for _, c := range candidates {
		if c.ID == *skippedID {
			return h.vendorName(ctx, c.VendorOrgID)
		}
	}
	return ""
}

func (h *UIHandler) vendorName(ctx context.Context, orgID int64) string {
	if h.orgSvc == nil {
		return "مورد"
	}
	o, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || o == nil || o.LegalName == "" {
		return "مورد"
	}
	return o.LegalName
}

func (h *UIHandler) branchName(ctx context.Context, run *smartorder.Run) string {
	if h.orgSvc == nil {
		return ""
	}
	b, err := h.orgSvc.GetBranch(ctx, run.BranchID)
	if err != nil || b == nil {
		return ""
	}
	return b.Name.Get("ar")
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
