package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

func newTestCoverageHandler() *UIHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
}

func TestCheckOfferCoverage_NilInputs(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	cityID := int64(1)
	branch := &org.Branch{ID: 10, CityID: &cityID}
	offer := &promo.SpecialOffer{ID: 100}

	// 1. Nil offer
	covered, reason := h.checkOfferCoverage(ctx, nil, branch)
	if covered {
		t.Errorf("expected covered=false for nil offer, got true")
	}
	if reason == "" {
		t.Errorf("expected non-empty reason for nil offer")
	}

	// 2. Nil branch
	covered, reason = h.checkOfferCoverage(ctx, offer, nil)
	if covered {
		t.Errorf("expected covered=false for nil branch, got true")
	}
	if reason == "" {
		t.Errorf("expected non-empty reason for nil branch")
	}
}

func TestCheckOfferCoverage_DirectCityMatch(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	city1 := int64(10)
	city2 := int64(20)

	branch := &org.Branch{
		ID:     1,
		CityID: &city1,
	}

	offer := &promo.SpecialOffer{
		ID:             101,
		OrganizationID: 5,
		Locations: []*promo.SpecialOfferLocation{
			{
				ID:      1,
				OfferID: 101,
				CityID:  &city2,
				Status:  "active",
			},
			{
				ID:      2,
				OfferID: 101,
				CityID:  &city1,
				Status:  "active",
			},
		},
	}

	covered, reason := h.checkOfferCoverage(ctx, offer, branch)
	if !covered {
		t.Fatalf("expected covered=true for city match, got false (%s)", reason)
	}
	if reason != "مشمول بالتغطية في مدينتك" {
		t.Errorf("expected reason 'مشمول بالتغطية في مدينتك', got %q", reason)
	}
}

func TestCheckOfferCoverage_SpatialHaversineWithinRadius(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	offerCity := int64(10)
	branchCity := int64(20) // Different city ID

	// Offer location in Baghdad (33.3152, 44.3661) with 10 km radius
	offerLat := 33.3152
	offerLon := 44.3661
	offerRadius := 10000 // 10 km

	// Branch in Karrada (~3.5 km away)
	branchLat := 33.3000
	branchLon := 44.4000

	branch := &org.Branch{
		ID:        1,
		CityID:    &branchCity,
		Latitude:  &branchLat,
		Longitude: &branchLon,
	}

	offer := &promo.SpecialOffer{
		ID:             102,
		OrganizationID: 5,
		Locations: []*promo.SpecialOfferLocation{
			{
				ID:        1,
				OfferID:   102,
				CityID:    &offerCity,
				Latitude:  offerLat,
				Longitude: offerLon,
				Radius:    offerRadius,
				Status:    "active",
			},
		},
	}

	covered, reason := h.checkOfferCoverage(ctx, offer, branch)
	if !covered {
		t.Fatalf("expected covered=true for spatial match within 10km, got false (%s)", reason)
	}
	if reason != "مشمول بنطاق التغطية الجغرافي للعرض" {
		t.Errorf("expected reason 'مشمول بنطاق التغطية الجغرافي للعرض', got %q", reason)
	}
}

func TestCheckOfferCoverage_OutsideCoverage(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	offerCity := int64(10)
	branchCity := int64(20)

	// Offer in Baghdad with 5 km radius
	offerLat := 33.3152
	offerLon := 44.3661
	offerRadius := 5000 // 5 km

	// Branch in Erbil (~320 km away)
	branchLat := 36.1912
	branchLon := 44.0091

	branch := &org.Branch{
		ID:        1,
		CityID:    &branchCity,
		Latitude:  &branchLat,
		Longitude: &branchLon,
	}

	offer := &promo.SpecialOffer{
		ID:             103,
		OrganizationID: 5,
		Locations: []*promo.SpecialOfferLocation{
			{
				ID:        1,
				OfferID:   103,
				CityID:    &offerCity,
				Latitude:  offerLat,
				Longitude: offerLon,
				Radius:    offerRadius,
				Status:    "active",
			},
		},
	}

	covered, reason := h.checkOfferCoverage(ctx, offer, branch)
	if covered {
		t.Fatalf("expected covered=false for branch outside radius and different city, got true")
	}
	if reason != "فرع الصيدلية خارج النطاق الجغرافي المحدد لهذا العرض" {
		t.Errorf("expected outside coverage reason, got %q", reason)
	}
}

func TestCheckOfferCoverage_InactiveLocationsIgnored(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	city1 := int64(10)

	branch := &org.Branch{
		ID:     1,
		CityID: &city1,
	}

	// Only inactive location exists
	offer := &promo.SpecialOffer{
		ID:             104,
		OrganizationID: 5,
		Locations: []*promo.SpecialOfferLocation{
			{
				ID:      1,
				OfferID: 104,
				CityID:  &city1,
				Status:  "inactive",
			},
		},
	}

	// Should fall back to default coverage since no active locations exist
	covered, reason := h.checkOfferCoverage(ctx, offer, branch)
	if !covered {
		t.Fatalf("expected covered=true fallback when only inactive locations exist, got false (%s)", reason)
	}
	if reason != "مشمول بالتغطية" {
		t.Errorf("expected fallback reason 'مشمول بالتغطية', got %q", reason)
	}
}

func TestCheckOfferCoverage_FallbackDefaultWithoutLocations(t *testing.T) {
	h := newTestCoverageHandler()
	ctx := context.Background()

	city1 := int64(10)
	branch := &org.Branch{
		ID:     1,
		CityID: &city1,
	}

	offer := &promo.SpecialOffer{
		ID:             105,
		OrganizationID: 5,
		Locations:      nil,
	}

	covered, reason := h.checkOfferCoverage(ctx, offer, branch)
	if !covered {
		t.Fatalf("expected covered=true fallback without locations, got false")
	}
	if reason != "مشمول بالتغطية" {
		t.Errorf("expected 'مشمول بالتغطية', got %q", reason)
	}
}
