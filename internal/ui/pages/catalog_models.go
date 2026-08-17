package pages

import (
	"fmt"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SupplierOffer represents a distinct verified vendor/supplier offering a product.
type SupplierOffer struct {
	SupplierID       int64
	SupplierName     string
	SupplierRating   float64
	ReviewCount      int
	IsVerified       bool
	Price            money.Amount
	OldPrice         money.Amount
	DiscountPercent  int
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

// GetMockOffersForProduct returns verified supplier offers for a given product.
func GetOffersForProduct(p *catalog.Product) []SupplierOffer {
	if p == nil {
		return nil
	}

	baseMinor := p.EffectivePrice().Minor()
	if baseMinor <= 0 {
		baseMinor = 3500 // fallback 35.00 EGP
	}

	return []SupplierOffer{
		{
			SupplierID:       101,
			SupplierName:     "مستودع أدوية النيل المركز للتوزيع",
			SupplierRating:   4.9,
			ReviewCount:      214,
			IsVerified:       true,
			Price:            money.FromMinor(baseMinor),
			OldPrice:         money.FromMinor(int64(float64(baseMinor) * 1.15)),
			DiscountPercent:  15,
			AvailableStock:   850,
			MinOrderQty:      5,
			BatchNumber:      fmt.Sprintf("NL-%d82", p.ID*7+10),
			ExpiryDate:       "2028-06",
			DeliveryEstimate: "توصيل سريع خلال 24 ساعة",
			ColdChain:        true,
		},
		{
			SupplierID:       102,
			SupplierName:     "الشركة المتحدة لتوزيع الأدوية (UCP)",
			SupplierRating:   4.8,
			ReviewCount:      189,
			IsVerified:       true,
			Price:            money.FromMinor(int64(float64(baseMinor) * 1.04)),
			OldPrice:         money.FromMinor(int64(float64(baseMinor) * 1.12)),
			DiscountPercent:  8,
			AvailableStock:   1420,
			MinOrderQty:      3,
			BatchNumber:      fmt.Sprintf("UC-%d41", p.ID*3+99),
			ExpiryDate:       "2028-09",
			DeliveryEstimate: "تسليم في نفس اليوم للمحافظات الرئيسية",
			ColdChain:        false,
		},
		{
			SupplierID:       103,
			SupplierName:     "شركة فارما تريد للخدمات الصيدلية",
			SupplierRating:   4.7,
			ReviewCount:      96,
			IsVerified:       true,
			Price:            money.FromMinor(int64(float64(baseMinor) * 1.08)),
			OldPrice:         money.FromMinor(int64(float64(baseMinor) * 1.10)),
			DiscountPercent:  5,
			AvailableStock:   320,
			MinOrderQty:      10,
			BatchNumber:      fmt.Sprintf("PT-%d02", p.ID*11+15),
			ExpiryDate:       "2027-12",
			DeliveryEstimate: "توصيل خلال 48 ساعة",
			ColdChain:        false,
		},
	}
}
