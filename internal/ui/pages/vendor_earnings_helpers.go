package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// VendorEarningsOrderPageData holds the financial summary, paged slices and pagination controls.
type VendorEarningsOrderPageData struct {
	Summary            *commerce.VendorFinancialSummary
	PagedShipments     []*commerce.VendorShipmentProfit
	OrdersPagination   components.PaginationProps
	PagedProducts      []*commerce.VendorProductProfit
	ProductsPagination components.PaginationProps
	Lang               string
	Dir                string
}
