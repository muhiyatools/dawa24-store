package ui

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerAutomationPage renders the Automatic Purchase Request upload and configuration screen.
func (h *UIHandler) CustomerAutomationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/automation", http.StatusSeeOther)
		return
	}

	var history []*workflow.AutomationRequest
	if h.wfSvc != nil {
		reqs, err := h.wfSvc.ListAutomationRequests(ctx, actor.UserID, 5, 0)
		if err == nil {
			history = reqs
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerAutomationPage(lang, dir, history, nil).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer automation page", "error", err)
	}
}

// CustomerAutomationUploadSubmit processes the uploaded order list and runs automatic procurement optimization.
func (h *UIHandler) CustomerAutomationUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/automation", http.StatusSeeOther)
		return
	}

	// Limit request body to 20MB for uploaded files
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		h.redirectWithNotice(w, r, "/customer/automation", "error", "حجم الملف كبير جداً أو البيانات غير صالحة.")
		return
	}

	file, header, err := r.FormFile("automation_file")
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/automation", "error", "يرجى اختيار ملف صالح (Excel أو CSV).")
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/automation", "error", "تعذر قراءة محتوى الملف.")
		return
	}

	highestDiscount := r.FormValue("priority_highest_discount") == "true" || r.FormValue("priority_highest_discount") == "on" || r.FormValue("priority_highest_discount") == "1"
	lowestPrice := r.FormValue("priority_lowest_price") == "true" || r.FormValue("priority_lowest_price") == "on" || r.FormValue("priority_lowest_price") == "1"
	fastestDelivery := r.FormValue("priority_fastest_delivery") == "true" || r.FormValue("priority_fastest_delivery") == "on" || r.FormValue("priority_fastest_delivery") == "1"
	preferredSuppliers := r.FormValue("priority_preferred_suppliers_only") == "true" || r.FormValue("priority_preferred_suppliers_only") == "on" || r.FormValue("priority_preferred_suppliers_only") == "1"

	var budgetPtr *money.Amount
	if bStr := r.FormValue("budget_constraint"); bStr != "" {
		if b, err := money.Parse(bStr); err == nil && b.IsPositive() {
			budgetPtr = &b
		}
	}

	prefs := workflow.Priorities{
		PriorityHighestDiscount:        highestDiscount,
		PriorityLowestPrice:            lowestPrice,
		PriorityFastestDelivery:        fastestDelivery,
		PriorityPreferredSuppliersOnly: preferredSuppliers,
		BudgetConstraint:               budgetPtr,
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.wfSvc != nil {
		created, err := h.wfSvc.CreateAutomationRequest(ctx, actor.UserID, orgPtr, header.Filename, fileBytes, prefs)
		if err != nil {
			h.redirectWithNotice(w, r, "/customer/automation", "error", h.safeMessage(err, langOf(r)))
			return
		}

		// Process request
		var lat, lng *float64
		if r.FormValue("use_location") == "true" || r.FormValue("use_location") == "on" {
			l1 := 30.0444
			l2 := 31.2357
			lat = &l1
			lng = &l2
		}

		processed, err := h.wfSvc.ProcessAutomationRequest(ctx, created.ID, actor.UserID, lat, lng, 50.0)
		if err != nil {
			h.log.WarnContext(ctx, "process automation error", "id", created.ID, "error", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerAutomationDetailPage(lang, dir, processed).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer automation detail after upload", "error", err)
		}
		return
	}

	h.redirectWithNotice(w, r, "/customer/automation", "success", "تم إرسال الملف بنجاح.")
}

// CustomerAutomationDetailPage displays optimization results, vendor splits, and alerts.
func (h *UIHandler) CustomerAutomationDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/automation", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/customer/automation", http.StatusSeeOther)
		return
	}

	var req *workflow.AutomationRequest
	if h.wfSvc != nil {
		rItem, err := h.wfSvc.GetAutomationRequest(ctx, id)
		if err == nil && rItem.UserID == actor.UserID {
			req = rItem
		}
	}
	if req == nil {
		http.Redirect(w, r, "/customer/automation", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerAutomationDetailPage(lang, dir, req).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer automation detail", "error", err)
	}
}

// CustomerAutomationPreviousPage displays history of submitted automation requests.
func (h *UIHandler) CustomerAutomationPreviousPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/automation/previous", http.StatusSeeOther)
		return
	}

	var list []*workflow.AutomationRequest
	if h.wfSvc != nil {
		reqs, err := h.wfSvc.ListAutomationRequests(ctx, actor.UserID, 50, 0)
		if err == nil {
			list = reqs
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerAutomationPreviousPage(lang, dir, list).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer automation previous page", "error", err)
	}
}
