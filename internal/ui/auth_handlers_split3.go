package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// landingPathForActor routes an authenticated actor to their home surface.
func landingPathForActor(actor authctx.Actor) string {
	if actor.IsStaff {
		return "/admin/dashboard"
	}
	switch actor.OrgStatus {
	case "pending", "under_review":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	case "suspended":
		return "/onboarding/pending?state=suspended"
	}
	switch actor.OrgType {
	case "vendor":
		return "/vendor/dashboard"
	case "customer":
		return "/customer/dashboard"
	}
	return "/catalog"
}

func (h *UIHandler) findNearestCityID(ctx context.Context, lat, lon float64) int64 {
	cities := h.listCities(ctx)
	if len(cities) == 0 {
		return 1
	}
	var bestID int64 = cities[0].ID
	var minDist float64 = 1e9
	for _, c := range cities {
		if c.Latitude == 0 && c.Longitude == 0 {
			continue
		}
		dLat := lat - c.Latitude
		dLon := lon - c.Longitude
		dist := dLat*dLat + dLon*dLon
		if dist < minDist {
			minDist = dist
			bestID = c.ID
		}
	}
	return bestID
}
