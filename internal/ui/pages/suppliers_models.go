package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// SupplierDirectoryData is the /suppliers directory view model.
type SupplierDirectoryData struct {
	Suppliers []*org.Organization
}

// SupplierProfileData is the /suppliers/{id} profile view model.
type SupplierProfileData struct {
	Org         *org.Organization
	Products    []*catalog.Product
	Reviews     []*org.Review
	Policies    []*org.Policy
	IsFollowing bool
	Rating      float64
	ReviewCount int
}

// OrgTypeLabel maps an organization type onto an Arabic label.
func OrgTypeLabel(t org.OrganizationType) string {
	switch t {
	case org.TypeSupplier:
		return "مورّد / شركة أدوية"
	case org.TypePharmacy:
		return "صيدلية"
	case org.TypeChainPharmacy:
		return "سلسلة صيدليات"
	case "company":
		return "شركة"
	case "agency":
		return "وكالة"
	default:
		return string(t)
	}
}
