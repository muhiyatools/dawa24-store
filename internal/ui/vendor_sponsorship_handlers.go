package ui

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) loadVendorInStockItems(ctx context.Context, orgID int64) []pages.VendorOfferItemOption {
	var itemOptions []pages.VendorOfferItemOption
	if orgID <= 0 {
		return itemOptions
	}
	var warehouses []*inventory.Warehouse
	var stocks []*inventory.Stock
	whNameMap := make(map[int64]string)
	stockMap := make(map[int64]*inventory.Stock)

	if h.invSvc != nil {
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, orgID)
		for _, wh := range warehouses {
			if wh != nil {
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
		variants, _, err := h.catSvc.ListVariantsByOrganization(ctx, orgID, catalog.VariantSearchParams{Limit: 500})
		if err == nil {
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
				if stockQty <= 0 {
					continue
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
	}
	return itemOptions
}

// VendorSponsorshipRequestsPage renders the vendor's sponsorship requests list
// and the package purchase form.
func (h *UIHandler) VendorSponsorshipRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/sponsorship-requests", http.StatusSeeOther)
		return
	}

	var packages []*promo.OfferPackage
	var purchases []*promo.SponsorshipPurchase
	var requests []*promo.SponsorshipRequest
	var activePurchases []*promo.SponsorshipPurchase
	var activeOffers []*promo.Offer

	if h.promoSvc != nil {
		packages, _ = h.promoSvc.ListPackages(ctx)
		purchases, _ = h.promoSvc.ListSponsorshipPurchases(ctx)
		requests, _ = h.promoSvc.ListSponsorshipRequestsByOrg(ctx, 100, 0)
		activePurchases, _ = h.promoSvc.ListActiveSponsorshipPurchases(ctx)
		activeOffers, _ = h.promoSvc.ListOffers(ctx, promo.OfferFilter{IsActive: boolPtr(true), Limit: 100})
	}

	totalCredits := 0
	for _, p := range activePurchases {
		if p != nil {
			totalCredits += p.CreditsRemainingInt()
		}
	}

	itemOptions := h.loadVendorInStockItems(ctx, actor.OrganizationID)

	data := pages.SponsorshipRequestsData{
		Packages:        packages,
		Purchases:       purchases,
		ActivePurchases: activePurchases,
		Requests:        requests,
		OrgID:           actor.OrganizationID,
		ItemOptions:     itemOptions,
		ActiveOffers:    activeOffers,
		TotalCredits:    totalCredits,
	}

	h.renderPage(ctx, w, "render vendor sponsorship requests", pages.VendorSponsorshipRequestsPage(lang, dir, data))
}

// VendorSponsorshipRequestSubmit handles batch or single sponsorship request submission.
func (h *UIHandler) VendorSponsorshipRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	itemType := strings.TrimSpace(r.PostFormValue("item_type"))
	if itemType != "product" && itemType != "offer" {
		itemType = "product"
	}

	packageID, err := strconv.ParseInt(r.PostFormValue("package_id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", "يرجى اختيار باقة رعاية تحتوي على رصيد كافٍ.")
		return
	}

	// Extract item IDs (supports multiple item_ids inputs or single item_id)
	var itemIDs []int64
	for _, raw := range r.PostForm["item_ids"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if id, parseErr := strconv.ParseInt(part, 10, 64); parseErr == nil && id > 0 {
				itemIDs = append(itemIDs, id)
			}
		}
	}
	if len(itemIDs) == 0 {
		if singleID, parseErr := strconv.ParseInt(r.PostFormValue("item_id"), 10, 64); parseErr == nil && singleID > 0 {
			itemIDs = append(itemIDs, singleID)
		}
	}

	if len(itemIDs) == 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", "يرجى اختيار عنصر واحد على الأقل للرعاية.")
		return
	}

	created, err := h.promoSvc.SubmitBatchSponsorshipRequests(ctx, promo.SponsorshipItemType(itemType), itemIDs, packageID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success",
		"تم تقديم "+strconv.Itoa(len(created))+" طلب رعاية بنجاح وخصم "+strconv.Itoa(len(created))+" رصيد من باقتك، وهي قيد المراجعة.")
}

// VendorSponsorshipRequestCancelSubmit cancels a pending sponsorship request.
func (h *UIHandler) VendorSponsorshipRequestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.invalid_request_id"))
		return
	}

	if err := h.promoSvc.CancelSponsorshipRequest(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.request_cancelled_success"))
}

// VendorSponsorshipPackagePurchaseSubmit handles the purchase of a sponsorship package.
func (h *UIHandler) VendorSponsorshipPackagePurchaseSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	packageID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.invalid_package_id"))
		return
	}

	_, err = h.promoSvc.PurchasePackage(ctx, packageID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.package_purchased_success"))
}

// VendorAdCreateSubmit handles the creation of a new advertisement with direct media upload.
func (h *UIHandler) VendorAdCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	// Max 20MB for direct image/video uploads
	_ = r.ParseMultipartForm(20 << 20)

	ad := h.parseAdForm(r, actor.OrganizationID)

	// Check if a direct file was uploaded
	if file, header, fileErr := r.FormFile("media_file"); fileErr == nil && file != nil {
		defer file.Close()
		if data, readErr := io.ReadAll(file); readErr == nil && len(data) > 0 {
			if u, saveErr := saveUploadedBytes(data, header.Filename, "ads"); saveErr == nil && u != "" {
				ad.MediaURL = u
				if strings.HasSuffix(strings.ToLower(header.Filename), ".mp4") || strings.HasSuffix(strings.ToLower(header.Filename), ".webm") {
					ad.MediaType = promo.MediaVideo
				} else {
					ad.MediaType = promo.MediaImage
				}
			}
		}
	}

	created, err := h.promoSvc.CreateAd(ctx, ad)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	_ = created
	h.redirectWithNotice(w, r, "/vendor/ads", "success", "تم إنشاء الإعلان الترويجي بنجاح وخصم 2 رصيد رعاية، وهو الآن قيد مراجعة الإدارة.")
}

// VendorAdUpdateSubmit handles the update of an existing advertisement.
func (h *UIHandler) VendorAdUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "vendor.ads.invalid_ad_id"))
		return
	}

	_ = r.ParseMultipartForm(20 << 20)
	ad := h.parseAdForm(r, actor.OrganizationID)
	ad.ID = id

	if file, header, fileErr := r.FormFile("media_file"); fileErr == nil && file != nil {
		defer file.Close()
		if data, readErr := io.ReadAll(file); readErr == nil && len(data) > 0 {
			if u, saveErr := saveUploadedBytes(data, header.Filename, "ads"); saveErr == nil && u != "" {
				ad.MediaURL = u
			}
		}
	}

	if err := h.promoSvc.UpdateAd(ctx, ad); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/ads", "success", i18n.T(lang, "vendor.ads.updated_success"))
}

func (h *UIHandler) parseAdForm(r *http.Request, orgID int64) *promo.Ad {
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	adTextAr := strings.TrimSpace(r.PostFormValue("ad_text_ar"))
	adTextEn := strings.TrimSpace(r.PostFormValue("ad_text_en"))
	mediaURL := strings.TrimSpace(r.PostFormValue("media_url"))
	thumbnailURL := strings.TrimSpace(r.PostFormValue("thumbnail_url"))
	mediaType := strings.TrimSpace(r.PostFormValue("media_type"))
	clickTarget := strings.TrimSpace(r.PostFormValue("click_target_type"))
	targetID := strings.TrimSpace(r.PostFormValue("click_target_id"))
	position := strings.TrimSpace(r.PostFormValue("position"))
	if position == "" {
		position = promo.PositionHomeHero
	}
	if mediaType == "" {
		mediaType = "image"
	}
	if clickTarget == "" {
		clickTarget = "product"
	}

	var clickTargetID *int64
	if targetID != "" {
		if id, err := strconv.ParseInt(targetID, 10, 64); err == nil && id > 0 {
			clickTargetID = &id
		}
	}

	durationDays := 30
	if d, err := strconv.Atoi(r.PostFormValue("duration_days")); err == nil && d > 0 {
		durationDays = d
	}

	title := titleAr
	if title == "" {
		title = titleEn
	}

	return &promo.Ad{
		OrganizationID:  &orgID,
		Title:           title,
		TitleAr:         titleAr,
		TitleEn:         titleEn,
		AdTextAr:        adTextAr,
		AdTextEn:        adTextEn,
		MediaType:       promo.AdMediaType(mediaType),
		MediaURL:        mediaURL,
		ThumbnailURL:    thumbnailURL,
		Position:        position,
		ClickTargetType: promo.AdClickTarget(clickTarget),
		ClickTargetID:   clickTargetID,
		DurationDays:    durationDays,
		IsActive:        false,
	}
}

func (h *UIHandler) localeAndDirLang(r *http.Request) string {
	lang, _ := h.localeAndDir(r)
	return lang
}
