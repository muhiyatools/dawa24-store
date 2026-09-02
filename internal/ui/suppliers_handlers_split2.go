package ui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SupplierProfilePage renders a supplier's public profile.
func (h *UIHandler) SupplierProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.orgSvc == nil {
		h.renderError(w, r, err)
		return
	}

	sysCtx := database.AsSystem(ctx)
	o, err := h.orgSvc.GetOrganization(sysCtx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// Allow approved or active suppliers
	if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
		h.renderError(w, r, fmt.Errorf("%s", i18n.T(lang, "suppliers.vendor_unavailable")))
		return
	}

	branches, _ := h.orgSvc.ListBranches(sysCtx, id)
	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		coverages, _ = h.wfSvc.ListCoverageForOrganization(sysCtx, id)
	}
	workingHours, coverageDays, coverageAreas, isOpenNow, statusNote := computeVendorWorkingStatus(branches, coverages)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}
	limit := 24

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "catalog"
	}

	data := pages.SupplierProfileData{
		Org:           o,
		Branches:      branches,
		Coverages:     coverages,
		WorkingHours:  workingHours,
		CoverageDays:  coverageDays,
		CoverageAreas: coverageAreas,
		IsOpenNow:     isOpenNow,
		StatusNote:    statusNote,
		CurrentPage:   page,
		SearchQuery:   q,
		ActiveTab:     tab,
	}

	data.VariantMeta = make(map[int64]pages.SupplierVariantMeta)
	actor, hasActor := authctx.From(ctx)
	isCustomer := hasActor && actor.IsCustomer()
	customerBranchID := int64(0)
	if isCustomer {
		customerBranchID = h.pharmacyBranchID(ctx, &actor)
	}

	stockFilter := catalog.StockFilter(r.URL.Query().Get("stock"))
	if h.catSvc != nil {
		variants, total, err := h.catSvc.ListVendorVariants(database.AsSystem(ctx), id, catalog.VendorVariantQuery{
			Query:      q,
			Status:     "active",
			Stock:      stockFilter,
			PageNumber: page,
			PerPage:    limit,
		})
		if err == nil {
			data.Variants = variants
			data.TotalVariants = total
			if total > 0 {
				data.TotalPages = int(math.Ceil(float64(total) / float64(limit)))
			} else {
				data.TotalPages = 1
			}

			if len(variants) > 0 {
				pIDs := make([]int64, 0, len(variants))
				for _, v := range variants {
					if v != nil && v.ProductID > 0 {
						pIDs = append(pIDs, v.ProductID)
					}
				}
				if len(pIDs) > 0 {
					data.ProductsMap, _ = h.catSvc.ProductsByIDs(database.AsSystem(ctx), pIDs)
				}

				for _, v := range variants {
					if v == nil {
						continue
					}
					availStock := v.StockQty
					minQty := v.MinOrderQty
					if minQty <= 0 {
						minQty = 1
					}

					isCovered := true
					canAddToCart := (availStock > 0)
					covReason := ""

					if isCustomer {
						if h.commSvc != nil && customerBranchID > 0 {
							res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
								VariantID:        v.ID,
								VendorOrgID:      id,
								CustomerOrgID:    actor.OrganizationID,
								CustomerBranchID: customerBranchID,
								Quantity:         minQty,
								When:             time.Now(),
							})
							if err == nil {
								if res.Allowed {
									isCovered = true
									canAddToCart = (availStock > 0)
								} else {
									covReason = res.MessageAr
									if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation {
										isCovered = false
										canAddToCart = false
									} else if res.Reason == commerce.ReasonOutOfStock || res.Reason == commerce.ReasonInsufficientStock {
										isCovered = true
										canAddToCart = false
									} else if res.Reason == commerce.ReasonBelowMinimum {
										isCovered = true
										canAddToCart = (availStock > 0)
									} else {
										isCovered = false
										canAddToCart = false
									}
								}
							}
						}
					}

					data.VariantMeta[v.ID] = pages.SupplierVariantMeta{
						AvailableStock: availStock,
						MinOrderQty:    minQty,
						IsCovered:      isCovered,
						CoverageReason: covReason,
						CanAddToCart:   canAddToCart,
					}
				}
			}
		}
	}

	if h.promoSvc != nil {
		data.Sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, id)
	}
	if h.orgSvc != nil {
		data.Reviews, _ = h.orgSvc.ListReviews(ctx, id, 20, 0)
		data.Policies, _ = h.orgSvc.ListPolicies(ctx, id)
		if actor, ok := authctx.From(ctx); ok {
			data.IsFollowing, _ = h.orgSvc.IsFollowing(ctx, id, actor.UserID)
		}
	}
	data.ReviewCount = len(data.Reviews)
	if data.ReviewCount > 0 {
		var sum int
		for _, rv := range data.Reviews {
			sum += rv.Rating
		}
		data.Rating = float64(sum) / float64(data.ReviewCount)
	} else {
		data.Rating = 0
	}

	h.renderPage(ctx, w, "render supplier profile", pages.SupplierProfile(lang, dir, data))
}

// SupplierFollowSubmit toggles following for the signed-in user.
func (h *UIHandler) SupplierFollowSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_, _ = h.orgSvc.ToggleFollow(ctx, id, userID)
	}

	back := r.Referer()
	if back == "" {
		back = "/suppliers"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
