package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorDeliveryBandCreateSubmit creates a new distance delivery fee tier for the vendor.
func (h *UIHandler) VendorDeliveryBandCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.read_error"))
		return
	}

	fromMeters := 0
	if val := r.PostFormValue("from_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		fromMeters = km * 1000
	}

	toMeters := 0
	if val := r.PostFormValue("to_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		toMeters = km * 1000
	}

	feeAmount, _ := strconv.ParseFloat(r.PostFormValue("delivery_fee"), 64)

	if toMeters <= fromMeters || fromMeters < 0 || feeAmount < 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.invalid_params"))
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err != nil {
			bands = []*org.DeliveryBand{}
		}
		feeAmt, _ := money.Parse(fmt.Sprintf("%.2f", feeAmount))
		newBand := &org.DeliveryBand{
			OrganizationID: actor.OrganizationID,
			FromMeters:     fromMeters,
			ToMeters:       toMeters,
			Fee:            feeAmt,
			IsActive:       true,
		}
		bands = append(bands, newBand)
		if err := h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, bands); err != nil {
			h.log.ErrorContext(ctx, "save delivery bands", "error", err, "org", actor.OrganizationID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", fmt.Sprintf(i18n.T(lang, "delivery.band.save_failed_format"), err.Error()))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", i18n.T(lang, "delivery.band.added_success"))
}

// VendorDeliveryBandDeleteSubmit removes a distance delivery fee tier.
func (h *UIHandler) VendorDeliveryBandDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	bandID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || bandID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.invalid_id"))
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err == nil {
			var updated []*org.DeliveryBand
			for _, b := range bands {
				if b.ID != bandID {
					updated = append(updated, b)
				}
			}
			_ = h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, updated)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", i18n.T(lang, "delivery.band.deleted_success"))
}

// ResolveVendorShippingFee calculates the dynamic distance-based delivery fee between a vendor
// (or vendor warehouse/coverage city) and the customer pharmacy branch.
func (h *UIHandler) ResolveVendorShippingFee(ctx context.Context, vendorOrgID int64, vendorBranchID *int64, customerBranchID *int64) money.Amount {
	if h.orgSvc == nil || vendorOrgID <= 0 {
		return money.Zero
	}

	// 1. Resolve Customer Branch Coordinates
	var custLat, custLon *float64
	if customerBranchID != nil && *customerBranchID > 0 {
		if cb, err := h.orgSvc.GetBranch(ctx, *customerBranchID); err == nil && cb != nil {
			if cb.Latitude != nil && cb.Longitude != nil && *cb.Latitude != 0 && *cb.Longitude != 0 {
				custLat = cb.Latitude
				custLon = cb.Longitude
			}
		}
	}

	// 2. Resolve Vendor Coordinates (Branch or Coverage Center)
	var vendLat, vendLon *float64
	if vendorBranchID != nil && *vendorBranchID > 0 {
		if vb, err := h.orgSvc.GetBranch(ctx, *vendorBranchID); err == nil && vb != nil {
			if vb.Latitude != nil && vb.Longitude != nil && *vb.Latitude != 0 && *vb.Longitude != 0 {
				vendLat = vb.Latitude
				vendLon = vb.Longitude
			}
		}
	}

	if (vendLat == nil || vendLon == nil) && h.wfSvc != nil {
		if coverages, err := h.wfSvc.ListCoverageForOrganization(ctx, vendorOrgID); err == nil && len(coverages) > 0 {
			for _, cov := range coverages {
				if cov.Latitude != nil && cov.Longitude != nil && *cov.Latitude != 0 && *cov.Longitude != 0 {
					vendLat = cov.Latitude
					vendLon = cov.Longitude
					break
				}
			}
		}
	}

	// 3. Compute Distance in Meters (default 5,000 meters / 5 km if in same delivery zone without GPS)
	distMeters := 5000
	if custLat != nil && custLon != nil && vendLat != nil && vendLon != nil {
		distMeters = haversineCoverageDistance(*vendLat, *vendLon, *custLat, *custLon)
	}

	// 4. Match against vendor's DeliveryBands
	fee, matched, err := h.orgSvc.CalculateDeliveryFee(ctx, vendorOrgID, distMeters)
	if err == nil && matched {
		return fee
	}

	return money.Zero
}
