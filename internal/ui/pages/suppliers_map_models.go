package pages

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SuppliersMapItem is the JSON serialization schema for client-side map rendering.
type SuppliersMapItem struct {
	ID           int64   `json:"id"`
	BranchID     int64   `json:"branchId"`
	SupplierName string  `json:"supplierName"`
	BranchName   string  `json:"branchName"`
	Name         string  `json:"name"`
	IsMain       bool    `json:"isMain"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	IsOpen       bool    `json:"isOpen"`
	Hours        string  `json:"hours"`
	StatusNote   string  `json:"statusNote"`
	CoverageDays string  `json:"coverageDays"`
	Address      string  `json:"address"`
	Phone        string  `json:"phone"`
}

// BuildSuppliersMapItems generates individual map pin items for every available branch of each supplier.
func BuildSuppliersMapItems(suppliers []*SupplierDirectoryItem, lang string) []SuppliersMapItem {
	var list []SuppliersMapItem
	for _, s := range suppliers {
		if s == nil || s.Org == nil {
			continue
		}
		sName := s.Org.TradeName.Get(i18n.Lang(lang))
		if sName == "" {
			sName = s.Org.LegalName
		}
		if len(s.Branches) > 0 {
			for _, b := range s.Branches {
				if b == nil || b.Status == "inactive" || b.Status == "suspended" {
					continue
				}
				bName := b.Name.Get(i18n.Lang(lang))
				if bName == "" {
					bName = b.Name.Get(i18n.AR)
				}
				if bName == "" {
					if b.IsMain {
						bName = "الفرع الرئيسي"
					} else {
						bName = fmt.Sprintf("فرع #%d", b.ID)
					}
				}
				displayName := sName
				if bName != "" && !strings.EqualFold(bName, sName) {
					displayName = fmt.Sprintf("%s (%s)", sName, bName)
				}
				lat, lng := resolveBranchCoordinates(b, s.Org.ID, s.Coverages, s.Latitude, s.Longitude)
				addr := b.Address
				if addr == "" {
					addr = s.Address
				}
				phone := b.Phone
				if phone == "" {
					phone = s.Phone
				}
				hours := b.OperatingHours
				if hours == "" {
					hours = s.WorkingHours
				}
				list = append(list, SuppliersMapItem{
					ID:           s.Org.ID,
					BranchID:     b.ID,
					SupplierName: sName,
					BranchName:   bName,
					Name:         displayName,
					IsMain:       b.IsMain,
					Lat:          lat,
					Lng:          lng,
					IsOpen:       s.IsOpenNow,
					Hours:        hours,
					StatusNote:   s.StatusNote,
					CoverageDays: s.CoverageDays,
					Address:      addr,
					Phone:        phone,
				})
			}
		} else {
			list = append(list, SuppliersMapItem{
				ID:           s.Org.ID,
				BranchID:     0,
				SupplierName: sName,
				BranchName:   "الفرع الرئيسي",
				Name:         sName,
				IsMain:       true,
				Lat:          s.Latitude,
				Lng:          s.Longitude,
				IsOpen:       s.IsOpenNow,
				Hours:        s.WorkingHours,
				StatusNote:   s.StatusNote,
				CoverageDays: s.CoverageDays,
				Address:      s.Address,
				Phone:        s.Phone,
			})
		}
	}
	return list
}

func resolveBranchCoordinates(b *org.Branch, orgID int64, coverages []*workflow.CoverageView, sLat, sLng float64) (float64, float64) {
	if b != nil && b.Latitude != nil && b.Longitude != nil && *b.Latitude != 0 && *b.Longitude != 0 {
		return *b.Latitude, *b.Longitude
	}
	if sLat != 0 && sLng != 0 {
		if b != nil && !b.IsMain {
			offsetLat := float64((b.ID*7)%10-5) * 0.004
			offsetLng := float64((b.ID*13)%10-5) * 0.004
			return sLat + offsetLat, sLng + offsetLng
		}
		return sLat, sLng
	}
	for _, c := range coverages {
		if c != nil && c.Latitude != nil && c.Longitude != nil && *c.Latitude != 0 && *c.Longitude != 0 {
			return *c.Latitude, *c.Longitude
		}
	}
	bID := orgID
	if b != nil && b.ID > 0 {
		bID = b.ID
	}
	baseLat := 30.0444 + float64((bID*7)%100)*0.003
	baseLng := 31.2357 + float64((bID*13)%100)*0.003
	return baseLat, baseLng
}

// SuppliersJSON converts a slice of SupplierDirectoryItem into JSON for map initialization.
func SuppliersJSON(suppliers []*SupplierDirectoryItem, lang string) string {
	list := BuildSuppliersMapItems(suppliers, lang)
	bytes, _ := json.Marshal(list)
	return string(bytes)
}

// MapPinsJSON serializes a pre-built slice of SuppliersMapItem.
func MapPinsJSON(pins []SuppliersMapItem) string {
	bytes, _ := json.Marshal(pins)
	return string(bytes)
}

