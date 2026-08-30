package ui

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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
	}

	h.renderPage(ctx, w, "render catalog page", pages.CustomerCatalog(viewData, lang, dir, h.isHTMX(r)))
}
