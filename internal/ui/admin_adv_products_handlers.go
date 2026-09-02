package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAdvProductsPage renders the product sponsorships and promoted items dashboard.
func (h *UIHandler) AdminAdvProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	tab := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tab")))
	if tab == "" {
		tab = "all"
	}
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	packageFilterID, _ := strconv.ParseInt(r.URL.Query().Get("package_id"), 10, 64)

	var items []pages.AdminAdvProductItem
	var packages []*promo.OfferPackage
	var vendors []*org.Organization
	var catalogProducts []*catalog.Product

	var totalCount, activeCount, pendingCount, expiredCount, rejectedCount int
	now := time.Now().UTC()

	// Load packages
	if h.promoSvc != nil {
		if pkgs, err := h.promoSvc.AdminListPackages(sysCtx); err == nil {
			packages = pkgs
		}
	}
	pkgMap := make(map[int64]*promo.OfferPackage)
	for _, p := range packages {
		if p != nil {
			pkgMap[p.ID] = p
		}
	}

	// Load vendors for manual sponsorship modal
	if h.orgSvc != nil {
		vendorType := org.TypeVendor
		if orgs, err := h.orgSvc.ListOrganizations(sysCtx, &vendorType, nil, 100, 0); err == nil {
			vendors = orgs
		}
	}

	// Load catalog products for manual modal
	if h.catSvc != nil {
		if prods, err := h.catSvc.ListProducts(sysCtx, string(catalog.StatusActive), 100, 0); err == nil {
			catalogProducts = prods
		}
	}

	// Cache maps for fast lookup
	prodMap := make(map[int64]*catalog.Product)
	orgMap := make(map[int64]*org.Organization)

	// Load all sponsorship requests
	if h.promoSvc != nil {
		if reqs, err := h.promoSvc.AdminListSponsorshipRequests(sysCtx, 500, 0); err == nil {
			for _, req := range reqs {
				if req == nil {
					continue
				}

				// Only consider product sponsorships (or default to product if not specified)
				if req.ItemType != "" && req.ItemType != promo.SponsorItemProduct {
					continue
				}

				isExpired := req.Status == promo.SRSExpired || (!req.ExpiresAt.IsZero() && req.ExpiresAt.Before(now))
				isActive := (req.AdminStatus == promo.AdminApproved || req.Status == promo.SRSActive) && !isExpired
				isPending := req.AdminStatus == promo.AdminPending && !isExpired
				isRejected := req.AdminStatus == promo.AdminRejected || req.Status == promo.SRSRejected

				totalCount++
				if isActive {
					activeCount++
				} else if isPending {
					pendingCount++
				} else if isRejected {
					rejectedCount++
				} else if isExpired {
					expiredCount++
				}

				// Resolve Product
				var product *catalog.Product
				if req.ItemID > 0 && h.catSvc != nil {
					if cached, found := prodMap[req.ItemID]; found {
						product = cached
					} else {
						if p, _, err := h.catSvc.GetProduct(sysCtx, req.ItemID); err == nil && p != nil {
							product = p
							prodMap[req.ItemID] = p
						}
					}
				}

				// Resolve Organization
				var organization *org.Organization
				if req.OrganizationID > 0 && h.orgSvc != nil {
					if cached, found := orgMap[req.OrganizationID]; found {
						organization = cached
					} else {
						if o, err := h.orgSvc.GetOrganization(sysCtx, req.OrganizationID); err == nil && o != nil {
							organization = o
							orgMap[req.OrganizationID] = o
						}
					}
				}

				// Resolve Package
				pkg := req.Package
				if pkg == nil && req.PackageID > 0 {
					pkg = pkgMap[req.PackageID]
				}

				// Apply Tab Filtering
				include := false
				switch tab {
				case "active":
					include = isActive
				case "pending":
					include = isPending
				case "expired":
					include = isExpired && !isPending && !isRejected
				case "rejected":
					include = isRejected
				default: // "all"
					include = true
				}

				// Apply Package Filter
				if include && packageFilterID > 0 && req.PackageID != packageFilterID {
					include = false
				}

				// Apply Search Filter
				if include && searchQuery != "" {
					q := strings.ToLower(searchQuery)
					matched := false
					if product != nil {
						if strings.Contains(strings.ToLower(product.Name.Get(i18n.AR)), q) ||
							strings.Contains(strings.ToLower(product.Name.Get(i18n.EN)), q) ||
							strings.Contains(strings.ToLower(product.PublicID), q) ||
							strings.Contains(strings.ToLower(product.Barcode), q) {
							matched = true
						}
					}
					if !matched && organization != nil {
						if strings.Contains(strings.ToLower(organization.LegalName), q) ||
							strings.Contains(strings.ToLower(organization.TradeName.Get("ar")), q) ||
							strings.Contains(strings.ToLower(organization.TradeName.Get("en")), q) ||
							strings.Contains(strings.ToLower(organization.OrganizationNumber), q) {
							matched = true
						}
					}
					if !matched && pkg != nil {
						if strings.Contains(strings.ToLower(pkg.Name.Get(i18n.AR)), q) ||
							strings.Contains(strings.ToLower(pkg.Name.Get(i18n.EN)), q) {
							matched = true
						}
					}
					if !matched {
						include = false
					}
				}

				if include {
					items = append(items, pages.AdminAdvProductItem{
						Request:      req,
						Product:      product,
						Organization: organization,
						Package:      pkg,
						IsActive:     isActive,
						IsPending:    isPending,
						IsExpired:    isExpired,
						IsRejected:   isRejected,
					})
				}
			}
		}
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)

	filteredTotal := len(items)
	start := (page - 1) * limit
	if start < 0 {
		start = 0
	}
	end := start + limit
	var paginatedItems []pages.AdminAdvProductItem
	if start < filteredTotal {
		if end > filteredTotal {
			end = filteredTotal
		}
		paginatedItems = items[start:end]
	}

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.AdminAdvProductsData{
		Items:             paginatedItems,
		TotalCount:        totalCount,
		ActiveCount:       activeCount,
		PendingCount:      pendingCount,
		ExpiredCount:      expiredCount,
		RejectedCount:     rejectedCount,
		Packages:          packages,
		Vendors:           vendors,
		CatalogProducts:   catalogProducts,
		ActiveTab:         tab,
		SearchQuery:       searchQuery,
		SelectedPackageID: packageFilterID,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
		Page:              page,
		PerPage:           limit,
		FilteredTotal:     filteredTotal,
	}

	h.renderPage(ctx, w, "render admin adv-products page", pages.AdminAdvProductsPage(lang, dir, data))
}

// AdminAdvProductApproveSubmit approves and activates a product sponsorship request.
func (h *UIHandler) AdminAdvProductApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", "معرف طلب الرعاية غير صالح.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	req, err := h.promoSvc.AdminApproveSponsorshipRequest(sysCtx, id, notes)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	if req != nil && req.OrganizationID > 0 {
		pkgName := "باقة الرعاية"
		if req.Package != nil {
			pkgName = req.Package.Name.Get("ar")
		}
		go h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, true, notes)
	}

	h.redirectWithNotice(w, r, "/admin/adv-products", "success", "تمت الموافقة على رعاية المنتج وتفعيله في صدارة نتائج البحث بنجاح.")
}

// AdminAdvProductRejectSubmit rejects a product sponsorship request and refunds credits.
func (h *UIHandler) AdminAdvProductRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", "معرف طلب الرعاية غير صالح.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	req, _ := h.promoSvc.GetSponsorshipRequestByID(sysCtx, id)
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = "تم رفض طلب الرعاية من قبل إدارة المنصة."
	}

	if err := h.promoSvc.AdminRejectSponsorshipRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	if req != nil && req.OrganizationID > 0 {
		pkgName := "باقة الرعاية"
		if req.Package != nil {
			pkgName = req.Package.Name.Get("ar")
		}
		go h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, false, notes)
	}

	h.redirectWithNotice(w, r, "/admin/adv-products", "success", "تم رفض طلب رعاية المنتج وإعادة الرصيد للمورد بنجاح.")
}

// AdminAdvProductCreateSubmit creates an instant product sponsorship directly from the admin panel.
func (h *UIHandler) AdminAdvProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	packageID, _ := strconv.ParseInt(r.PostFormValue("package_id"), 10, 64)
	days, _ := strconv.Atoi(r.PostFormValue("duration_days"))

	if orgID <= 0 || productID <= 0 || packageID <= 0 {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", "يرجى تحديد المنشأة، المنتج، وباقة الرعاية بدقة.")
		return
	}

	if days <= 0 {
		days = 30
	}

	sysCtx := database.AsSystem(ctx)
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)

	sr := &promo.SponsorshipRequest{
		OrganizationID: orgID,
		PackageID:      packageID,
		ItemType:       promo.SponsorItemProduct,
		ItemID:         productID,
		CreditsUsed:    1,
		AdminStatus:    promo.AdminApproved,
		Status:         promo.SRSActive,
		StartsAt:       now,
		ExpiresAt:      expiresAt,
	}

	if err := h.promoSvc.AdminCreateDirectSponsorship(sysCtx, sr); err != nil {
		h.redirectWithNotice(w, r, "/admin/adv-products", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/adv-products", "success", "تم تفعيل رعاية وتثبيت المنتج في الصدارة بنجاح.")
}
