package ui

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
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

	h.renderPage(ctx, w, "render vendor warehouses page", pages.VendorWarehousesPage(warehouses, branches, lang, dir))
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
	var allDetailed []*inventory.DetailedWarehouseStockView
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID && item.OrganizationID == actor.OrganizationID {
				wh = item
				break
			}
		}
		if wh != nil {
			allDetailed, _ = h.invSvc.ListDetailedStocksByWarehouse(ctx, whID)
		}
	}

	if wh == nil {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	// Calculate overall warehouse metrics
	stats := pages.VendorWarehouseStats{
		TotalItems: len(allDetailed),
	}
	now := time.Now()
	sixMonths := now.AddDate(0, 6, 0)
	for _, s := range allDetailed {
		stats.TotalUnits += s.Quantity
		if s.Quantity <= s.MinThreshold && s.Quantity > 0 {
			stats.LowStockCount++
		} else if s.Quantity == 0 {
			stats.OutOfStockCount++
		}
		if s.ExpiryDate != nil {
			if s.ExpiryDate.Before(now) {
				stats.ExpiredCount++
			} else if s.ExpiryDate.Before(sixMonths) {
				stats.ExpiringSoonCount++
			}
		}
	}

	// Read filter and pagination params
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	stockStatus := r.URL.Query().Get("stock_status")
	expiryStatus := r.URL.Query().Get("expiry_status")
	sortParam := r.URL.Query().Get("sort")
	if sortParam == "" {
		sortParam = "updated_desc"
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
	}

	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil {
		switch l {
		case 10, 20, 50, 100:
			limit = l
		}
	}

	// Apply filtering
	var filtered []*inventory.DetailedWarehouseStockView
	for _, s := range allDetailed {
		if q != "" {
			qNorm := arabic.Normalize(q)
			qLower := strings.ToLower(q)
			nameNorm := arabic.Normalize(s.ProductName)
			nameLower := strings.ToLower(s.ProductName)
			varNorm := arabic.Normalize(s.VariantName)
			varLower := strings.ToLower(s.VariantName)
			sciLower := strings.ToLower(s.ScientificName)
			mfgLower := strings.ToLower(s.Manufacturer)
			skuLower := strings.ToLower(s.SKU)
			bcLower := strings.ToLower(s.Barcode)
			bnLower := strings.ToLower(s.BatchNumber)

			match := strings.Contains(nameNorm, qNorm) || strings.Contains(nameLower, qLower) ||
				strings.Contains(varNorm, qNorm) || strings.Contains(varLower, qLower) ||
				strings.Contains(sciLower, qLower) || strings.Contains(mfgLower, qLower) ||
				strings.Contains(skuLower, qLower) || strings.Contains(bcLower, qLower) ||
				strings.Contains(bnLower, qLower)

			if !match {
				continue
			}
		}

		if stockStatus == "available" && s.Quantity <= s.MinThreshold {
			continue
		} else if stockStatus == "low" && (s.Quantity <= 0 || s.Quantity > s.MinThreshold) {
			continue
		} else if stockStatus == "out_of_stock" && s.Quantity > 0 {
			continue
		}

		if expiryStatus == "expiring_soon" {
			if s.ExpiryDate == nil || s.ExpiryDate.Before(now) || s.ExpiryDate.After(sixMonths) {
				continue
			}
		} else if expiryStatus == "expired" {
			if s.ExpiryDate == nil || s.ExpiryDate.After(now) {
				continue
			}
		}

		filtered = append(filtered, s)
	}

	// Apply sorting
	sort.SliceStable(filtered, func(i, j int) bool {
		switch sortParam {
		case "name_asc":
			return strings.ToLower(filtered[i].ProductName) < strings.ToLower(filtered[j].ProductName)
		case "name_desc":
			return strings.ToLower(filtered[i].ProductName) > strings.ToLower(filtered[j].ProductName)
		case "qty_desc":
			return filtered[i].Quantity > filtered[j].Quantity
		case "qty_asc":
			return filtered[i].Quantity < filtered[j].Quantity
		case "expiry_asc":
			if filtered[i].ExpiryDate == nil {
				return false
			}
			if filtered[j].ExpiryDate == nil {
				return true
			}
			return filtered[i].ExpiryDate.Before(*filtered[j].ExpiryDate)
		case "updated_desc":
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		default:
			return filtered[i].StockID > filtered[j].StockID
		}
	})

	// Apply pagination
	totalFiltered := len(filtered)
	totalPages := (totalFiltered + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * limit
	end := start + limit
	if start > totalFiltered {
		start = totalFiltered
	}
	if end > totalFiltered {
		end = totalFiltered
	}

	var paged []*inventory.DetailedWarehouseStockView
	if start < end {
		paged = filtered[start:end]
	}

	fromItem := 0
	toItem := 0
	if totalFiltered > 0 {
		fromItem = start + 1
		toItem = end
	}

	data := pages.VendorWarehouseDetailData{
		Warehouse: wh,
		Stocks:    paged,
		Stats:     stats,
		Filter: pages.VendorWarehouseStockFilter{
			Query:        q,
			StockStatus:  stockStatus,
			ExpiryStatus: expiryStatus,
			Sort:         sortParam,
			Page:         page,
			PerPage:      limit,
		},
		TotalFiltered: totalFiltered,
		TotalPages:    totalPages,
		FromItem:      fromItem,
		ToItem:        toItem,
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMsg:     r.URL.Query().Get("msg"),
	}

	h.renderPage(ctx, w, "render vendor warehouse detail", pages.VendorWarehouseDetailPage(data, lang, dir))
}

// VendorWarehouseStockAdjustSubmit adjusts a stock level directly from warehouse detail page.
func (h *UIHandler) VendorWarehouseStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	whID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	stockID, _ := strconv.ParseInt(chi.URLParam(r, "stockID"), 10, 64)
	if whID <= 0 || stockID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", "بيانات النموذج غير صالحة.")
		return
	}

	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		reason = "تسوية جردية يدوية من شاشة المخزن"
	}

	if h.invSvc != nil {
		if newQtyStr := strings.TrimSpace(r.PostFormValue("new_quantity")); newQtyStr != "" {
			if newQty, err := strconv.Atoi(newQtyStr); err == nil && newQty >= 0 {
				stocks, _ := h.invSvc.ListStocksByWarehouse(ctx, whID)
				var currentStock *inventory.Stock
				for _, st := range stocks {
					if st.ID == stockID {
						currentStock = st
						break
					}
				}
				if currentStock != nil {
					delta := newQty - currentStock.Quantity
					if delta != 0 {
						_, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
							StockID: stockID,
							Delta:   delta,
							Type:    inventory.MovementAdjustment,
							Details: reason,
							UserID:  &actor.UserID,
						})
						if err != nil {
							h.log.ErrorContext(ctx, "adjust stock failed", "error", err)
							h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", "تعذر تعديل الرصيد: "+h.safeMessage(err, langOf(r)))
							return
						}
					}
				}
			}
		} else if deltaStr := strings.TrimSpace(r.PostFormValue("delta")); deltaStr != "" {
			if delta, err := strconv.Atoi(deltaStr); err == nil && delta != 0 {
				_, err := h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
					StockID: stockID,
					Delta:   delta,
					Type:    inventory.MovementAdjustment,
					Details: reason,
					UserID:  &actor.UserID,
				})
				if err != nil {
					h.log.ErrorContext(ctx, "adjust stock failed", "error", err)
					h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "error", "تعذر تعديل الرصيد: "+h.safeMessage(err, langOf(r)))
					return
				}
			}
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/warehouses/%d", whID), "success", "تم تحديث رصيد الصنف في المخزن بنجاح.")
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
