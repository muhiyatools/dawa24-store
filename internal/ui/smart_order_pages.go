package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
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
		Caption: i18n.T(lang, "smartorder.staging_caption"),
		Message: i18n.T(lang, "smartorder.matching_message"),
	}
	if data.Failed {
		data.Message = run.FailureReason
		if data.Message == "" {
			data.Message = i18n.T(lang, "smartorder.unexpected_error")
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

	h.renderPage(ctx, w, "render smart order progress page", pages.SmartOrderProgressPage(lang, dir, data))
}

// SmartOrderProgressJSON is the poll behind the progress ring.
//
// The page used to refresh itself whole every three seconds with a
// <meta http-equiv="refresh">, which reloads the shell, the sidebar and the
// assistant to move one number, loses the scroll position each time, and makes
// the ring advance in visible three-second jumps. It polls this instead and
// eases between the values, using the same bar every other import tool uses.
func (h *UIHandler) SmartOrderProgressJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	events, _ := h.smartOrderSvc.Events(ctx, run.ID, 0)
	percent := smartorder.RunPercent(events)
	caption := i18n.T(lang, "smartorder.staging_caption")
	if stage := smartorder.CurrentStage(events); stage != "" {
		caption = stage.Label()
	}

	// The run is over when the run says so, never when the arithmetic reaches
	// the end of the last band. A percentage that hits 100 while the finalise
	// step is still writing is the bug this whole pass exists to remove.
	done := false
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale,
		smartorder.StatusPlaced, smartorder.StatusFailed:
		done = true
		percent = importprogress.Complete
	default:
		if percent >= importprogress.Complete {
			percent = importprogress.Complete - 1
		}
		if percent <= 0 {
			percent = 2
		}
	}

	payload := map[string]any{
		"percent": percent,
		"message": caption,
		"status":  run.Status,
		"done":    done,
		"failed":  run.Status == smartorder.StatusFailed,
		"error":   run.FailureReason,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(ctx, "smart order progress encode failed", "error", err)
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

	// The screen opens on what the buyer can actually order.
	//
	// It used to open on "unmatched", which is the opposite of how the tool is
	// used: a pharmacist runs a smart order because it is fast, and the first
	// thing they were shown was the pile of rows that had failed. Everything
	// that failed is still one click away and its counts are on the page — it
	// is simply not the first thing, and it is not mixed in with the lines that
	// are ready.
	match := strings.TrimSpace(q.Get("match"))
	switch {
	case match == "all":
		match = ""
	case match != "":
	case q.Get("outcome") != "":
		match = q.Get("outcome")
	case q.Has("all"):
		match = ""
	default:
		match = "ready_to_order"
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

	// A tab may name a match GROUP ("unmatched", "ready_to_order") or a single
	// OUTCOME ("out_of_stock", "coverage_blocked"). The collapsed panel links
	// by outcome, because "why is this line not ordered" is a question the
	// outcome answers exactly and a group only approximately.
	filter := smartorder.LineFilter{
		MatchGroup: match,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		Search:     search,
		Limit:      limit,
		Offset:     (page - 1) * limit,
		All:        limit == -1,
	}
	if isLineOutcome(match) {
		filter.MatchGroup = ""
		filter.Outcome = match
	}

	filterCounts, err := h.smartOrderSvc.FilterCounts(ctx, run.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order filter counts", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
		return
	}

	lines, total, err := h.smartOrderSvc.Results(ctx, run, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order results", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
		return
	}

	// What the run will not order, split by why, for the collapsed section.
	// A failure here costs a panel, not the page.
	blocked, err := h.smartOrderSvc.BlockedCounts(ctx, run.ID)
	if err != nil {
		h.log.WarnContext(ctx, "load smart order blocked counts", "run_id", run.ID, "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderResultsPage(lang, dir, pages.SmartOrderResultsData{
		Run:       run,
		Counts:    filterCounts,
		Blocked:   blocked,
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

// isLineOutcome reports whether a tab key names one of the run's own outcomes.
//
// Listed rather than derived so an unknown value from a hand-edited URL selects
// nothing rather than being passed to the query as a filter it cannot satisfy.
func isLineOutcome(key string) bool {
	switch smartorder.Outcome(key) {
	case smartorder.OutcomeNoSupplier, smartorder.OutcomeCoverageBlocked,
		smartorder.OutcomeInstitutionalBlocked, smartorder.OutcomeOutOfStock,
		smartorder.OutcomeBelowMinQty, smartorder.OutcomeZeroQty,
		smartorder.OutcomeRemoved:
		return true
	}
	return false
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
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
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

	h.renderPage(ctx, w, "render smart order review page", pages.SmartOrderReviewPage(lang, dir, data))
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
		return i18n.T("ar", "common.supplier")
	}
	o, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || o == nil || o.LegalName == "" {
		return i18n.T("ar", "common.supplier")
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
