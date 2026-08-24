package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
)

// SupplierDirectoryData is the /suppliers directory view model.
type SupplierDirectoryData struct {
	Suppliers []*org.Organization
	Query     string
}

// SupplierProfileData is the /suppliers/{id} profile view model.
type SupplierProfileData struct {
	Org           *org.Organization
	Products      []*catalog.Product
	Variants      []*catalog.ProductVariant
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
