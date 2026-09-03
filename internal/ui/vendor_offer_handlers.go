package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOffersPage renders the Laravel-parity Special Offers management view.
func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	var offers []*promo.SpecialOffer
	if h.promoSvc != nil && actor.OrganizationID > 0 {
		offers, _ = h.promoSvc.ListSpecialOffersByOrg(ctx, actor.OrganizationID)
	}

	searchQ := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	var filtered []*promo.SpecialOffer
	for _, o := range offers {
		if o == nil {
			continue
		}
		if statusFilter != "" && statusFilter != "all" {
			if statusFilter == "pending" && o.AdminStatus != "pending" {
				continue
			} else if statusFilter == "changes_requested" && o.AdminStatus != "changes_requested" {
				continue
			} else if statusFilter == "rejected" && o.AdminStatus != "rejected" {
				continue
			} else if statusFilter == "approved" && o.AdminStatus != "approved" {
				continue
			} else if statusFilter != "pending" && statusFilter != "changes_requested" && statusFilter != "rejected" && statusFilter != "approved" {
				if o.Status != statusFilter {
					continue
				}
			}
		}
		if searchQ != "" {
			match := strings.Contains(strings.ToLower(o.Title.Get("ar")), searchQ) ||
				strings.Contains(strings.ToLower(o.Title.Get("en")), searchQ) ||
				strings.Contains(strings.ToLower(o.Description.Get("ar")), searchQ)
			if !match {
				continue
			}
		}
		filtered = append(filtered, o)
	}

	data := pages.VendorSpecialOffersData{
		Offers:       filtered,
		AllOffers:    offers,
		FilterStatus: statusFilter,
		SearchQuery:  r.URL.Query().Get("q"),
	}

	h.renderPage(ctx, w, "render vendor offers page", pages.VendorSpecialOffersPage(data, lang, dir))
}

func (h *UIHandler) loadVendorOfferItemOptions(ctx context.Context, orgID int64) ([]*org.Branch, []*catalog.ProductVariant, []pages.VendorOfferItemOption) {
	var branches []*org.Branch
	if h.orgSvc != nil && orgID > 0 {
		bList, err := h.orgSvc.ListBranches(ctx, orgID)
		if err != nil {
			h.log.WarnContext(ctx, "vendor offer: list branches", "error", err)
		} else {
			branches = bList
		}
	}

	var variants []*catalog.ProductVariant
	var itemOptions []pages.VendorOfferItemOption

	var warehouses []*inventory.Warehouse
	var stocks []*inventory.Stock
	whNameMap := make(map[int64]string)
	stockMap := make(map[int64]*inventory.Stock)

	if h.invSvc != nil && orgID > 0 {
		allWhs, _ := h.invSvc.ListWarehouses(ctx)
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, orgID)
		for _, wh := range allWhs {
			if wh != nil && wh.OrganizationID == orgID {
				warehouses = append(warehouses, wh)
				whNameMap[wh.ID] = wh.Name
			}
		}
		for _, s := range stocks {
			if s != nil {
				stockMap[s.ProductVariantID] = s
			}
		}
	}

	if h.catSvc != nil {
		vars, _, err := h.catSvc.ListVariantsByOrganization(ctx, orgID, catalog.VariantSearchParams{Limit: 500})
		if err == nil {
			variants = vars
		}
		if len(variants) == 0 {
			products, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
			if err == nil {
				for _, p := range products {
					pVars, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
					for _, v := range pVars {
						if v.OrganizationID == orgID || v.OrganizationID == 0 {
							variants = append(variants, v)
						}
					}
				}
			}
		}

		for _, v := range variants {
			if v == nil {
				continue
			}
			whName := ""
			stockQty := v.StockQty
			if s, ok := stockMap[v.ID]; ok && s != nil {
				stockQty = s.Quantity
				if n, exists := whNameMap[s.WarehouseID]; exists {
					whName = n
				}
			}
			expStr := ""
			if v.ExpiryDate != nil {
				expStr = v.ExpiryDate.Format("2006-01-02")
			}
			itemOptions = append(itemOptions, pages.VendorOfferItemOption{
				VariantID:      v.ID,
				NameAr:         v.Name["ar"],
				NameEn:         v.Name["en"],
				SKU:            v.SKU,
				BatchNumber:    v.BatchNumber,
				ExpiryDate:     expStr,
				Price:          v.Price.String(),
				PriceFloat:     float64(v.Price.Minor()) / 100.0,
				WarehouseName:  whName,
				AvailableStock: stockQty,
			})
		}
	}

	return branches, variants, itemOptions
}

// VendorOfferNewPage renders the special offer creation form.
func (h *UIHandler) VendorOfferNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers/new", http.StatusSeeOther)
		return
	}

	branches, variants, itemOptions := h.loadVendorOfferItemOptions(ctx, actor.OrganizationID)

	data := pages.VendorOfferFormData{
		Branches:    branches,
		Variants:    variants,
		ItemOptions: itemOptions,
		IsEdit:      false,
	}

	h.renderPage(ctx, w, "render vendor offer new page", pages.VendorOfferFormPage(data, lang, dir))
}

// VendorOfferEditPage renders the special offer edit form.
func (h *UIHandler) VendorOfferEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود")
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود أو ليس لديك صلاحية لتعديله")
		return
	}

	branches, variants, itemOptions := h.loadVendorOfferItemOptions(ctx, actor.OrganizationID)

	data := pages.VendorOfferFormData{
		Offer:       offer,
		Branches:    branches,
		Variants:    variants,
		ItemOptions: itemOptions,
		IsEdit:      true,
	}

	h.renderPage(ctx, w, "render vendor offer edit page", pages.VendorOfferFormPage(data, lang, dir))
}

// VendorOfferNewSubmit handles special offer creation with file uploads.
func (h *UIHandler) VendorOfferNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		_ = r.ParseForm()
	}

	titleAr := r.PostFormValue("title_ar")
	titleEn := r.PostFormValue("title_en")
	if titleEn == "" {
		titleEn = titleAr
	}
	descAr := r.PostFormValue("description_ar")
	status := r.PostFormValue("status")
	if status == "" {
		status = "active"
	}
	image := r.PostFormValue("image")
	if uploadedImg, err := saveUploadedFile(r, "image_file", "offers"); err == nil && uploadedImg != "" {
		image = uploadedImg
	}

	branchIDVal, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	var branchID *int64
	if branchIDVal > 0 {
		branchID = &branchIDVal
	}

	discPct, _ := strconv.ParseFloat(r.PostFormValue("discount_percentage"), 64)
	totalPriceFloat, _ := strconv.ParseFloat(r.PostFormValue("total_price"), 64)
	minOrderFloat, _ := strconv.ParseFloat(r.PostFormValue("min_order_amount"), 64)

	var startPtr, endPtr *time.Time
	if sDate := r.PostFormValue("start_date"); sDate != "" {
		if t, err := time.Parse("2006-01-02", sDate); err == nil {
			startPtr = &t
		}
	}
	if eDate := r.PostFormValue("end_date"); eDate != "" {
		if t, err := time.Parse("2006-01-02", eDate); err == nil {
			endPtr = &t
		}
	}

	var prods []*promo.SpecialOfferProduct
	for _, vIDStr := range r.Form["selected_variants"] {
		vID, _ := strconv.ParseInt(vIDStr, 10, 64)
		if vID > 0 {
			qty, _ := strconv.Atoi(r.PostFormValue(fmt.Sprintf("qty_%d", vID)))
			if qty <= 0 {
				qty = 1
			}
			customPrice, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("custom_price_%d", vID)), 64)
			pDiscPct, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("discount_pct_%d", vID)), 64)

			prods = append(prods, &promo.SpecialOfferProduct{
				VariantID:          vID,
				Quantity:           qty,
				CustomPrice:        money.FromMinor(int64(customPrice * 100)),
				DiscountPercentage: pDiscPct,
			})
		}
	}

	o := &promo.SpecialOffer{
		OrganizationID:     actor.OrganizationID,
		BranchID:           branchID,
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(descAr, descAr),
		DiscountPercentage: discPct,
		TotalPrice:         money.FromMinor(int64(totalPriceFloat * 100)),
		MinOrderAmount:     money.FromMinor(int64(minOrderFloat * 100)),
		StartDate:          startPtr,
		EndDate:            endPtr,
		Status:             status,
		AdminStatus:        "pending",
		Image:              image,
		Products:           prods,
	}

	if h.promoSvc != nil {
		if _, err := h.promoSvc.CreateSpecialOffer(ctx, o); err != nil {
			h.log.ErrorContext(ctx, "create special offer error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/offers/new", "error", h.safeMessage(err, langOf(r)))
			return
		}

		go h.dispatchInAppNotification(context.Background(), actor.UserID, &actor.OrganizationID,
			i18n.T(langOf(r), "vendor.offer.created_notification_title"),
			fmt.Sprintf(i18n.T(langOf(r), "vendor.offer.created_notification_body"), titleAr))
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", "تم إنشاء العرض بنجاح وإرساله للمراجعة والاعتماد من قِبل الإدارة.")
}

// VendorOfferEditSubmit processes special offer updates with file uploads.
func (h *UIHandler) VendorOfferEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود")
		return
	}

	existing, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "العرض غير موجود أو ليس لديك صلاحية لتعديله")
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		_ = r.ParseForm()
	}

	titleAr := r.PostFormValue("title_ar")
	titleEn := r.PostFormValue("title_en")
	if titleEn == "" {
		titleEn = titleAr
	}
	descAr := r.PostFormValue("description_ar")
	status := r.PostFormValue("status")
	if status == "" {
		status = "active"
	}
	image := existing.Image
	if formImg := r.PostFormValue("image"); formImg != "" {
		image = formImg
	}
	if uploadedImg, err := saveUploadedFile(r, "image_file", "offers"); err == nil && uploadedImg != "" {
		image = uploadedImg
	}

	branchIDVal, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	var branchID *int64
	if branchIDVal > 0 {
		branchID = &branchIDVal
	}

	discPct, _ := strconv.ParseFloat(r.PostFormValue("discount_percentage"), 64)
	totalPriceFloat, _ := strconv.ParseFloat(r.PostFormValue("total_price"), 64)
	minOrderFloat, _ := strconv.ParseFloat(r.PostFormValue("min_order_amount"), 64)

	var startPtr, endPtr *time.Time
	if sDate := r.PostFormValue("start_date"); sDate != "" {
		if t, err := time.Parse("2006-01-02", sDate); err == nil {
			startPtr = &t
		}
	}
	if eDate := r.PostFormValue("end_date"); eDate != "" {
		if t, err := time.Parse("2006-01-02", eDate); err == nil {
			endPtr = &t
		}
	}

	var prods []*promo.SpecialOfferProduct
	for _, vIDStr := range r.Form["selected_variants"] {
		vID, _ := strconv.ParseInt(vIDStr, 10, 64)
		if vID > 0 {
			qty, _ := strconv.Atoi(r.PostFormValue(fmt.Sprintf("qty_%d", vID)))
			if qty <= 0 {
				qty = 1
			}
			customPrice, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("custom_price_%d", vID)), 64)
			pDiscPct, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("discount_pct_%d", vID)), 64)

			prods = append(prods, &promo.SpecialOfferProduct{
				OfferID:            existing.ID,
				VariantID:          vID,
				Quantity:           qty,
				CustomPrice:        money.FromMinor(int64(customPrice * 100)),
				DiscountPercentage: pDiscPct,
			})
		}
	}

	existing.BranchID = branchID
	existing.Title = i18n.New(titleAr, titleEn)
	existing.Description = i18n.New(descAr, descAr)
	existing.DiscountPercentage = discPct
	existing.TotalPrice = money.FromMinor(int64(totalPriceFloat * 100))
	existing.MinOrderAmount = money.FromMinor(int64(minOrderFloat * 100))
	existing.StartDate = startPtr
	existing.EndDate = endPtr
	existing.Status = status
	existing.AdminStatus = "pending"
	existing.Image = image
	existing.Products = prods

	if err := h.promoSvc.UpdateSpecialOffer(ctx, existing); err != nil {
		h.log.ErrorContext(ctx, "update special offer error", "error", err)
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/offers/%d/edit", existing.ID), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", "تم تحديث بيانات العرض بنجاح وإرساله للمراجعة والاعتماد الإداري.")
}

// VendorOfferLocationsPage renders geographic location coverage management for an offer.
func (h *UIHandler) VendorOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		http.Redirect(w, r, "/vendor/offers", http.StatusSeeOther)
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)

	data := pages.VendorOfferLocationsData{
		Offer:     offer,
		Locations: locs,
		Cities:    h.listCities(ctx),
	}

	h.renderPage(ctx, w, "render vendor offer locations page", pages.VendorOfferLocationsPage(data, lang, dir))
}

// VendorOfferLocationNewSubmit adds a geographic coverage location to an offer.
func (h *UIHandler) VendorOfferLocationNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("latitude"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("longitude"), 64)
	radius, _ := strconv.Atoi(r.PostFormValue("radius"))
	if radius <= 0 {
		radius = 500
	}
	day, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	if day <= 0 {
		day = 1
	}

	loc := &promo.SpecialOfferLocation{
		OfferID:     offerID,
		CityID:      cityID,
		AddressAr:   r.PostFormValue("address_ar"),
		AddressEn:   r.PostFormValue("address_ar"),
		Latitude:    lat,
		Longitude:   lon,
		Radius:      radius,
		DayOfWeek:   day,
		TimeFrom:    r.PostFormValue("time_from"),
		TimeTo:      r.PostFormValue("time_to"),
		Status:      "active",
		AdminStatus: "approved",
	}

	if h.promoSvc != nil {
		_ = h.promoSvc.AddSpecialOfferLocation(ctx, loc)
	}
	_ = actor

	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/offers/%d/locations", offerID), "success", i18n.T(lang, "vendor.offer.location_added_success"))
}

// VendorOfferDeleteSubmit deletes a special offer.
func (h *UIHandler) VendorOfferDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.promoSvc != nil && id > 0 {
		_ = h.promoSvc.DeleteSpecialOffer(ctx, id, actor.OrganizationID)
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", i18n.T(lang, "vendor.offer.deleted_success"))
}
