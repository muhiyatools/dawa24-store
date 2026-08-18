package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SupplierOffer represents one real vendor offer line shown on the storefront.
// Every price here is resolved through promo.EffectivePrice by the handler.
type SupplierOffer struct {
	OfferID          int64
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

// CatalogFilterParams encapsulates filter inputs for the catalog page.
type CatalogFilterParams struct {
	Query        string
	CategoryID   *int64
	MinPrice     string
	MaxPrice     string
	DosageForm   string
	Sort         string
	InStock      bool
}