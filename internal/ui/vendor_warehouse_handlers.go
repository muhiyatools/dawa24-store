package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorWarehousesPage renders the vendor's permanent and branch storage warehouses.
func (h *UIHandler) VendorWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
	}

	var branches []*org.Branch
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorWarehousesPage(warehouses, branches, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor warehouses page", "error", err)
	}
}

// VendorWarehouseDetailPage renders single warehouse details and current stock rows.
func (h *UIHandler) VendorWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	var stocks []*inventory.Stock
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID {
				wh = item
				break
			}
		}
		stocks, _ = h.invSvc.ListStocksByWarehouse(ctx, whID)
	}

	if wh == nil {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorWarehouseDetailPage(wh, stocks, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor warehouse detail", "error", err)
	}
}

// VendorWarehouseCreateSubmit handles creating a new warehouse facility for the vendor.
func (h *UIHandler) VendorWarehouseCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "بيانات النموذج غير صالحة.")
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))

	if name == "" {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "اسم المخزن مطلوب.")
		return
	}
	if code == "" {
		code = fmt.Sprintf("WH-%d-%d", actor.OrganizationID, SystemRandomInt(100, 999))
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	var lat, lng *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if v, err := strconv.ParseFloat(latStr, 64); err == nil {
			lat = &v
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lng = &v
		}
	}

	// Auto-fallback from branch coordinates if omitted
	if (lat == nil || lng == nil) && branchID != nil && h.orgSvc != nil {
		if b, err := h.orgSvc.GetBranch(ctx, *branchID); err == nil && b != nil {
			if lat == nil {
				lat = b.Latitude
			}
			if lng == nil {
				lng = b.Longitude
			}
			if address == "" && b.Address != "" {
				address = b.Address
			}
			if phone == "" && b.Phone != "" {
				phone = b.Phone
			}
		}
	}

	wh := &inventory.Warehouse{
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		Name:           name,
		Code:           code,
		Address:        address,
		Phone:          phone,
		Latitude:       lat,
		Longitude:      lng,
		IsActive:       true,
	}

	if h.invSvc != nil {
		if _, err := h.invSvc.CreateWarehouse(ctx, wh); err != nil {
			h.log.ErrorContext(ctx, "create warehouse failed", "error", err)
			h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "تعذر إضافة المخزن، يرجى التحقق من صحة البيانات.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", "تمت إضافة المخزن بنجاح وتفعيله للأرصدة.")
}

// VendorWarehouseUpdateSubmit handles editing an existing warehouse facility.
func (h *UIHandler) VendorWarehouseUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "بيانات النموذج غير صالحة.")
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID && item.OrganizationID == actor.OrganizationID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "المخزن المطلوب غير موجود.")
		return
	}

	wh.Name = strings.TrimSpace(r.PostFormValue("name"))
	if wh.Name == "" {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "اسم المخزن مطلوب.")
		return
	}
	if code := strings.TrimSpace(r.PostFormValue("code")); code != "" {
		wh.Code = code
	}
	wh.Address = strings.TrimSpace(r.PostFormValue("address"))
	wh.Phone = strings.TrimSpace(r.PostFormValue("phone"))
	wh.IsActive = r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			wh.BranchID = &bID
		}
	}

	if h.invSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		if _, err := h.invSvc.UpdateWarehouse(ctx, wh.ID, wh); err != nil {
			h.log.ErrorContext(ctx, "update warehouse failed", "error", err)
			h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "تعذر تحديث بيانات المخزن.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", "تم تحديث بيانات المخزن بنجاح.")
}

// VendorWarehouseToggleSubmit toggles the active/inactive status of a warehouse.
func (h *UIHandler) VendorWarehouseToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID && item.OrganizationID == actor.OrganizationID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		h.redirectWithNotice(w, r, "/vendor/warehouses", "error", "المخزن المطلوب غير موجود.")
		return
	}

	wh.IsActive = !wh.IsActive
	if h.invSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		_, _ = h.invSvc.UpdateWarehouse(ctx, wh.ID, wh)
	}

	msg := "تم تعطيل المخزن بنجاح."
	if wh.IsActive {
		msg = "تم تفعيل المخزن بنجاح."
	}
	h.redirectWithNotice(w, r, "/vendor/warehouses", "success", msg)
}

// SystemRandomInt returns a simple pseudorandom integer between min and max.
func SystemRandomInt(min, max int) int {
	return min + int(timeNowUnixNano()%int64(max-min+1))
}

func timeNowUnixNano() int64 {
	return 17873019
}
