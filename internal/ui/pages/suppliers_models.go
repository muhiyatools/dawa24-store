package pages

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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
	WorkingHours   string   // e.g. i18n.TDefault("w4m_ui.09_00_06_00_5")
	CoverageDays   string   // e.g. i18n.TDefault("w4m_ui.s_6_6")
	CoverageAreas  []string // e.g. [i18n.TDefault("w4m_ui.s_7_7"), i18n.TDefault("w4m_ui.s_8_8"), i18n.TDefault("w4m_ui.s_9_9")]
	CoverageRadius int      // in km, e.g. 50
	IsOpenNow      bool
	StatusNote     string // e.g. i18n.TDefault("w4_ui.06_00_45")
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
	Suppliers  []*SupplierDirectoryItem
	AllPins    []SuppliersMapItem
	Query      string
	ActiveTab  string // "list" or "map"
	Page       int
	PageSize   int
	TotalCount int
}

// SupplierVariantMeta holds live availability, stock, and coverage state for a variant.
type SupplierVariantMeta struct {
	AvailableStock int
	MinOrderQty    int
	// MaxOrderQty is the most this buyer's branch may actually order right
	// now: the stock, lowered to what is left of the supplier's per-branch
	// quota. It is separate from AvailableStock because the two answer
	// different questions — "how many are on the shelf" is what the badge
	// shows, "how many may I take" is what the number box must not exceed.
	MaxOrderQty    int
	IsCovered      bool
	CoverageReason string
	CanAddToCart   bool
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
	VariantMeta   map[int64]SupplierVariantMeta
	TotalVariants int
	CurrentPage   int
	PerPage       int
	TotalPages    int
	SearchQuery   string
	Reviews       []*org.Review
	Policies      []*org.Policy
	Sections      []*promo.HighlightSection
	IsFollowing   bool
	Rating        float64
	ReviewCount   int
	ActiveTab     string // "catalog", "sections", "policies", "branches", "reviews"
}

// supplierCatalogQuery carries the search box and the active tab through a
// catalogue page change; components.B2BPagination appends page and limit itself.
func supplierCatalogQuery(d SupplierProfileData) url.Values {
	q := url.Values{}
	if d.SearchQuery != "" {
		q.Set("q", d.SearchQuery)
	}
	if d.ActiveTab != "" && d.ActiveTab != "catalog" {
		q.Set("tab", d.ActiveTab)
	}
	return q
}

// GetAvailableStock returns the actual warehouse inventory balance for this variant.
func (d *SupplierProfileData) GetAvailableStock(v *catalog.ProductVariant) int {
	if v == nil {
		return 0
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok {
			return m.AvailableStock
		}
	}
	if v.StockQty > 0 {
		return v.StockQty
	}
	return 0
}

// IsVariantCovered returns whether the variant can be delivered to the buyer's branch.
func (d *SupplierProfileData) IsVariantCovered(v *catalog.ProductVariant) bool {
	if v == nil {
		return false
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok {
			return m.IsCovered
		}
	}
	return true
}

// GetCoverageReason returns a refusal reason or empty string if covered.
func (d *SupplierProfileData) GetCoverageReason(v *catalog.ProductVariant) string {
	if v == nil {
		return ""
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok {
			return m.CoverageReason
		}
	}
	return ""
}

// GetMinOrderQty returns the minimum order quantity for this variant, defaulting to 1.
func (d *SupplierProfileData) GetMinOrderQty(v *catalog.ProductVariant) int {
	if v == nil {
		return 1
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok && m.MinOrderQty > 0 {
			return m.MinOrderQty
		}
	}
	if v.MinOrderQty > 0 {
		return v.MinOrderQty
	}
	return 1
}

// CanAddToCart returns whether the buyer can immediately add this variant to cart.
func (d *SupplierProfileData) CanAddToCart(v *catalog.ProductVariant) bool {
	if v == nil {
		return false
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok {
			return m.CanAddToCart
		}
	}
	return v.StockQty > 0
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

func OrgTypeBadgeClass(t org.OrganizationType) string {
	switch t {
	case org.TypeVendor, "wholesaler":
		return "badge-primary"
	case "distributor":
		return "badge-brand"
	case "manufacturer":
		return "badge-purple"
	default:
		return "badge-slate"
	}
}

// OrgTypeLabel maps an organization type onto an Arabic label.
//
// Two types exist (Rebuild V2 rule 1); legacy values are mapped by migration
// 060, but labels stay tolerant so admin screens can render rows from an old
// dump until the ETL runs.
func OrgTypeLabel(t org.OrganizationType) string {
	switch t {
	case org.TypeVendor:
		return i18n.TDefault("w4_ui.s_192_192")
	case "wholesaler":
		return i18n.TDefault("w4_ui.s_193_193")
	case "distributor":
		return i18n.TDefault("w4_ui.s_194_194")
	case "manufacturer":
		return i18n.TDefault("w4_mod.manufacturer_14")
	case "pharmacy", "chain_pharmacy", "individual":
		return i18n.TDefault("w4_ui.s_195_195")
	default:
		return string(t)
	}
}

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

func dosageFormIcon(form string) string {
	f := strings.ToLower(form)
	switch {
	case strings.Contains(f, "tablet") || strings.Contains(f, "قرص") || strings.Contains(f, "أقراص"):
		return "💊"
	case strings.Contains(f, "syrup") || strings.Contains(f, "شراب"):
		return "🧪"
	case strings.Contains(f, "injection") || strings.Contains(f, "حقن") || strings.Contains(f, "أمبول"):
		return "💉"
	case strings.Contains(f, "cream") || strings.Contains(f, "ointment") || strings.Contains(f, "كريم") || strings.Contains(f, "مرهم"):
		return "🧴"
	case strings.Contains(f, "drop") || strings.Contains(f, "قطرة"):
		return "💧"
	default:
		return "📦"
	}
}

// GetMaxOrderQty is the ceiling for a variant's quantity box.
//
// It falls back to the stock, so a surface that never ran an availability check
// — a signed-out visitor browsing, a buyer with no receiving branch chosen —
// behaves exactly as it did before quotas existed.
func (d *SupplierProfileData) GetMaxOrderQty(v *catalog.ProductVariant) int {
	if v == nil {
		return 0
	}
	if d.VariantMeta != nil {
		if m, ok := d.VariantMeta[v.ID]; ok && m.MaxOrderQty > 0 {
			return m.MaxOrderQty
		}
	}
	return d.GetAvailableStock(v)
}
