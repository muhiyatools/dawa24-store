package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// /catalog is authenticated-only: guests are sent to login instead of
	// receiving a capped public listing.
	if actor, ok := authctx.From(ctx); !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	// 1. Security & Anti-Scraping / Bot Defense
	// Honeypot trap check: the filter form carries a hidden field no person can
	// see or tab into, so a value in it means the caller submitted the form by
	// reading the HTML. The response is a normal-looking empty catalogue rather
	// than an error: a scraper that is told it was caught adapts, one that is
	// handed plausible nothing does not.
	//
	// The caller is also put on the guard's refused list, which is what makes
	// this cost more than one wasted request.
	if botTrap := r.URL.Query().Get("company_tax_ref"); botTrap != "" {
		h.scrape.Penalize(r, "honeypot_field")
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
	//
	// The two ceilings are the cap on how much of the catalogue one caller can
	// reach at all, which is the part of the anti-scraping defence that a
	// forged User-Agent cannot walk around: the request budgets decide how fast
	// the catalogue can be read, these decide how much of it is readable. A
	// caller who has not signed in gets the lower pair.
	maxPage, maxPageSize := h.guestListingBounds(r, 200, 96)

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
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	page := 1
	if pVal := r.URL.Query().Get("page"); pVal != "" {
		if p, err := strconv.Atoi(pVal); err == nil && p >= 1 {
			page = p
		}
	}
	// Bound page depth so no single filter combination can be walked to the end
	// of the catalogue.
	if page > maxPage {
		page = maxPage
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
		InStock:    inStock,
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
	variantCards := h.buildCatalogVariantCards(
		ctx,
		filtered,
		productIDs,
		variantsByProduct,
		brandMap,
		env,
		lang,
		inStock,
		hasDiscount,
		func() *int64 {
			if minPrice != nil {
				v := minPrice.Minor()
				return &v
			}
			return nil
		}(),
		func() *int64 {
			if maxPrice != nil {
				v := maxPrice.Minor()
				return &v
			}
			return nil
		}(),
		sortBy,
	)
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

	var catalogAds []*promo.Ad
	if h.promoSvc != nil {
		if ads, err := h.promoSvc.ListActiveAds(ctx, promo.PositionCatalogTop); err == nil {
			catalogAds = ads
			h.enrichAds(ctx, catalogAds)
		}
	}

	viewData := pages.CatalogPageData{
		Variants:            variantCards,
		Categories:          categories,
		Brands:              brands,
		Query:               query,
		CategoryID:          categoryID,
		BrandID:             brandID,
		MinPrice:            minPriceStr,
		MaxPrice:            maxPriceStr,
		DosageForm:          dosageForm,
		Sort:                sortBy,
		InStock:             inStock,
		HasDiscount:         hasDiscount,
		ViewMode:            viewMode,
		Page:                page,
		PageSize:            pageSize,
		TotalItems:          totalCount,
		TotalPages:          totalPages,
		HasPrev:             page > 1,
		HasNext:             page < totalPages,
		PrevPage:            page - 1,
		NextPage:            page + 1,
		StartItem:           startItem,
		EndItem:             endItem,
		ActiveCategory:      activeCatName,
		ActiveBrand:         activeBrandName,
		SponsoredProductIDs: sponsoredProductIDs,
		CatalogAds:          catalogAds,
	}

	h.renderPage(ctx, w, "render catalog page", pages.CustomerCatalog(viewData, lang, dir, h.isHTMX(r)))
}
