package ui

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// OffersPage renders the public offers listing.
func (h *UIHandler) OffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "offers.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	sortParam := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortParam == "" {
		sortParam = "newest"
	}

	actor, hasActor := authctx.From(ctx)
	isCustomer := hasActor && actor.IsCustomer()
	var customerBranch *org.Branch
	if isCustomer {
		customerBranch = h.pharmacyCustomerBranch(ctx, &actor)
	}

	var offerCards []*pages.OfferCardData
	if h.promoSvc != nil {
		offers, err := h.promoSvc.ListActiveOffers(ctx, 100, 0)
		if err != nil {
			h.log.WarnContext(ctx, "offers page: list active offers", "error", err)
		}

		// Sponsorship ranking: check which offers are sponsored.
		var offerIDs []int64
		for _, o := range offers {
			if o != nil {
				offerIDs = append(offerIDs, o.ID)
			}
		}
		sponsoredOfferIDs := make(map[int64]bool)
		if len(offerIDs) > 0 {
			if rankings, rErr := h.promoSvc.RankedSponsorshipsForOffers(ctx, offerIDs); rErr == nil {
				for _, rs := range rankings {
					if rs != nil {
						sponsoredOfferIDs[rs.ItemID] = true
					}
				}
			}
		}

		for _, o := range offers {
			if o == nil {
				continue
			}

			orgName := i18n.T(lang, "offers.default_supplier_name")
			if h.orgSvc != nil && o.OrganizationID > 0 {
				if oOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), o.OrganizationID); err == nil && oOrg != nil {
					if oOrg.LegalName != "" {
						orgName = oOrg.LegalName
					} else if !oOrg.TradeName.IsEmpty() {
						orgName = oOrg.TradeName.Get(i18n.Lang(lang))
					}
				}
			}

			discPct := 0.0
			if o.DiscountType == promo.DiscountPercentage {
				discPct = float64(o.DiscountValue.Minor()) / 100.0
			}

			prodCount := len(o.ProductIDs)
			var totalPrice money.Amount
			var sp *promo.SpecialOffer
			if spRes, err := h.promoSvc.GetSpecialOffer(ctx, o.ID); err == nil && spRes != nil {
				sp = spRes
				if len(sp.Products) > 0 {
					prodCount = len(sp.Products)
				}
				totalPrice = sp.TotalPrice
				if sp.OrganizationName != "" {
					orgName = sp.OrganizationName
				}
				if sp.DiscountPercentage > 0 {
					discPct = sp.DiscountPercentage
				}
			}

			isCovered := true
			covReason := ""
			if isCustomer {
				if sp != nil {
					isCovered, covReason = h.checkOfferCoverage(ctx, sp, customerBranch)
				} else {
					spStub := &promo.SpecialOffer{
						ID:             o.ID,
						OrganizationID: o.OrganizationID,
						BranchID:       o.BranchID,
					}
					isCovered, covReason = h.checkOfferCoverage(ctx, spStub, customerBranch)
				}
			}

			offerCards = append(offerCards, &pages.OfferCardData{
				ID:                 o.ID,
				Title:              o.Title,
				Description:        o.Description,
				OrganizationID:     o.OrganizationID,
				OrganizationName:   orgName,
				DiscountType:       string(o.DiscountType),
				DiscountValue:      o.DiscountValue,
				DiscountPercentage: discPct,
				MinOrderAmount:     o.MinOrderAmount,
				TotalPrice:         totalPrice,
				StartsAt:           o.StartsAt,
				ExpiresAt:          o.ExpiresAt,
				ProductsCount:      prodCount,
				IsSponsored:        sponsoredOfferIDs[o.ID],
				IsCustomerUser:     isCustomer,
				IsCovered:          isCovered,
				CoverageReason:     covReason,
			})
		}

		// Sort according to user preference
		switch sortParam {
		case "discount_desc":
			sort.SliceStable(offerCards, func(i, j int) bool {
				if offerCards[i].IsSponsored != offerCards[j].IsSponsored {
					return offerCards[i].IsSponsored
				}
				if offerCards[i].DiscountPercentage != offerCards[j].DiscountPercentage {
					return offerCards[i].DiscountPercentage > offerCards[j].DiscountPercentage
				}
				return offerCards[i].DiscountValue.Minor() > offerCards[j].DiscountValue.Minor()
			})
		case "discount_asc":
			sort.SliceStable(offerCards, func(i, j int) bool {
				if offerCards[i].IsSponsored != offerCards[j].IsSponsored {
					return offerCards[i].IsSponsored
				}
				if offerCards[i].DiscountPercentage != offerCards[j].DiscountPercentage {
					return offerCards[i].DiscountPercentage < offerCards[j].DiscountPercentage
				}
				return offerCards[i].DiscountValue.Minor() < offerCards[j].DiscountValue.Minor()
			})
		case "price_asc":
			sort.SliceStable(offerCards, func(i, j int) bool {
				if offerCards[i].IsSponsored != offerCards[j].IsSponsored {
					return offerCards[i].IsSponsored
				}
				priceI := offerCards[i].TotalPrice.Minor()
				if priceI == 0 {
					priceI = offerCards[i].MinOrderAmount.Minor()
				}
				priceJ := offerCards[j].TotalPrice.Minor()
				if priceJ == 0 {
					priceJ = offerCards[j].MinOrderAmount.Minor()
				}
				if priceI == 0 && priceJ > 0 {
					return false
				}
				if priceJ == 0 && priceI > 0 {
					return true
				}
				return priceI < priceJ
			})
		case "price_desc":
			sort.SliceStable(offerCards, func(i, j int) bool {
				if offerCards[i].IsSponsored != offerCards[j].IsSponsored {
					return offerCards[i].IsSponsored
				}
				priceI := offerCards[i].TotalPrice.Minor()
				if priceI == 0 {
					priceI = offerCards[i].MinOrderAmount.Minor()
				}
				priceJ := offerCards[j].TotalPrice.Minor()
				if priceJ == 0 {
					priceJ = offerCards[j].MinOrderAmount.Minor()
				}
				return priceI > priceJ
			})
		case "newest":
			fallthrough
		default:
			sort.SliceStable(offerCards, func(i, j int) bool {
				if offerCards[i].IsSponsored != offerCards[j].IsSponsored {
					return offerCards[i].IsSponsored
				}
				if !offerCards[i].StartsAt.Equal(offerCards[j].StartsAt) {
					return offerCards[i].StartsAt.After(offerCards[j].StartsAt)
				}
				return offerCards[i].ID > offerCards[j].ID
			})
		}
	}

	h.renderPage(ctx, w, "render offers page", pages.OffersPage(lang, dir, offerCards, sortParam))
}

// OfferDetailPage renders one offer with its full products and records an impression.
func (h *UIHandler) OfferDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.promoSvc == nil {
		h.renderError(w, r, err)
		return
	}

	sp, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || sp == nil {
		// Fallback to GetOffer if not structured as special offer
		o, oErr := h.promoSvc.GetOffer(ctx, id)
		if oErr != nil || o == nil {
			h.renderError(w, r, err)
			return
		}
		starts := o.StartsAt
		expires := o.ExpiresAt
		sp = &promo.SpecialOffer{
			ID:                 o.ID,
			PublicID:           o.PublicID,
			OrganizationID:     o.OrganizationID,
			Title:              o.Title,
			Description:        o.Description,
			DiscountPercentage: float64(o.DiscountValue.Minor()) / 100.0,
			MinOrderAmount:     o.MinOrderAmount,
			StartDate:          &starts,
			EndDate:            &expires,
			Status:             "active",
			AdminStatus:        o.AdminStatus,
		}
	}

	_ = h.promoSvc.RecordOfferView(ctx, id)

	var orgInfo *org.Organization
	if h.orgSvc != nil && sp.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), sp.OrganizationID); err == nil && o != nil {
			orgInfo = o
			if sp.OrganizationName == "" {
				if o.LegalName != "" {
					sp.OrganizationName = o.LegalName
				} else if !o.TradeName.IsEmpty() {
					sp.OrganizationName = o.TradeName.Get(i18n.Lang(lang))
				}
			}
		}
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)
	sp.Locations = locs

	actor, ok := authctx.From(ctx)
	isCustomer := ok && actor.IsCustomer()
	var customerBranch *org.Branch
	isCovered := true
	covReason := ""
	if isCustomer {
		customerBranch = h.pharmacyCustomerBranch(ctx, &actor)
		isCovered, covReason = h.checkOfferCoverage(ctx, sp, customerBranch)
	}

	data := pages.OfferDetailPageData{
		Offer:          sp,
		Organization:   orgInfo,
		Products:       sp.Products,
		Locations:      locs,
		IsCustomerUser: isCustomer,
		IsCovered:      isCovered,
		CoverageReason: covReason,
		CustomerBranch: customerBranch,
	}

	h.renderPage(ctx, w, "render offer detail", pages.OfferDetail(lang, dir, data))
}

// OfferClickSubmit records an offer click and sends the user to the catalogue.
func (h *UIHandler) OfferClickSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.promoSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.promoSvc.RecordOfferClick(ctx, id)
		}
	}
	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
}
