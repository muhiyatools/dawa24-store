package pages

import (
	"encoding/json"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

type VendorCoverageData struct {
	Coverages           []*workflow.CoverageView
	Branches            []*org.Branch
	Bands               []*org.DeliveryBand
	Governorates        []*platformadmin.Governorate
	Cities              []*platformadmin.City
	CoverageUnavailable bool
	NoticeType          string
	NoticeMessage       string
}

// cityClientItem is what the selector needs about one city.
type cityClientItem struct {
	ID        int64   `json:"id"`
	GovID     int64   `json:"gov_id"`
	NameAR    string  `json:"name_ar"`
	NameEN    string  `json:"name_en"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	IsCapital bool    `json:"is_capital"`
	RadiusM   int     `json:"radius_m"`
}

func citiesToJSON(cities []*platformadmin.City) string {
	var list []cityClientItem
	for _, c := range cities {
		var gID int64
		if c.GovernorateID != nil {
			gID = *c.GovernorateID
		}
		list = append(list, cityClientItem{
			ID:        c.ID,
			GovID:     gID,
			NameAR:    c.Name.Get("ar"),
			NameEN:    c.Name.Get("en"),
			Lat:       c.Latitude,
			Lon:       c.Longitude,
			IsCapital: c.IsCapital,
			RadiusM:   c.NormalizedRadius(),
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func dayNameArabic(day int) string {
	switch day {
	case 0:
		return "الأحد"
	case 1:
		return "الاثنين"
	case 2:
		return "الثلاثاء"
	case 3:
		return "الأربعاء"
	case 4:
		return "الخميس"
	case 5:
		return "الجمعة"
	case 6:
		return "السبت"
	default:
		return fmt.Sprintf("يوم %d", day)
	}
}

func dayBadgeClass(day int) string {
	switch day {
	case 0:
		return "badge-primary"
	case 1:
		return "badge-sky"
	case 2:
		return "badge-indigo"
	case 3:
		return "badge-violet"
	case 4:
		return "badge-emerald"
	case 5:
		return "badge-amber"
	case 6:
		return "badge-rose"
	default:
		return "badge-secondary"
	}
}

func countActiveDays(coverages []*workflow.CoverageView) int {
	days := make(map[int]bool)
	for _, c := range coverages {
		if c.IsActive {
			days[c.DayOfWeek] = true
		}
	}
	return len(days)
}

func countCoveredGovernorates(coverages []*workflow.CoverageView) int {
	govs := make(map[string]bool)
	for _, c := range coverages {
		if c.IsActive {
			if c.GovernorateNameAr != "" {
				govs[c.GovernorateNameAr] = true
			} else if c.GovernorateName != "" {
				govs[c.GovernorateName] = true
			}
		}
	}
	return len(govs)
}

func countVendorCoveredCities(coverages []*workflow.CoverageView) int {
	cities := make(map[string]bool)
	for _, c := range coverages {
		if c.IsActive && c.CityID != nil {
			key := fmt.Sprintf("%d_%d", c.DayOfWeek, *c.CityID)
			cities[key] = true
		}
	}
	return len(cities)
}

func formatDistanceKM(meters int) string {
	if meters >= 1000 {
		return fmt.Sprintf("%.1f كم (%d م)", float64(meters)/1000.0, meters)
	}
	return fmt.Sprintf("%d متر", meters)
}