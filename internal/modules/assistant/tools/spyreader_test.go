package tools_test

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// spyReader records every read that reaches the data layer. A denied call must
// leave calls empty: refusing after fetching is not refusing.
type spyReader struct {
	calls []string
}

func (s *spyReader) note(name string) { s.calls = append(s.calls, name) }

func (s *spyReader) Branches(context.Context, authctx.Actor) ([]assistant.BranchRow, error) {
	s.note("Branches")
	return []assistant.BranchRow{{ID: 11, Name: "الفرع الرئيسي", IsMain: true, Status: "active"}}, nil
}

func (s *spyReader) Wallet(context.Context, authctx.Actor) (*assistant.WalletSummary, error) {
	s.note("Wallet")
	return &assistant.WalletSummary{Currency: "EGP"}, nil
}

func (s *spyReader) Subscription(context.Context, authctx.Actor) (*assistant.SubscriptionSummary, error) {
	s.note("Subscription")
	return &assistant.SubscriptionSummary{PlanName: "الباقة الفضية", Status: "active"}, nil
}

func (s *spyReader) PurchaseOrders(context.Context, authctx.Actor, assistant.OrderQuery) (assistant.Page[assistant.PurchaseOrderRow], error) {
	s.note("PurchaseOrders")
	return assistant.Page[assistant.PurchaseOrderRow]{
		Rows: []assistant.PurchaseOrderRow{{ID: 501, Number: "PO-1"}},
	}, nil
}

func (s *spyReader) PurchaseOrderDetail(_ context.Context, _ authctx.Actor, id int64) (*assistant.OrderDetail, error) {
	s.note("PurchaseOrderDetail")
	return &assistant.OrderDetail{Order: assistant.PurchaseOrderRow{ID: id, Number: "PO-1"}}, nil
}

func (s *spyReader) PurchaseSummary(context.Context, authctx.Actor, assistant.AggregateQuery) (*assistant.Aggregate, error) {
	s.note("PurchaseSummary")
	return &assistant.Aggregate{Count: 3}, nil
}

func (s *spyReader) PurchasedProducts(context.Context, authctx.Actor, assistant.AggregateQuery) (assistant.Page[assistant.ProductSpendRow], error) {
	s.note("PurchasedProducts")
	return assistant.Page[assistant.ProductSpendRow]{}, nil
}

func (s *spyReader) MarketProducts(context.Context, authctx.Actor, assistant.ProductQuery) (assistant.Page[assistant.MarketProductRow], error) {
	s.note("MarketProducts")
	return assistant.Page[assistant.MarketProductRow]{}, nil
}

func (s *spyReader) SupplyOrders(context.Context, authctx.Actor, assistant.OrderQuery) (assistant.Page[assistant.SupplyOrderRow], error) {
	s.note("SupplyOrders")
	return assistant.Page[assistant.SupplyOrderRow]{}, nil
}

func (s *spyReader) SupplyOrderDetail(context.Context, authctx.Actor, int64) (*assistant.SupplyOrderDetail, error) {
	s.note("SupplyOrderDetail")
	return nil, nil
}

func (s *spyReader) SalesSummary(context.Context, authctx.Actor, assistant.AggregateQuery) (*assistant.Aggregate, error) {
	s.note("SalesSummary")
	return &assistant.Aggregate{Count: 1}, nil
}

func (s *spyReader) SoldProducts(context.Context, authctx.Actor, assistant.AggregateQuery) (assistant.Page[assistant.SoldProductRow], error) {
	s.note("SoldProducts")
	return assistant.Page[assistant.SoldProductRow]{}, nil
}

func (s *spyReader) VendorProducts(context.Context, authctx.Actor, assistant.ProductQuery) (assistant.Page[assistant.VendorProductRow], error) {
	s.note("VendorProducts")
	return assistant.Page[assistant.VendorProductRow]{}, nil
}

func (s *spyReader) LowStock(context.Context, authctx.Actor, int) (assistant.Page[assistant.LowStockRow], error) {
	s.note("LowStock")
	return assistant.Page[assistant.LowStockRow]{}, nil
}

func (s *spyReader) Offers(context.Context, authctx.Actor, bool, int) (assistant.Page[assistant.OfferRow], error) {
	s.note("Offers")
	return assistant.Page[assistant.OfferRow]{}, nil
}

func (s *spyReader) Organizations(context.Context, authctx.Actor, assistant.ProductQuery) (assistant.Page[assistant.OrganizationRow], error) {
	s.note("Organizations")
	return assistant.Page[assistant.OrganizationRow]{}, nil
}

func (s *spyReader) PlatformOverview(context.Context, authctx.Actor, assistant.DateRange) (*assistant.PlatformSummary, error) {
	s.note("PlatformOverview")
	return &assistant.PlatformSummary{}, nil
}

func (s *spyReader) AIUsage(context.Context, authctx.Actor, assistant.DateRange, int) (assistant.Page[assistant.AIUsageRow], error) {
	s.note("AIUsage")
	return assistant.Page[assistant.AIUsageRow]{}, nil
}
