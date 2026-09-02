package http_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	r.fail("GetOrCreateCart")
	return nil, nil
}
func (r stubRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	r.fail("GetCartWithItems")
	return nil, nil
}
func (r stubRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	r.fail("AddToCartItem")
	return nil
}
func (r stubRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	r.fail("SetCartItemQuantity")
	return nil
}
func (r stubRepo) RemoveCartItemByID(ctx context.Context, cartID, itemID int64) error {
	r.fail("RemoveCartItemByID")
	return nil
}

func (r stubRepo) SetCartItemQuantityByID(ctx context.Context, cartID, itemID int64, qty int) error {
	r.fail("SetCartItemQuantityByID")
	return nil
}

func (r stubRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	r.fail("RemoveCartItem")
	return nil
}
func (r stubRepo) ClearCart(ctx context.Context, cartID int64) error {
	r.fail("ClearCart")
	return nil
}

func (r stubRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	r.fail("CreateOrder")
	return nil
}
func (r stubRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	r.fail("GetOrderByID")
	return nil, nil
}
func (r stubRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	r.fail("GetOrderByNumber")
	return nil, nil
}
func (r stubRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateOrderStatus")
	return nil
}
func (r stubRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	r.fail("UpdateCustomerPendingOrder")
	return nil, nil
}
func (r stubRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	r.fail("ListOrdersByCustomer")
	return nil, nil
}
func (r stubRepo) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	r.fail("ListOrdersByCustomerWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	r.fail("ListShipmentsByVendor")
	return nil, nil
}
func (r stubRepo) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	r.fail("ListShipmentsByVendorWithTotal")
	return nil, 0, nil
}
func (r stubRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	r.fail("GetShipmentByID")
	return nil, nil
}
func (r stubRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateShipmentStatus")
	return nil
}
func (r stubRepo) GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*commerce.OrderShipment, error) {
	r.fail("GetShipmentForDeliveryByTracking")
	return nil, nil
}
func (r stubRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	r.fail("VerifyAndCompleteDelivery")
	return nil, nil
}
func (r stubRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	r.fail("ListOrderHistory")
	return nil, nil
}
func (r stubRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	r.fail("RateOrder")
	return nil
}

func (r stubRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("AddToWishlist")
	return nil
}
func (r stubRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("RemoveFromWishlist")
	return nil
}
func (r stubRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	r.fail("ListWishlist")
	return nil, nil
}

func (r stubRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	r.fail("CreateQuoteRequest")
	return nil
}
func (r stubRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	r.fail("GetQuoteRequestByID")
	return nil, nil
}
func (r stubRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	r.fail("UpdateQuoteStatus")
	return nil
}
func (r stubRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	r.fail("ListQuoteRequestsByOrg")
	return nil, nil
}

func (r stubRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	r.fail("AdminSearchOrders")
	return nil, nil
}
func (r stubRepo) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	r.fail("AdminSearchOrdersWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminOrderStats(ctx context.Context) (int, int, int, error) {
	r.fail("AdminOrderStats")
	return 0, 0, 0, nil
}
func (r stubRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	r.fail("CreatePurchaseRequest")
	return nil
}
func (r stubRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByID")
	return nil, nil
}
func (r stubRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByNumber")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByVendor")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, int, error) {
	r.fail("ListPurchaseRequestsByVendorWithTotal")
	return nil, 0, nil
}
func (r stubRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	r.fail("CountPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	r.fail("UpdatePurchaseRequestStatus")
	return nil
}
func (r stubRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	r.fail("UpdatePurchaseRequestLineOffer")
	return nil
}
func (r stubRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	r.fail("AcceptNegotiation")
	return nil
}
func (r stubRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	r.fail("RejectNegotiation")
	return nil
}
func (r stubRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*commerce.VendorFinancialSummary, error) {
	r.fail("GetVendorFinancialSummary")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: 1, UserID: userID}, nil
}
func (happyRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: cartID, UserID: 1, Items: []*commerce.CartItem{{ID: 1, CartID: cartID, ProductID: 1, ProductVariantID: 1, Quantity: 2, UnitPrice: money.MustParse("50.00")}}}, nil
}
func (happyRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	return nil
}
func (happyRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	return nil
}
func (happyRepo) RemoveCartItemByID(ctx context.Context, cartID, itemID int64) error { return nil }

func (happyRepo) SetCartItemQuantityByID(ctx context.Context, cartID, itemID int64, qty int) error {
	return nil
}

func (happyRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	return nil
}
func (happyRepo) ClearCart(ctx context.Context, cartID int64) error {
	return nil
}
func (happyRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	order.ID = 1
	order.OrderNumber = "ORD-2026-0001"
	return nil
}
func (happyRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, CustomerID: 1, OrderNumber: "ORD-1", Status: commerce.StatusDelivered, TotalAmount: money.MustParse("100.00")}, nil
}
func (happyRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	return &commerce.Order{ID: 1, CustomerID: 1, OrderNumber: number, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: customerID, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) ListOrdersByCustomerWithTotal(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, int, error) {
	return []*commerce.Order{{ID: 1, CustomerID: customerID, OrderNumber: "ORD-1"}}, 1, nil
}
func (happyRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return []*commerce.OrderShipment{{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"}}, nil
}
func (happyRepo) ListShipmentsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.OrderShipment, int, error) {
	return []*commerce.OrderShipment{{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"}}, 1, nil
}
func (happyRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrganizationID: 1, Status: commerce.StatusPending}, nil
}
func (happyRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: 1, ShipmentNumber: "SH-1", TrackingNumber: tracking, Status: commerce.StatusShipped, DeliveryCode: "123456"}, nil
}
func (happyRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: shipmentID, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return []*commerce.OrderStatusHistory{{ID: 1, OrderID: orderID, ToStatus: string(commerce.StatusPending)}}, nil
}
func (happyRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	return nil
}
func (happyRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return []*commerce.WishlistItem{{ID: 1, UserID: userID, ProductID: 1}}, nil
}
func (happyRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	qr.ID = 1
	return nil
}
func (happyRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	return &commerce.QuoteRequest{ID: id, OrganizationID: 1, CustomerOrgID: 2, Status: commerce.QuotePending}, nil
}
func (happyRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	return nil
}
func (happyRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return []*commerce.QuoteRequest{{ID: 1, OrganizationID: orgID, Status: commerce.QuotePending}}, nil
}
func (happyRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, 1, nil
}
func (happyRepo) AdminOrderStats(ctx context.Context) (int, int, int, error) {
	return 1, 1, 0, nil
}
func (happyRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	pr.ID = 1
	pr.RequestNumber = "PR-2026-0001"
	return nil
}
func (happyRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: id, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: 1, RequestNumber: number, Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) ListPurchaseRequestsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, int, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, 1, nil
}
func (happyRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return map[string]int{"all": 1, "pending": 1}, nil
}
func (happyRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	return nil
}
func (happyRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return nil
}
func (happyRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return nil
}
func (happyRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return nil
}
func (happyRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	return order, nil
}
func (happyRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}
