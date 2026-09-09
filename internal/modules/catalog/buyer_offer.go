package catalog

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// BuyerOfferQuery is one page of the buying catalogue, from one buyer's point of view.
type BuyerOfferQuery struct {
	BuyerOrgID     int64   // excluded as a seller; 0 for a signed-out visitor
	BuyerBranchID  int64   // 0 when no branch is selected
	AllowedWorkIDs []int64 // institutional works the buyer branch may buy from
	Weekday        int
	Query          string
	CategoryID     *int64
	BrandID        *int64
	DosageForm     string
	MinPriceMinor  *int64
	MaxPriceMinor  *int64
	OnlyDiscounted bool
	OnlyInStock    bool
	Sort           string
	Limit, Offset  int
	SupplierOrgID  int64 // needed for scoping to one supplier on /suppliers/{id}
}

// BuyerOffer is one sellable supplier offer joined with product and vendor details.
type BuyerOffer struct {
	VariantID            int64        `json:"variant_id"`
	ProductID            int64        `json:"product_id"`
	VendorOrgID          int64        `json:"vendor_org_id"`
	VendorOrgName        string       `json:"vendor_org_name"`
	VendorBranchID       *int64       `json:"vendor_branch_id,omitempty"`
	VendorBranchName     string       `json:"vendor_branch_name,omitempty"`
	CityName             string       `json:"city_name,omitempty"`
	GovernorateName      string       `json:"governorate_name,omitempty"`
	VendorLatitude       *float64     `json:"vendor_latitude,omitempty"`
	VendorLongitude      *float64     `json:"vendor_longitude,omitempty"`
	ProductName          i18n.Text    `json:"product_name"`
	ProductImage         string       `json:"product_image,omitempty"`
	ProductSKU           string       `json:"product_sku,omitempty"`
	ProductBarcode       string       `json:"product_barcode,omitempty"`
	ScientificName       string       `json:"scientific_name,omitempty"`
	DosageForm           string       `json:"dosage_form,omitempty"`
	ManufacturingCompany string       `json:"manufacturing_company,omitempty"`
	BrandID              *int64       `json:"brand_id,omitempty"`
	BrandName            string       `json:"brand_name,omitempty"`
	BrandLogo            string       `json:"brand_logo,omitempty"`
	CategoryID           *int64       `json:"category_id,omitempty"`
	VariantName          i18n.Text    `json:"variant_name"`
	VariantSKU           string       `json:"variant_sku,omitempty"`
	VariantImage         string       `json:"variant_image,omitempty"`
	PublicPrice          money.Amount `json:"public_price"`
	Price                money.Amount `json:"price"`
	OldPrice             money.Amount `json:"old_price"`
	Discount             money.Amount `json:"discount"`
	DiscountPercent      int          `json:"discount_percent"`
	AvailableStock       int          `json:"available_stock"`
	MinOrderQty          int          `json:"min_order_qty"`
	QuotaLimit           int          `json:"quota_limit"`
	ExpiryDate           *time.Time   `json:"expiry_date,omitempty"`
	IsFeatured           bool         `json:"is_featured"`
	IsNegotiable         bool         `json:"is_negotiable"`
	IsSponsored          bool         `json:"is_sponsored"`
	SponsoredTier        int          `json:"sponsored_tier"`
	Status               string       `json:"status"`
}
