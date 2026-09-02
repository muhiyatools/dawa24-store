package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
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
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	decisions, total, err := h.catSvc.ListMatchDecisions(ctx, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions", "error", err)
	}

	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.AdminMatchDecisionsData{
		Decisions: decisions,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Search:    search,
		IsEnabled: isEnabled,
	}

	_ = pages.AdminMatchDecisionsPage(lang, dir, data).Render(ctx, w)
}

// AdminMatchDecisionToggleStateSubmit toggles the global Decision Memory active switch across the platform.
func (h *UIHandler) AdminMatchDecisionToggleStateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	currentlyEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)
	targetState := !currentlyEnabled

	if stateStr := r.FormValue("enabled"); stateStr != "" {
		targetState = (stateStr == "true" || stateStr == "1" || stateStr == "on")
	}

	if err := h.catSvc.SetDecisionMemoryEnabled(ctx, targetState); err != nil {
		h.log.ErrorContext(ctx, "failed to toggle decision memory state", "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.toggle_error"))
		return
	}

	stateLabel := i18n.T(lang, "admin.decision_memory.state_enabled")
	if !targetState {
		stateLabel = i18n.T(lang, "admin.decision_memory.state_disabled")
	}
	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", fmt.Sprintf(i18n.T(lang, "admin.decision_memory.toggle_success"), stateLabel))
}

// AdminMatchDecisionDeleteSubmit handles deleting a single match decision from the central cache.
func (h *UIHandler) AdminMatchDecisionDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if err := h.catSvc.DeleteMatchDecision(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// AdminMatchDecisionsClearSubmit purges the entire match decision cache.
func (h *UIHandler) AdminMatchDecisionsClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	if err := h.catSvc.ClearMatchDecisions(ctx); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions", "error", err)
		h.redirectWithNotice(w, r, "/admin/match-decisions", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}

	h.redirectWithNotice(w, r, "/admin/match-decisions", "success", i18n.T(lang, "admin.decision_memory.clear_success"))
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
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	// Scoped strictly to the pharmacy's organization
	decisions, total, err := h.catSvc.ListMatchDecisionsForOrg(ctx, actor.OrganizationID, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions for customer", "error", err)
	}

	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.CustomerDecisionMemoryData{
		Decisions: decisions,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Search:    search,
		IsEnabled: isEnabled,
		IsVendor:  false,
	}

	_ = pages.CustomerDecisionMemoryPage(lang, dir, data).Render(ctx, w)
}

// CustomerDecisionMemoryAddSubmit adds a new manual match decision for the pharmacy.
func (h *UIHandler) CustomerDecisionMemoryAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))
	productIDStr := strings.TrimSpace(r.FormValue("product_id"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 || rawName == "" {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "decision_memory.item_and_drug_required"))
		return
	}

	if reason == "" {
		reason = i18n.T(lang, "decision_memory.customer_manual_reason")
	}

	if err := h.catSvc.SaveManualDecision(ctx, actor.OrganizationID, actor.UserID, rawName, productID, reason); err != nil {
		h.log.ErrorContext(ctx, "failed to add manual decision for customer", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "decision_memory.save_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "decision_memory.customer_saved_success"))
}

// CustomerDecisionMemoryDeleteSubmit deletes a single customer saved match decision.
func (h *UIHandler) CustomerDecisionMemoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if err := h.catSvc.DeleteMatchDecisionForOrg(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision for customer", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// CustomerDecisionMemoryClearSubmit clears all match decisions from the customer organization's memory.
func (h *UIHandler) CustomerDecisionMemoryClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/decision-memory", http.StatusSeeOther)
		return
	}

	if err := h.catSvc.ClearMatchDecisionsForOrg(ctx, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions for customer", "error", err)
		h.redirectWithNotice(w, r, "/customer/decision-memory", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}

	h.redirectWithNotice(w, r, "/customer/decision-memory", "success", i18n.T(lang, "decision_memory.org_cleared_success"))
}

// VendorDecisionMemoryPage renders the decision memory list for vendor organizations.
func (h *UIHandler) VendorDecisionMemoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	// Scoped strictly to the vendor's organization
	decisions, total, err := h.catSvc.ListMatchDecisionsForOrg(ctx, actor.OrganizationID, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list match decisions for vendor", "error", err)
	}

	isEnabled := h.catSvc.IsDecisionMemoryEnabled(ctx)

	data := pages.CustomerDecisionMemoryData{
		Decisions: decisions,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Search:    search,
		IsEnabled: isEnabled,
		IsVendor:  true,
	}

	_ = pages.CustomerDecisionMemoryPage(lang, dir, data).Render(ctx, w)
}

// VendorDecisionMemoryAddSubmit adds a new manual match decision for the vendor.
func (h *UIHandler) VendorDecisionMemoryAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))
	productIDStr := strings.TrimSpace(r.FormValue("product_id"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 || rawName == "" {
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "decision_memory.item_and_drug_required"))
		return
	}

	if reason == "" {
		reason = i18n.T(lang, "decision_memory.vendor_manual_reason")
	}

	if err := h.catSvc.SaveManualDecision(ctx, actor.OrganizationID, actor.UserID, rawName, productID, reason); err != nil {
		h.log.ErrorContext(ctx, "failed to add manual decision for vendor", "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "decision_memory.save_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "decision_memory.vendor_saved_success"))
}

// VendorDecisionMemoryDeleteSubmit deletes a single vendor saved match decision.
func (h *UIHandler) VendorDecisionMemoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.invalid_id"))
		return
	}

	if err := h.catSvc.DeleteMatchDecisionForOrg(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "failed to delete match decision for vendor", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.delete_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "admin.decision_memory.delete_success"))
}

// VendorDecisionMemoryClearSubmit clears all match decisions from the vendor organization's memory.
func (h *UIHandler) VendorDecisionMemoryClearSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/decision-memory", http.StatusSeeOther)
		return
	}

	if err := h.catSvc.ClearMatchDecisionsForOrg(ctx, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "failed to clear match decisions for vendor", "error", err)
		h.redirectWithNotice(w, r, "/vendor/decision-memory", "error", i18n.T(lang, "admin.decision_memory.clear_error"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/decision-memory", "success", i18n.T(lang, "decision_memory.org_cleared_success"))
}
