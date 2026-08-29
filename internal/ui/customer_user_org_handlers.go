package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerVendorSearchJSON provides live AJAX vendor search for customers/pharmacies to link to vendors.
func (h *UIHandler) CustomerVendorSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sysCtx := database.AsSystem(ctx)

	type VendorSearchResult struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		LegalName          string `json:"legal_name"`
		OrganizationNumber string `json:"organization_number"`
		TaxNumber          string `json:"tax_number"`
	}

	var results []VendorSearchResult
	if h.orgSvc != nil {
		vType := org.TypeVendor
		vendors, err := h.orgSvc.ListOrganizations(sysCtx, &vType, nil, 200, 0)
		if err == nil {
			qLower := strings.ToLower(q)
			for _, v := range vendors {
				if v == nil {
					continue
				}
				nameAr := v.TradeName.Get("ar")
				nameEn := v.TradeName.Get("en")
				legal := v.LegalName
				name := legal
				if name == "" {
					name = nameAr
				}
				if name == "" {
					name = nameEn
				}
				if name == "" {
					name = fmt.Sprintf("مورد #%d", v.ID)
				}

				if q != "" {
					matches := strings.Contains(strings.ToLower(name), qLower) ||
						strings.Contains(strings.ToLower(legal), qLower) ||
						strings.Contains(strings.ToLower(nameAr), qLower) ||
						strings.Contains(strings.ToLower(nameEn), qLower) ||
						strings.Contains(strings.ToLower(v.TaxNumber), qLower) ||
						strings.Contains(strings.ToLower(v.CommercialRegister), qLower) ||
						strings.Contains(strings.ToLower(v.OrganizationNumber), qLower) ||
						strconv.FormatInt(v.ID, 10) == q
					if !matches {
						continue
					}
				}

				results = append(results, VendorSearchResult{
					ID:                 v.ID,
					Name:               name,
					LegalName:          legal,
					OrganizationNumber: v.OrganizationNumber,
					TaxNumber:          v.TaxNumber,
				})
				if len(results) >= 20 {
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

// CustomerUserOrganizationsPage renders the customer pharmacy organization connections view.
func (h *UIHandler) CustomerUserOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/user-organization", http.StatusSeeOther)
		return
	}

	data := &pages.CustomerUserOrgData{
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice_msg"),
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		userOrgs, err := h.orgSvc.ListUserOrganizationsByUser(sysCtx, actor.UserID)
		if err == nil {
			data.UserOrgs = userOrgs
		}
		// Load approved vendors for selection in add modal
		vType := org.TypeVendor
		vStatus := org.StatusApproved
		vendors, err := h.orgSvc.ListOrganizations(sysCtx, &vType, &vStatus, 200, 0)
		if err == nil {
			data.Vendors = vendors
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerUserOrganizationsPage(lang, dir, data, actor.Permissions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer user organizations", "error", err)
	}
}

// CustomerUserOrganizationCreateSubmit submits a new link request to a vendor organization.
func (h *UIHandler) CustomerUserOrganizationCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/user-organization", http.StatusSeeOther)
		return
	}

	vendorOrgID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("vendor_org_id")), 10, 64)
	if err != nil || vendorOrgID <= 0 {
		http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape("يجب اختيار مورد / شركة صالحة."), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	if orgNumber == "" {
		http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape("رقم المنظمة مطلوب."), http.StatusSeeOther)
		return
	}

	var customerOrgID *int64
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID > 0 {
		customerOrgID = &orgID
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		_, err := h.orgSvc.CreateUserOrgLink(sysCtx, actor.UserID, customerOrgID, vendorOrgID, orgNumber, org.UserOrgStatusPending)
		if err != nil {
			h.log.ErrorContext(ctx, "create user org link failed", "error", err)
			http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/customer/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم ربط المنظمة بنجاح، والطلب الآن قيد انتظار اعتماد المورد."), http.StatusSeeOther)
}

// CustomerUserOrganizationUpdateSubmit updates the organization number for a link.
func (h *UIHandler) CustomerUserOrganizationUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف الربط غير صالح."), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.UpdateUserOrgLink(sysCtx, id, orgNumber, notes); err != nil {
			http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/customer/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم تحديث رقم المنظمة بنجاح."), http.StatusSeeOther)
}

// CustomerUserOrganizationDeleteSubmit removes a user-organization link.
func (h *UIHandler) CustomerUserOrganizationDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف الربط غير صالح."), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.DeleteUserOrgLink(sysCtx, id); err != nil {
			http.Redirect(w, r, "/customer/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/customer/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم حذف ربط المنظمة بنجاح."), http.StatusSeeOther)
}
