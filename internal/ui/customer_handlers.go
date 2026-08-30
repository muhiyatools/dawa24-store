package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// 1. Security & Anti-Scraping / Bot Defense
	// Honeypot trap check: if filled by a scraper/bot, return empty page safely
	if botTrap := r.URL.Query().Get("company_tax_ref"); botTrap != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(pages.CatalogPageData{
			Page:     1,
			PageSize: 24,
			ViewMode: "grid",
		}, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	// Sanitize and cap search query string to prevent ReDoS / query stuffing
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 80 {
		query = query[:80]
	}

	var categoryID *int64
	if v := r.URL.Query().Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			categoryID = &id
		}
	}

	var brandID *int64
	if v := r.URL.Query().Get("brand_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			brandID = &id
		}
	}

	var minPrice, maxPrice *money.Amount
	minPriceStr := r.URL.Query().Get("min_price")
	maxPriceStr := r.URL.Query().Get("max_price")
	if minPriceStr != "" {
		if a, err := money.Parse(minPriceStr); err == nil && a.IsPositive() {
			minPrice = &a
		}
	}
	if maxPriceStr != "" {
		if a, err := money.Parse(maxPriceStr); err == nil && a.IsPositive() {
			maxPrice = &a
		}
	}

	dosageForm := strings.TrimSpace(r.URL.Query().Get("dosage_form"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	inStock := r.URL.Query().Get("in_stock") == "true"
	hasDiscount := r.URL.Query().Get("has_discount") == "true"
	viewMode := r.URL.Query().Get("view")
	if viewMode != "table" && viewMode != "grid" {
		viewMode = "grid"
	}

	// 2. Rows per page (PageSize) & Page bounds enforcement
	pageSize := 24
	if psVal := r.URL.Query().Get("page_size"); psVal != "" {
		if ps, err := strconv.Atoi(psVal); err == nil {
			switch ps {
			case 12, 24, 48, 96:
				pageSize = ps
			default:
				pageSize = 24
			}
		}
	}

	page := 1
	if pVal := r.URL.Query().Get("page"); pVal != "" {
		if p, err := strconv.Atoi(pVal); err == nil && p >= 1 {
			page = p
		}
	}
	// Bound page depth to 200 to prevent database exhaustion by scrapers
	if page > 200 {
		page = 200
	}

	if h.catSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCatalog(pages.CatalogPageData{
			Query:    query,
			Page:     page,
			PageSize: pageSize,
			ViewMode: viewMode,
		}, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer catalog", "error", err)
		}
		return
	}

	offset := (page - 1) * pageSize

	// Search products with total count for accurate server-side pagination
	products, totalCount, err := h.catSvc.SearchWithTotal(ctx, catalog.SearchParams{
		Query:      query,
		CategoryID: categoryID,
		BrandID:    brandID,
		DosageForm: dosageForm,
		Sort:       sortBy,
		MinPrice:   minPrice,
		MaxPrice:   maxPrice,
		Limit:      pageSize,
		Offset:     offset,
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	categories, _ := h.catSvc.ListCategories(ctx)
	brands, _ := h.catSvc.ListBrands(database.AsSystem(ctx))
	brandMap := make(map[int64]*catalog.Brand)
	for _, b := range brands {
		if b != nil {
			brandMap[b.ID] = b
		}
	}

	// Resolve active category and brand names for active filter tags
	activeCatName := ""
	if categoryID != nil {
		for _, cat := range categories {
			if cat != nil && cat.ID == *categoryID {
				activeCatName = cat.Name["ar"]
				if activeCatName == "" {
					activeCatName = cat.Name["en"]
				}
				break
			}
		}
	}

	activeBrandName := ""
	if brandID != nil {
		if b, ok := brandMap[*brandID]; ok && b != nil {
			activeBrandName = b.Name.Get(i18n.AR)
			if activeBrandName == "" {
				activeBrandName = b.Name.Get(i18n.EN)
			}
		}
	}

	// Batch-prefetch variants for current page slice
	filtered := make([]*catalog.Product, 0, len(products))
	for _, p := range products {
		if p == nil {
			continue
		}
		if dosageForm != "" && !strings.Contains(strings.ToLower(p.DosageForm), strings.ToLower(dosageForm)) {
			continue
		}
		filtered = append(filtered, p)
	}

	productIDs := make([]int64, 0, len(filtered))
	for _, p := range filtered {
		productIDs = append(productIDs, p.ID)
	}

	variantsByProduct := make(map[int64][]*catalog.ProductVariant)
	if h.catSvc != nil && len(productIDs) > 0 {
		if grouped, err := h.catSvc.ListVariantsByProducts(ctx, productIDs); err == nil && grouped != nil {
			variantsByProduct = grouped
		}
	}
	env := h.buildOfferEnv(ctx, productIDs, variantsByProduct)

	var variantCards []*pages.SupplierVariantCard

	for _, p := range filtered {
		variants := variantsByProduct[p.ID]

		var pBrandID *int64
		var pBrandName string
		var pBrandLogo string
		if p.BrandID != nil {
			pBrandID = p.BrandID
			if b, found := brandMap[*p.BrandID]; found && b != nil {
				pBrandName = b.Name.Get(i18n.AR)
				if pBrandName == "" {
					pBrandName = b.Name.Get(i18n.EN)
				}
				pBrandLogo = b.Image
			}
		}
		if pBrandName == "" && p.ManufacturingCompanies != "" {
			pBrandName = p.ManufacturingCompanies
		}

		offers := h.offersForProduct(ctx, p, variants, env)

		if len(offers) > 0 {
			for _, off := range offers {
				if inStock && off.AvailableStock <= 0 {
					continue
				}
				if hasDiscount && off.DiscountBPS <= 0 {
					continue
				}
				if minPrice != nil && off.Price.Minor() < minPrice.Minor() {
					continue
				}
				if maxPrice != nil && off.Price.Minor() > maxPrice.Minor() {
					continue
				}

				// Find variant unit name
				varUnitName := ""
				varSKU := ""
				for _, v := range variants {
					if v != nil && v.ID == off.VariantID {
						varUnitName = v.Name["ar"]
						if varUnitName == "" {
							varUnitName = v.Name["en"]
						}
						varSKU = v.SKU
						break
					}
				}

				discPct := int(off.DiscountBPS / 100)

				variantCards = append(variantCards, &pages.SupplierVariantCard{
					VariantID:       off.VariantID,
					ProductID:       p.ID,
					ProductNameAr:   p.Name.Get(i18n.AR),
					ProductNameEn:   p.Name.Get(i18n.EN),
					ProductImage:    p.Image,
					VariantName:     varUnitName,
					SKU:             varSKU,
					DosageForm:      p.DosageForm,
					Manufacturer:    p.ManufacturingCompanies,
					BrandID:         pBrandID,
					BrandName:       pBrandName,
					BrandLogo:       pBrandLogo,
					ScientificName:  p.ScientificName,
					PublicPrice:     p.Price,
					SupplierID:      off.SupplierID,
					SupplierName:    off.SupplierName,
					SupplierRating:  off.SupplierRating,
					IsVerified:      off.IsVerified,
					BranchName:      off.BranchName,
					CityName:        off.CityName,
					GovernorateName: off.GovernorateName,
					DistanceKM:      off.DistanceKM,
					DistanceText:    off.DistanceText,
					Price:           off.Price,
					OriginalPrice:   off.OldPrice,
					DiscountPercent: discPct,
					AvailableStock:  off.AvailableStock,
					MinOrderQty:     off.MinOrderQty,
					ExpiryDate:      off.ExpiryDate,
					IsCovered:       off.IsCovered,
					CoverageReason:  off.CoverageReason,
					CanAddToCart:    off.CanAddToCart,
					IsNegotiable:    off.IsNegotiable,
				})
			}
		} else {
			// Master product placeholder when no active offer
			if !inStock && !hasDiscount {
				variantCards = append(variantCards, &pages.SupplierVariantCard{
					ProductID:      p.ID,
					ProductNameAr:  p.Name.Get(i18n.AR),
					ProductNameEn:  p.Name.Get(i18n.EN),
					ProductImage:   p.Image,
					DosageForm:     p.DosageForm,
					Manufacturer:   p.ManufacturingCompanies,
					BrandID:        pBrandID,
					BrandName:      pBrandName,
					BrandLogo:      pBrandLogo,
					ScientificName: p.ScientificName,
					PublicPrice:    p.Price,
					Price:          p.Price,
					SupplierName:   "طلب توريد خاص",
					IsCovered:      false,
					CoverageReason: "لا تتوفر عروض توريد نشطة لهذا الصنف حالياً",
					CanAddToCart:   false,
				})
			}
		}
	}

	// Sponsorship ranking: fetch which products are sponsored and their tier.
	// Sponsored products appear at the top with a "Sponsored" tag, ranked by
	// package tier level (highest first). Ties at the same tier are randomized.
	sponsoredProductIDs := make(map[int64]bool)
	if h.promoSvc != nil && len(productIDs) > 0 {
		rankings, err := h.promoSvc.RankedSponsorshipsForProducts(ctx, productIDs)
		if err == nil {
			for _, rs := range rankings {
				if rs != nil {
					sponsoredProductIDs[rs.ItemID] = true
				}
			}
		}
	}
	for _, vc := range variantCards {
		if vc != nil && sponsoredProductIDs[vc.ProductID] {
			vc.IsSponsored = true
		}
	}

	// 3. Prioritize variant cards: Sponsored first, then In-Stock & Covered, then Proximity
	sort.SliceStable(variantCards, func(i, j int) bool {
		if variantCards[i].IsSponsored != variantCards[j].IsSponsored {
			return variantCards[i].IsSponsored
		}
		if variantCards[i].IsSponsored && variantCards[j].IsSponsored {
			return variantCards[i].ProductID < variantCards[j].ProductID
		}
		// Tier 1: Actionable (In-stock & Covered)
		if variantCards[i].CanAddToCart != variantCards[j].CanAddToCart {
			return variantCards[i].CanAddToCart
		}
		if (variantCards[i].AvailableStock > 0) != (variantCards[j].AvailableStock > 0) {
			return variantCards[i].AvailableStock > 0
		}
		if variantCards[i].IsCovered != variantCards[j].IsCovered {
			return variantCards[i].IsCovered
		}

		// Tier 2: User sort or Proximity to Client
		switch sortBy {
		case "price_asc":
			if variantCards[i].Price.Minor() != variantCards[j].Price.Minor() {
				return variantCards[i].Price.Minor() < variantCards[j].Price.Minor()
			}
		case "price_desc":
			if variantCards[i].Price.Minor() != variantCards[j].Price.Minor() {
				return variantCards[i].Price.Minor() > variantCards[j].Price.Minor()
			}
		case "discount", "discount_desc":
			if variantCards[i].DiscountPercent != variantCards[j].DiscountPercent {
				return variantCards[i].DiscountPercent > variantCards[j].DiscountPercent
			}
		case "name":
			return variantCards[i].ProductNameAr < variantCards[j].ProductNameAr
		case "newest":
			return variantCards[i].ProductID > variantCards[j].ProductID
		default:
			// Default / "nearby": Nearest vendor first!
			if variantCards[i].DistanceKM > 0 && variantCards[j].DistanceKM > 0 && math.Abs(variantCards[i].DistanceKM-variantCards[j].DistanceKM) > 1.0 {
				return variantCards[i].DistanceKM < variantCards[j].DistanceKM
			}
			if variantCards[i].DistanceKM > 0 && variantCards[j].DistanceKM <= 0 {
				return true
			}
			if variantCards[i].DistanceKM <= 0 && variantCards[j].DistanceKM > 0 {
				return false
			}
			if variantCards[i].DiscountPercent != variantCards[j].DiscountPercent {
				return variantCards[i].DiscountPercent > variantCards[j].DiscountPercent
			}
		}

		return variantCards[i].ProductID < variantCards[j].ProductID
	})

	// 3. Compute pagination metrics
	totalPages := 1
	if totalCount > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	if totalPages < 1 {
		totalPages = 1
	}

	startItem := 0
	endItem := 0
	if totalCount > 0 {
		startItem = offset + 1
		endItem = offset + len(variantCards)
		if endItem > totalCount {
			endItem = totalCount
		}
	}

	viewData := pages.CatalogPageData{
		Variants:       variantCards,
		Categories:     categories,
		Brands:         brands,
		Query:          query,
		CategoryID:     categoryID,
		BrandID:        brandID,
		MinPrice:       minPriceStr,
		MaxPrice:       maxPriceStr,
		DosageForm:     dosageForm,
		Sort:           sortBy,
		InStock:        inStock,
		HasDiscount:    hasDiscount,
		ViewMode:       viewMode,
		Page:           page,
		PageSize:       pageSize,
		TotalItems:     totalCount,
		TotalPages:     totalPages,
		HasPrev:        page > 1,
		HasNext:        page < totalPages,
		PrevPage:       page - 1,
		NextPage:       page + 1,
		StartItem:      startItem,
		EndItem:        endItem,
		ActiveCategory: activeCatName,
		ActiveBrand:    activeBrandName,
		SponsoredProductIDs: sponsoredProductIDs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCatalog(viewData, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render catalog page", "error", err)
	}
}

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.catSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	// Try lookup as variant first, then fallback to product
	var targetProductID int64
	var targetVariantID int64

	if variant, err := h.catSvc.GetVariant(ctx, id); err == nil && variant != nil {
		targetProductID = variant.ProductID
		targetVariantID = variant.ID
	} else {
		targetProductID = id
	}

	product, variants, err := h.catSvc.GetProduct(ctx, targetProductID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	env := h.buildOfferEnv(ctx,
		[]int64{product.ID},
		map[int64][]*catalog.ProductVariant{product.ID: variants},
	)
	offers := h.offersForProduct(ctx, product, variants, env)

	// If target variant was specified, prioritize its offer to the top
	if targetVariantID > 0 && len(offers) > 1 {
		for i, off := range offers {
			if off.VariantID == targetVariantID && i > 0 {
				targetOffer := offers[i]
				offers = append([]pages.SupplierOffer{targetOffer}, append(offers[:i], offers[i+1:]...)...)
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerProductDetail(product, variants, offers, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render product detail page", "error", err)
	}
}

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، الشراء وسلة الطلبات متاحة حصرياً للصيدليات المرخصة.")
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCart(nil, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render cart page", "error", err)
		}
		return
	}

	cart, err := h.commSvc.GetCart(ctx, actor.UserID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if cart != nil && len(cart.Items) > 0 {
		branchID := h.pharmacyBranchID(ctx, &actor)
		for _, it := range cart.Items {
			it.IsCovered = true
			if branchID > 0 && it.ProductVariantID > 0 && it.OrganizationID > 0 {
				res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
					VariantID:        it.ProductVariantID,
					VendorOrgID:      it.OrganizationID,
					CustomerOrgID:    actor.OrganizationID,
					CustomerBranchID: branchID,
					Quantity:         it.Quantity,
					When:             time.Now(),
				})
				if err == nil {
					if !res.Allowed {
						if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation {
							it.IsCovered = false
							it.CoverageReason = "خارج نطاق التغطية للفرع المحدد"
						} else if res.Reason == commerce.ReasonOutOfStock || res.Reason == commerce.ReasonInsufficientStock {
							it.CoverageReason = "نفد المخزون لدى المورد"
						}
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCart(cart, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cart page", "error", err)
	}
}

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، إتمام الشراء والتوريد متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCheckout(nil, nil, lang, dir).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render checkout page", "error", err)
		}
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "checkout: list customer branches", "error", err)
		} else {
			branches = bList
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCheckout(cart, branches, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render checkout page", "error", err)
	}
}

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerOrders(nil, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer orders page", "error", err)
		}
		return
	}

	orders, err := h.commSvc.ListCustomerOrders(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrders(orders, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer orders page", "error", err)
	}
}

func (h *UIHandler) CustomerOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.commSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	history, _ := h.commSvc.GetOrderHistory(ctx, id)

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrderDetail(order, history, noticeType, noticeMsg, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer order detail page", "error", err)
	}
}

// CustomerOrderEditSubmit handles customer edits to quantities and items of a pending order.
func (h *UIHandler) CustomerOrderEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/orders", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", "خدمة إدارة الطلبات غير متوفرة حالياً.")
		return
	}

	_ = r.ParseForm()

	lineIDs := r.PostForm["line_id[]"]
	productNames := r.PostForm["product_name[]"]
	quantities := r.PostForm["quantity[]"]
	unitPrices := r.PostForm["unit_price[]"]
	discountAmounts := r.PostForm["discount_amount[]"]
	isDeletedList := r.PostForm["is_deleted[]"]

	lineCount := len(lineIDs)
	if len(productNames) > lineCount {
		lineCount = len(productNames)
	}
	if len(quantities) > lineCount {
		lineCount = len(quantities)
	}

	var lines []commerce.OrderLineEditItem
	for i := 0; i < lineCount; i++ {
		var lineID int64
		if i < len(lineIDs) {
			lineID, _ = strconv.ParseInt(lineIDs[i], 10, 64)
		}

		pName := ""
		if i < len(productNames) {
			pName = strings.TrimSpace(productNames[i])
		}

		qty := 1
		if i < len(quantities) {
			if qVal, err := strconv.Atoi(quantities[i]); err == nil && qVal > 0 {
				qty = qVal
			}
		}

		uPrice := money.Zero
		if i < len(unitPrices) {
			if parsed, err := money.Parse(unitPrices[i]); err == nil {
				uPrice = parsed
			}
		}

		dAmount := money.Zero
		if i < len(discountAmounts) {
			if parsed, err := money.Parse(discountAmounts[i]); err == nil {
				dAmount = parsed
			}
		}

		isDel := false
		if i < len(isDeletedList) {
			isDel = isDeletedList[i] == "true" || isDeletedList[i] == "1"
		}

		if lineID <= 0 && pName == "" && !isDel {
			continue
		}

		lines = append(lines, commerce.OrderLineEditItem{
			ID:             lineID,
			ProductName:    pName,
			Quantity:       qty,
			UnitPrice:      uPrice,
			DiscountAmount: dAmount,
			IsDeleted:      isDel,
		})
	}

	input := commerce.UpdateCustomerOrderInput{
		OrderID: id,
		Lines:   lines,
		Notes:   strings.TrimSpace(r.PostFormValue("notes")),
	}

	updatedOrder, err := h.commSvc.UpdateCustomerPendingOrder(ctx, actor, input)
	if err != nil {
		errMsg := "تعذر تعديل الطلب: " + h.safeMessage(err, langOf(r))
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   errMsg,
			})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
		return
	}

	if updatedOrder != nil {
		pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
		orderNum := updatedOrder.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", updatedOrder.ID)
		}
		for _, sh := range updatedOrder.Shipments {
			if sh != nil && sh.OrganizationID > 0 {
				go h.dispatchOrgNotification(context.Background(), sh.OrganizationID,
					fmt.Sprintf("تعديل على الطلب #%s", orderNum),
					fmt.Sprintf("قامت صيدلية %s بتعديل كميات وأصناف الطلب #%s بقيمة جديدة %s ج.م.", pharmacyName, orderNum, updatedOrder.TotalAmount.String()))
			}
		}
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"message":     "تم حفظ وتعديل بيانات الطلب وتحديث الإجماليات بنجاح.",
			"subtotal":    updatedOrder.Subtotal.String(),
			"discount":    updatedOrder.DiscountAmount.String(),
			"total":       updatedOrder.TotalAmount.String(),
			"shippingFee": updatedOrder.ShippingFee.String(),
			"taxAmount":   updatedOrder.TaxAmount.String(),
		})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", "تم حفظ وتعديل بيانات الطلب وتحديث الإجماليات بنجاح.")
}

func (h *UIHandler) NotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/notifications", http.StatusSeeOther)
		return
	}

	if h.notifSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.Notifications(nil, 0, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render notifications page", "error", err)
		}
		return
	}

	logs, err := h.notifSvc.ListUserNotifications(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	unread, _ := h.notifSvc.GetUnreadCount(ctx, userID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Notifications(logs, unread, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render notifications page", "error", err)
	}
}

func (h *UIHandler) AddToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/cart")
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"يرجى تسجيل الدخول كصيدلية مرخصة للشراء","type":"error"}}`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"عذراً، إضافة الأدوية وسلة المشتريات متاحة حصرياً لحسابات الصيدليات المرخصة","type":"error"}}`)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، إضافة الأدوية وطلب التوريد متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID
	if h.commSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"خدمة السلة غير متوفرة حالياً","type":"error"}}`)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	if vendorOrgID <= 0 {
		vendorOrgID, _ = strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	}

	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		qty, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if qty <= 0 {
		qty = 1
	}

	back := strings.TrimSpace(r.PostFormValue("return_to"))
	if back == "" {
		back = r.Header.Get("Referer")
	}
	if back == "" {
		back = "/cart"
	}

	// Auto-resolve missing product/vendor info from variant
	if h.catSvc != nil && variantID > 0 && (productID <= 0 || vendorOrgID <= 0) {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			if productID <= 0 {
				productID = v.ProductID
			}
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
		}
	}

	// Stock, supplier approval, branch ownership and weekly coverage are all
	// decided by commerce.CheckAvailability. Nothing here defaults a missing
	// supplier or quietly reduces the quantity the pharmacy asked for.
	if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, back) {
		return
	}

	item := &commerce.CartItem{
		ProductID:        productID,
		ProductVariantID: variantID,
		OrganizationID:   vendorOrgID,
		Quantity:         qty,
	}

	// Keep the offer identity and custom offer price
	if offerID, err := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64); err == nil && offerID > 0 {
		item.OfferID = &offerID
	}
	if offerPriceStr := strings.TrimSpace(r.PostFormValue("offer_price")); offerPriceStr != "" {
		if amt, err := money.Parse(offerPriceStr); err == nil && amt.IsPositive() {
			item.UnitPrice = amt
		}
	} else if customPriceStr := strings.TrimSpace(r.PostFormValue("custom_price")); customPriceStr != "" {
		if amt, err := money.Parse(customPriceStr); err == nil && amt.IsPositive() {
			item.UnitPrice = amt
		}
	}

	// Authoritative catalog price lookup if unit price is not set
	if item.UnitPrice.IsZero() && h.catSvc != nil {
		if variantID > 0 {
			if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil && !v.Price.IsZero() {
				item.UnitPrice = v.Price
			}
		}
		if item.UnitPrice.IsZero() && productID > 0 {
			if prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID); err == nil && prod != nil {
				item.ProductName = prod.Name
				item.UnitPrice = prod.EffectivePrice()
			}
		}
	}

	if _, err := h.commSvc.AddToCart(ctx, userID, item); err != nil {
		h.log.ErrorContext(ctx, "add to cart", "error", err,
			"user", userID, "variant", variantID, "vendor", vendorOrgID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, h.safeMessage(err, langOf(r))))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		itemCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				itemCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":"تمت إضافة الصنف إلى سلة المشتريات بنجاح","type":"success"},"cartUpdated":{"count":%d}}`, itemCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	if returnTo := strings.TrimSpace(r.PostFormValue("return_to")); returnTo != "" {
		h.redirectWithNotice(w, r, returnTo, "success", "تمت إضافة الصنف إلى سلة المشتريات بنجاح.")
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// AddOfferToCartSubmit adds an entire special offer bundle to the cart for a pharmacy.
func (h *UIHandler) AddOfferToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/offers")
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"يرجى تسجيل الدخول كصيدلية مرخصة للشراء","type":"error"}}`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/offers", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"عذراً، شراء باقات العروض متاح حصرياً لحسابات الصيدليات المرخصة","type":"error"}}`)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/offers", "error", "عذراً، شراء باقات العروض متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID
	if h.commSvc == nil || h.promoSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"خدمة السلة غير متوفرة حالياً","type":"error"}}`)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64)
	if offerID <= 0 {
		h.redirectWithNotice(w, r, "/offers", "error", "معرف العرض غير صالح.")
		return
	}

	bundleMultiplier, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if bundleMultiplier <= 0 {
		bundleMultiplier, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if bundleMultiplier <= 0 {
		bundleMultiplier = 1
	}

	sp, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || sp == nil {
		o, oErr := h.promoSvc.GetOffer(ctx, offerID)
		if oErr != nil || o == nil {
			h.redirectWithNotice(w, r, "/offers", "error", "العرض المطلوب غير موجود أو انتهت صلاحيته.")
			return
		}
		sp = &promo.SpecialOffer{
			ID:                 o.ID,
			OrganizationID:     o.OrganizationID,
			Title:              o.Title,
			DiscountPercentage: float64(o.DiscountValue.Minor()) / 100.0,
		}
	}

	addedItemsCount := 0
	if len(sp.Products) > 0 {
		for _, p := range sp.Products {
			if p == nil || p.VariantID <= 0 {
				continue
			}
			qty := p.Quantity * bundleMultiplier
			if qty <= 0 {
				qty = bundleMultiplier
			}

			prodID := int64(0)
			unitPrice := p.CustomPrice
			if unitPrice.IsZero() && p.OriginalPrice.IsPositive() {
				if sp.DiscountPercentage > 0 {
					discMinor := int64(float64(p.OriginalPrice.Minor()) * (sp.DiscountPercentage / 100.0))
					unitPrice = money.FromMinor(p.OriginalPrice.Minor() - discMinor)
				} else {
					unitPrice = p.OriginalPrice
				}
			}

			if h.catSvc != nil {
				if v, vErr := h.catSvc.GetVariant(database.AsSystem(ctx), p.VariantID); vErr == nil && v != nil {
					prodID = v.ProductID
					if unitPrice.IsZero() {
						unitPrice = v.Price
					}
				}
			}

			item := &commerce.CartItem{
				ProductID:        prodID,
				ProductVariantID: p.VariantID,
				OrganizationID:   sp.OrganizationID,
				Quantity:         qty,
				UnitPrice:        unitPrice,
				OfferID:          &offerID,
			}

			if _, aErr := h.commSvc.AddToCart(ctx, userID, item); aErr == nil {
				addedItemsCount++
			} else {
				h.log.WarnContext(ctx, "add offer item to cart error", "error", aErr, "variant_id", p.VariantID)
			}
		}
	}


	// Record offer conversion / click
	_ = h.promoSvc.RecordOfferClick(ctx, offerID)

	if addedItemsCount == 0 && len(sp.Products) > 0 {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"تعذر إضافة أصناف العرض إلى السلة، يرجى المحاولة لاحقاً","type":"error"}}`)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", "تعذر إضافة أصناف العرض إلى السلة.")
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		totalCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				totalCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":"تمت إضافة باقة العرض بالكامل إلى سلة المشتريات بنجاح","type":"success"},"cartUpdated":{"count":%d}}`, totalCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	h.redirectWithNotice(w, r, "/cart", "success", "تمت إضافة باقة العرض بالكامل إلى سلة المشتريات بنجاح.")
}


// assertCartLineAvailable runs the availability rule and, when it refuses,
// redirects with the reason. It returns false when the caller must stop.
//
// Every buying surface goes through here so the rules cannot drift: the cart,
// the quantity controls and checkout all ask the same question.
func (h *UIHandler) assertCartLineAvailable(
	w http.ResponseWriter, r *http.Request, actor authctx.Actor,
	variantID, vendorOrgID int64, qty int, back string,
) bool {
	ctx := r.Context()

	branchID := h.pharmacyBranchID(ctx, &actor)

	res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
		VariantID:        variantID,
		VendorOrgID:      vendorOrgID,
		CustomerOrgID:    actor.OrganizationID,
		CustomerBranchID: branchID,
		Quantity:         qty,
		When:             time.Now(),
	})
	if err != nil {
		// A failed check is not permission to buy.
		h.log.ErrorContext(ctx, "availability check failed", "error", err,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"تعذر التحقق من توفر الصنف حالياً، يرجى المحاولة مرة أخرى","type":"error"}}`)
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error",
			"تعذر التحقق من توفر الصنف حالياً، يرجى المحاولة مرة أخرى.")
		return false
	}
	if !res.Allowed {
		h.log.InfoContext(ctx, "cart line refused", "reason", res.Reason,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID, "qty", qty)
		if h.isHTMX(r) {
			if back == "/cart" {
				cart, _ := h.commSvc.GetCart(ctx, actor.UserID)
				lang, _ := h.localeAndDir(r)
				w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, res.MessageAr))
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = pages.CustomerCartContent(cart, lang).Render(ctx, w)
				return false
			}
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, res.MessageAr))
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error", res.MessageAr)
		return false
	}
	return true
}

func (h *UIHandler) RemoveFromCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	_, _ = h.commSvc.RemoveFromCart(ctx, userID, variantID)

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCartContent(cart, lang).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer cart content", "error", err)
		}
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) UpdateCartQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty < 0 {
		qty = 0
	}

	// Raising a quantity is a purchase decision and gets the same check as
	// adding the line. The client's "+" button is a hint; this is the rule.
	// Quantity 0 means "remove", which needs no availability check.
	if qty > 0 {
		actor, ok := authctx.From(ctx)
		if !ok {
			http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
			return
		}
		vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
		if vendorOrgID <= 0 {
			// The cart row knows its supplier even when the form omits it.
			if line, err := h.commSvc.GetCartLine(ctx, userID, variantID); err == nil && line != nil {
				vendorOrgID = line.OrganizationID
			}
		}
		if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, "/cart") {
			return
		}
	}

	if _, err := h.commSvc.SetCartQuantity(ctx, userID, variantID, qty); err != nil {
		h.log.ErrorContext(ctx, "set cart quantity", "error", err,
			"user", userID, "variant", variantID, "qty", qty)
		h.redirectWithNotice(w, r, "/cart", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCartContent(cart, lang).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer cart content", "error", err)
		}
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/orders", http.StatusSeeOther)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil || cart == nil || len(cart.Items) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	var items []commerce.CheckoutLineItem
	var offerID int64
	for _, it := range cart.Items {
		pID := it.ProductID
		vID := it.ProductVariantID
		vOrgID := it.OrganizationID
		if vOrgID <= 0 && h.catSvc != nil && pID > 0 {
			if prod, variants, err := h.catSvc.GetProduct(ctx, pID); err == nil && prod != nil {
				for _, v := range variants {
					if v != nil && v.ID == vID && v.OrganizationID > 0 {
						vOrgID = v.OrganizationID
						break
					}
				}
				if vOrgID <= 0 && prod.OrganizationID > 0 {
					vOrgID = prod.OrganizationID
				}
			}
		}
		uPrice := it.UnitPrice
		if uPrice.IsZero() {
			uPrice, _ = money.Parse("38.50")
		}
		pName := it.ProductName
		if len(pName) == 0 {
			pName = i18n.Text{"ar": "صنف دوائي معتمد", "en": "Certified Medicine"}
		}
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:      vOrgID,
			ProductID:        &pID,
			ProductVariantID: &vID,
			ProductName:      pName,
			Quantity:         it.Quantity,
			UnitPrice:        uPrice,
		})
		// One offer per order (main_orders parity). If the cart mixes offers,
		// the order degrades to a legacy non-offer order — the cart-per-offer
		// UI is Phase 5.
		if it.OfferID != nil {
			if offerID == 0 {
				offerID = *it.OfferID
			} else if offerID != *it.OfferID {
				offerID = 0
			}
		}
	}

	paymentMethod := "cod"

	var branchID *int64
	if bID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64); err == nil && bID > 0 {
		branchID = &bID
	} else if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
		branchID = buying.Active
	} else if actor, ok := authctx.From(ctx); ok && actor.BranchID != nil && *actor.BranchID > 0 {
		branchID = actor.BranchID
	}

	var targetBranchID int64
	if branchID != nil {
		targetBranchID = *branchID
	}

	if actor, ok := authctx.From(ctx); ok && targetBranchID > 0 {
		for _, it := range cart.Items {
			vOrgID := it.OrganizationID
			if vOrgID <= 0 {
				continue
			}
			res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
				VariantID:        it.ProductVariantID,
				VendorOrgID:      vOrgID,
				CustomerOrgID:    actor.OrganizationID,
				CustomerBranchID: targetBranchID,
				Quantity:         it.Quantity,
				When:             time.Now(),
			})
			if err == nil && !res.Allowed {
				h.redirectWithNotice(w, r, "/checkout", "error", "فرع الصيدلية المحدد خارج نطاق التغطية الجغرافية لشركات التوريد ("+res.MessageAr+"). يرجى اختيار فرع معتمد داخل التغطية.")
				return
			}
		}
	}

	input := commerce.CheckoutInput{
		CustomerID:    userID,
		BranchID:      branchID,
		PaymentMethod: paymentMethod,
		Notes:         r.PostFormValue("notes"),
		Items:         items,
	}
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		input.CustomerOrgID = actor.OrganizationID
	}
	if offerID > 0 {
		input.OfferID = offerID
		// The offer is the authority for the minimum order amount and the
		// fulfilling vendor branch; the buying branch comes from the shell
		// selector, validated against the actor's own branches.
		if h.promoSvc != nil {
			if offer, err := h.promoSvc.GetOffer(ctx, offerID); err == nil && offer != nil {
				input.MinOrderAmount = offer.MinOrderAmount
				if offer.BranchID != nil && *offer.BranchID > 0 {
					input.VendorBranchID = offer.BranchID
				}
			}
		}
	}

	if input.BranchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil {
			input.BranchID = buying.Active
		}
	}

	// Ensure vendor branch is resolved if vendor has branches
	if input.VendorBranchID == nil && len(items) > 0 && items[0].VendorOrgID > 0 && h.orgSvc != nil {
		if vBranches, err := h.orgSvc.ListBranches(ctx, items[0].VendorOrgID); err == nil && len(vBranches) > 0 {
			for _, vb := range vBranches {
				if vb.IsMain {
					input.VendorBranchID = &vb.ID
					break
				}
			}
			if input.VendorBranchID == nil {
				input.VendorBranchID = &vBranches[0].ID
			}
		}
	}

	// Calculate dynamic delivery fees based on vendor distance delivery bands
	vendorShippingFees := make(map[int64]money.Amount)
	for _, it := range items {
		if it.VendorOrgID > 0 {
			if _, exists := vendorShippingFees[it.VendorOrgID]; !exists {
				fee := h.ResolveVendorShippingFee(ctx, it.VendorOrgID, input.VendorBranchID, input.BranchID)
				vendorShippingFees[it.VendorOrgID] = fee
			}
		}
	}
	input.VendorShippingFees = vendorShippingFees

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "checkout failed", "error", err)
		h.renderError(w, r, err)
		return
	}

	// Dispatch real-time in-app notifications to pharmacy and fulfilling vendors
	pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
	go h.notifyOrderPlaced(context.Background(), order, pharmacyName)

	_ = h.commSvc.ClearCart(ctx, userID)
	http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
}

func (h *UIHandler) MarkNotificationReadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _ := authctx.UserID(ctx)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.notifSvc != nil && id > 0 {
		_ = h.notifSvc.MarkRead(ctx, id, userID)
	}
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// CustomerBranchCreatePage redirects to the unified branches page in add mode.
func (h *UIHandler) CustomerBranchCreatePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchEditPage redirects to the unified branches page with edit mode preloaded.
func (h *UIHandler) CustomerBranchEditPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id != "" {
		http.Redirect(w, r, "/customer/branches?edit="+id, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchesPage renders the pharmacy's own branches and employees management screen in CustomerShell.
func (h *UIHandler) CustomerBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	var branches []*org.Branch
	var employees []*org.EmployeeView
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, orgID)
		employees, _ = h.orgSvc.ListEmployees(ctx, orgID)
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab != "employees" {
		activeTab = "branches"
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.CustomerBranchesData{
		Branches:   branches,
		Employees:  employees,
		Cities:     h.listCities(ctx),
		ActiveTab:  activeTab,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerBranches(data, lang, dir, actor.Permissions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer branches page", "error", err)
	}
}

// CustomerBranchNewSubmit creates a new pharmacy branch for the customer organization.
func (h *UIHandler) CustomerBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = "فرع صيدلية"
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	b := &org.Branch{
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(nameAr, nameEn),
		Code:           code,
		WarehouseType:  "pharmacy",
		Address:        address,
		Phone:          phone,
		OperatingHours: operatingHours,
		HasColdStorage: hasColdStorage,
		GoogleMapsURL:  gmaps,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         "active",
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "customer create branch error", "error", err)
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم إضافة فرع الصيدلية بنجاح.")
}

// CustomerBranchEditSubmit updates an existing pharmacy branch.
func (h *UIHandler) CustomerBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "معرف الفرع غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	existing, err := h.orgSvc.GetBranch(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "الفرع غير موجود أو لا تملك صلاحية تعديله.")
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = existing.Name.Get(i18n.AR)
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "active"
	}
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	} else {
		cityID = existing.CityID
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}
	if latPtr == nil {
		latPtr = existing.Latitude
	}
	if lngPtr == nil {
		lngPtr = existing.Longitude
	}

	b := &org.Branch{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(nameAr, nameEn),
		Code:           code,
		WarehouseType:  "pharmacy",
		Address:        address,
		Phone:          phone,
		OperatingHours: operatingHours,
		HasColdStorage: hasColdStorage,
		GoogleMapsURL:  gmaps,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         status,
	}

	if err := h.orgSvc.UpdateBranch(ctx, b); err != nil {
		h.log.ErrorContext(ctx, "customer edit branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم تحديث بيانات الفرع بنجاح.")
}

// CustomerBranchDeleteSubmit deletes a branch owned by the customer organization.
func (h *UIHandler) CustomerBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "معرف الفرع غير صالح.")
		return
	}

	if h.orgSvc != nil {
		existing, err := h.orgSvc.GetBranch(ctx, id)
		if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
			h.redirectWithNotice(w, r, "/customer/branches", "error", "الفرع غير موجود أو لا تملك صلاحية حذفه.")
			return
		}
		if existing.IsMain {
			h.redirectWithNotice(w, r, "/customer/branches", "error", "لا يمكن حذف الفرع الرئيسي، يرجى تعيين فرع رئيسي آخر أولاً.")
			return
		}
		if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم حذف الفرع بنجاح.")
}

// CustomerEmployeeCreateSubmit creates a new employee user and binds them to the branch and role.
func (h *UIHandler) CustomerEmployeeCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	email := strings.TrimSpace(r.PostFormValue("email"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	if roleKey == "" || roleKey == "org_admin" {
		if roleKey == "org_admin" {
			roleKey = "org_owner"
		} else {
			roleKey = "org_pharmacist"
		}
	}
	password := strings.TrimSpace(r.PostFormValue("password"))

	if email == "" || name == "" {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "الاسم والبريد الإلكتروني حقول إلزامية.")
		return
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)

	// 1. Locate or create user account
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(sysCtx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		if password == "" {
			randBytes := make([]byte, 8)
			_, _ = rand.Read(randBytes)
			password = fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))
		}

		newUser, _, err := h.idSvc.Register(sysCtx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Add to organization members
	member := &org.Member{
		OrganizationID: orgID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// 3. If role is org_manager and branch is specified, assign as branch manager
	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, orgID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم إضافة وتعيين الموظف بالفرع وتحديد صلاحياته بنجاح.")
}

// CustomerEmployeeEditSubmit updates an employee's branch assignment, role, job title, and details.
func (h *UIHandler) CustomerEmployeeEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetUserID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف المستخدم غير صالح.")
		return
	}

	_ = r.ParseForm()

	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	if roleKey == "" || roleKey == "org_admin" {
		if roleKey == "org_admin" {
			roleKey = "org_owner"
		} else {
			roleKey = "org_pharmacist"
		}
	}
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)
	member := &org.Member{
		OrganizationID: orgID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		IsActive:       isActive,
	}

	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, orgID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم حفظ وتحديث بيانات وصلاحيات الموظف بنجاح.")
}

// CustomerEmployeeDeleteSubmit removes an employee member from the organization and branch.
func (h *UIHandler) CustomerEmployeeDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetUserID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف الموظف غير صالح.")
		return
	}

	if targetUserID == actor.UserID {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "لا يمكنك إزالة حسابك الحالي من المؤسسة.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.RemoveMember(sysCtx, orgID, targetUserID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم حذف الموظف وإلغاء ربطه بالفرع بنجاح.")
}

// CustomerEmployeeStatusSubmit toggles active status for an employee.
func (h *UIHandler) CustomerEmployeeStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memberID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف الموظف غير صالح.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.ToggleMemberStatus(sysCtx, orgID, memberID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم تحديث حالة تفعيل الموظف بنجاح.")
}

// ReviewSubmit handles customer feedback submissions with multi-criteria rating.
func (h *UIHandler) ReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	targetOrgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", "المؤسسة المستهدفة غير صالحة.")
		return
	}

	var orderID *int64
	if oIDStr := r.PostFormValue("order_id"); oIDStr != "" {
		if oID, err := strconv.ParseInt(oIDStr, 10, 64); err == nil && oID > 0 {
			orderID = &oID
		}
	}

	repScore, _ := strconv.Atoi(r.PostFormValue("rating_rep"))
	speedScore, _ := strconv.Atoi(r.PostFormValue("rating_speed"))
	qualityScore, _ := strconv.Atoi(r.PostFormValue("rating_quality"))

	if repScore < 1 {
		repScore = 5
	}
	if speedScore < 1 {
		speedScore = 5
	}
	if qualityScore < 1 {
		qualityScore = 5
	}

	rev := &org.Review{
		OrganizationID: targetOrgID,
		UserID:         actor.UserID,
		OrderID:        orderID,
		Title:          r.PostFormValue("title"),
		ReviewText:     r.PostFormValue("review_text"),
		Context:        r.PostFormValue("context"),
		IsApproved:     true,
		Ratings: []org.ReviewRating{
			{Criterion: "rep", Score: repScore},
			{Criterion: "speed", Score: speedScore},
			{Criterion: "quality", Score: qualityScore},
		},
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.SubmitReview(ctx, rev); err != nil {
			h.log.ErrorContext(ctx, "failed to submit review", "error", err, "target_org_id", targetOrgID)
			h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "success", "تم إرسال تقييمك بنجاح. شكراً لمشاركتك!")
}

// CustomerSwitchActiveBranchSubmit handles switching the active branch for customer context.
func (h *UIHandler) CustomerSwitchActiveBranchSubmit(w http.ResponseWriter, r *http.Request) {
	h.SetBuyingBranchSubmit(w, r)
}

// CustomerNegotiateOrderSubmit initiates a price negotiation order with a supplier.
func (h *UIHandler) CustomerNegotiateOrderSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/catalog", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/catalog", "error", "بيانات النموذج غير صالحة.")
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("qty"))
	if qty <= 0 {
		qty = 1
	}

	proposedPriceStr := r.PostFormValue("proposed_price")
	proposedPrice, err := money.Parse(proposedPriceStr)
	if err != nil || proposedPrice.IsZero() || proposedPrice.IsNegative() {
		h.redirectWithNotice(w, r, "/catalog", "error", "يرجى إدخال سعر تفاوضي صالح وموجب.")
		return
	}

	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = fmt.Sprintf("طلب تفاوض على سعر الصنف: %s ج.م للعبوة (كمية: %d)", proposedPrice.String(), qty)
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}
	if branchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
			branchID = buying.Active
		} else if actor.BranchID != nil && *actor.BranchID > 0 {
			branchID = actor.BranchID
		}
	}

	var productID int64
	prodName := i18n.Text{"ar": "صنف دوائي للتفاوض", "en": "Negotiated Medicine"}
	if h.catSvc != nil && variantID > 0 {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			productID = v.ProductID
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
			if len(v.Name) > 0 {
				prodName = v.Name
			}
		}
	}

	paymentMethod := r.PostFormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "cod"
	}

	var pIDPtr *int64
	if productID > 0 {
		pIDPtr = &productID
	}
	var vIDPtr *int64
	if variantID > 0 {
		vIDPtr = &variantID
	}

	input := commerce.CheckoutInput{
		CustomerID:       actor.UserID,
		CustomerOrgID:    actor.OrganizationID,
		BranchID:         branchID,
		PaymentMethod:    paymentMethod,
		Notes:            notes,
		IsNegotiation:    true,
		NegotiationNotes: notes,
		Items: []commerce.CheckoutLineItem{
			{
				ProductVariantID:  vIDPtr,
				ProductID:         pIDPtr,
				VendorOrgID:       vendorOrgID,
				ProductName:       prodName,
				Quantity:          qty,
				UnitPrice:         proposedPrice,
				ProposedUnitPrice: proposedPrice,
				IsNegotiated:      true,
			},
		},
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/catalog", "error", "خدمة المبيعات غير متاحة حالياً.")
		return
	}

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to submit negotiation order", "error", err, "variant_id", variantID)
		h.redirectWithNotice(w, r, "/catalog", "error", "تعذر إرسال طلب التفاوض: "+h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", order.ID), "success", "تم إرسال طلب التفاوض على السعر إلى المورد بنجاح وهو الآن قيد المراجعة.")
}
