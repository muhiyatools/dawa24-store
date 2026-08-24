package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// VendorVariantItem holds a vendor variant with resolved relations.
type VendorVariantItem struct {
	Variant        *catalog.ProductVariant
	DisplayImage   string
	IsParentImage  bool
	OrgName        string
	ParentProdName string
	HasImage       bool
}

// AdminProductChildrenData encapsulates the view model for /admin/product-child.
type AdminProductChildrenData struct {
	Items        []VendorVariantItem
	Total        int
	SearchQuery  string
	StatusFilter string
}
