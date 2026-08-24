package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SupplierVariantCard represents an enriched product variant supplied by a specific vendor.
// This is the core storefront item for the pharmacy catalog (Rebuild V2).
type SupplierVariantCard struct {
	VariantID       int64
	ProductID       int64
	ProductNameAr   string
	ProductNameEn   string
	ProductImage    string
	VariantName     string
	SKU             string
	DosageForm      string
	Manufacturer    string
	BrandID         *int64
	BrandName       string
	BrandLogo       string
	ScientificName  string
	PublicPrice     money.Amount // Suggested consumer retail price
	SupplierID      int64
	SupplierName    string
	SupplierLogo    string
	SupplierRating  float64
	IsVerified      bool
	BranchName      string
	CityName        string
	Price           money.Amount // Net pharmacy price
	OriginalPrice   money.Amount // Original list price before offer
	DiscountPercent int          // 15 = 15%
	AvailableStock  int
	MinOrderQty     int
	BatchNumber     string
	ExpiryDate      string
	IsCovered       bool
	CoverageReason  string
	CanAddToCart    bool
	IsNegotiable    bool
}

// SupplierOffer represents one real vendor offer line shown on the storefront.
type SupplierOffer struct {
	OfferID          int64
	VariantID        int64
	SupplierID       int64
	SupplierName     string
	SupplierRating   float64
	ReviewCount      int
	IsVerified       bool
	Price            money.Amount // final price (after offers) — the buy price
	OldPrice         money.Amount // list price before the offer
	DiscountAmount   money.Amount // saved amount off the list price
	DiscountBPS      int64        // effective percent in basis points (1500 = 15%)
	AvailableStock   int
	MinOrderQty      int
	BatchNumber      string
	ExpiryDate       string
	DeliveryEstimate string
	ColdChain        bool
	BranchName       string
	WarehouseName    string
	CityName         string
	IsCovered        bool
	CoverageReason   string // e.g. "مفيش فرع بيوصل لموقعك للمنتج ده"
	CanAddToCart     bool
	IsNegotiable     bool
}

// VariantDetailPageData contains full data for the dedicated Product Variant Details page.
type VariantDetailPageData struct {
	Variant        *catalog.ProductVariant
	Product        *catalog.Product
	SupplierOrg    *org.Organization
	SupplierBranch *org.Branch
	CurrentOffer   SupplierOffer
	OtherOffers    []SupplierOffer
	AllVariants    []*catalog.ProductVariant
	StockQty       int
	IsCovered      bool
	CoverageReason string
	Rating         float64
	ReviewCount    int
}

// ProductDetailViewData encapsulates complete B2B pharmaceutical presentation.
type ProductDetailViewData struct {
	Product        *catalog.Product
	Variants       []*catalog.ProductVariant
	SupplierOffers []SupplierOffer
	Rating         float64
	ReviewCount    int
	LowestPrice    money.Amount
}

// CatalogPageData encapsulates filter inputs and variant card results for the catalog page.
type CatalogPageData struct {
	Variants   []*SupplierVariantCard
	Categories []*catalog.Category
	Query      string
	CategoryID *int64
	MinPrice   string
	MaxPrice   string
	DosageForm string
	Sort       string
	InStock    bool
}

// CatalogFilterParams encapsulates filter inputs for the catalog page.
type CatalogFilterParams struct {
	Query      string
	CategoryID *int64
	MinPrice   string
	MaxPrice   string
	DosageForm string
	Sort       string
	InStock    bool
}

// IngestWizardData contains comprehensive state for the multi-step ingest wizard.
type IngestWizardData struct {
	Step             int // 1 = Upload, 2 = Mapping, 3 = Review & Commit
	Session          *ingest.ImportSession
	Sessions         []*ingest.ImportSession
	Rows             []*ingest.ImportRow
	Warehouses       []*inventory.Warehouse
	MasterProducts   []*catalog.Product
	CurrentWarehouse *inventory.Warehouse
	NoticeType       string
	NoticeMessage    string
	ConfidenceFilter string
}
