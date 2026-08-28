package pages

import (
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SupplierDirectoryItem holds rich view data for one supplier in the directory.
type SupplierDirectoryItem struct {
	Org            *org.Organization
	Branches       []*org.Branch
	MainBranch     *org.Branch
	Coverages      []*workflow.CoverageView
	WorkingHours   string   // e.g. "09:00 ص - 06:00 م"
	CoverageDays   string   // e.g. "السبت - الخميس"
	CoverageAreas  []string // e.g. ["القاهرة", "الجيزة", "الإسكندرية"]
	CoverageRadius int      // in km, e.g. 50
	IsOpenNow      bool
	StatusNote     string // e.g. "مفتوح الآن (يغلق 06:00 م)"
	Latitude       float64
	Longitude      float64
	HasCoordinates bool
	MinOrderPrice  money.Amount
	Rating         float64
	ReviewCount    int
	IsFollowing    bool
	TotalProducts  int
	Phone          string
	Address        string
}

// SupplierDirectoryData is the /suppliers directory view model.
type SupplierDirectoryData struct {
	Suppliers []*SupplierDirectoryItem
	Query     string
	ActiveTab string // "list" or "map"
}

// SupplierProfileData is the /suppliers/{id} profile view model.
type SupplierProfileData struct {
	Org           *org.Organization
	Branches      []*org.Branch
	Coverages     []*workflow.CoverageView
	WorkingHours  string
	CoverageDays  string
	CoverageAreas []string
	IsOpenNow     bool
	StatusNote    string
	Products      []*catalog.Product
	Variants      []*catalog.ProductVariant
	ProductsMap   map[int64]*catalog.Product
	TotalVariants int
	CurrentPage   int
	TotalPages    int
	SearchQuery   string
	Reviews       []*org.Review
	Policies      []*org.Policy
	Sections      []*promo.HighlightSection
	IsFollowing   bool
	Rating        float64
	ReviewCount   int
}

// GetProduct returns the master product associated with productID, or nil.
func (d *SupplierProfileData) GetProduct(productID int64) *catalog.Product {
	if d.ProductsMap != nil {
		return d.ProductsMap[productID]
	}
	return nil
}

// GetProductImage returns the image URL from the variant or parent product.
func (d *SupplierProfileData) GetProductImage(v *catalog.ProductVariant) string {
	if v == nil {
		return ""
	}
	if v.Image != "" {
		return v.Image
	}
	if p := d.GetProduct(v.ProductID); p != nil {
		if p.Image != "" {
			return p.Image
		}
		if p.ImageLink != "" {
			return p.ImageLink
		}
	}
	return ""
}

// GetDosageForm returns the dosage form if available.
func (d *SupplierProfileData) GetDosageForm(v *catalog.ProductVariant) string {
	if v == nil {
		return ""
	}
	if p := d.GetProduct(v.ProductID); p != nil && p.DosageForm != "" {
		return p.DosageForm
	}
	return ""
}

// GetScientificName returns the scientific/active ingredient name.
func (d *SupplierProfileData) GetScientificName(v *catalog.ProductVariant) string {
	if v == nil {
		return ""
	}
	if p := d.GetProduct(v.ProductID); p != nil && p.ScientificName != "" {
		return p.ScientificName
	}
	return ""
}

// GetConcentration returns the product concentration if available.
func (d *SupplierProfileData) GetConcentration(v *catalog.ProductVariant) string {
	if v == nil {
		return ""
	}
	if p := d.GetProduct(v.ProductID); p != nil && p.Concentration != "" {
		return p.Concentration
	}
	return ""
}

// OrgTypeLabel maps an organization type onto an Arabic label.
//
// Two types exist (Rebuild V2 rule 1); legacy values are mapped by migration
// 060, but labels stay tolerant so admin screens can render rows from an old
// dump until the ETL runs.
func OrgTypeLabel(t org.OrganizationType) string {
	switch t {
	case org.TypeVendor:
		return "مورّد / شركة أدوية"
	case org.TypeCustomer:
		return "صيدلية"
	case "supplier", "company", "agency":
		return "مورّد / شركة أدوية"
	case "pharmacy", "chain_pharmacy", "individual":
		return "صيدلية"
	default:
		return string(t)
	}
}

// SuppliersMapItem is the JSON serialization schema for client-side map rendering.
type SuppliersMapItem struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	IsOpen       bool    `json:"isOpen"`
	Hours        string  `json:"hours"`
	StatusNote   string  `json:"statusNote"`
	CoverageDays string  `json:"coverageDays"`
	Address      string  `json:"address"`
	Phone        string  `json:"phone"`
}

// SuppliersJSON converts a slice of SupplierDirectoryItem into JSON for map initialization.
func SuppliersJSON(suppliers []*SupplierDirectoryItem, lang string) string {
	var list []SuppliersMapItem
	for _, s := range suppliers {
		if s == nil || s.Org == nil {
			continue
		}
		name := s.Org.TradeName.Get(i18n.Lang(lang))
		if name == "" {
			name = s.Org.LegalName
		}
		list = append(list, SuppliersMapItem{
			ID:           s.Org.ID,
			Name:         name,
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
	bytes, _ := json.Marshal(list)
	return string(bytes)
}
