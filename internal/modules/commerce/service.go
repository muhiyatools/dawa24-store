package commerce

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Service manages shopping carts, order checkouts, and state machine transitions.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new commerce service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CheckoutLineItem represents an item to be purchased in an order.
type CheckoutLineItem struct {
	VendorOrgID      int64        `json:"vendor_org_id"`
	ProductID        *int64       `json:"product_id,omitempty"`
	ProductVariantID *int64       `json:"product_variant_id,omitempty"`
	ProductName      i18n.Text    `json:"product_name"`
	VariantName      i18n.Text    `json:"variant_name,omitempty"`
	SKU              string       `json:"sku,omitempty"`
	UnitPrice        money.Amount `json:"unit_price"`
	Quantity         int          `json:"quantity"`
	DiscountAmount   money.Amount `json:"discount_amount"`
}

// CheckoutInput contains all details required to finalize a purchase.
type CheckoutInput struct {
	CustomerID    int64              `json:"customer_id"`
	PaymentMethod string             `json:"payment_method"`
	Notes         string             `json:"notes,omitempty"`
	Items         []CheckoutLineItem `json:"items"`
}

// Checkout processes an order and partitions it into vendor shipments with exact price snapshots.
func (s *Service) Checkout(ctx context.Context, input CheckoutInput) (*Order, error) {
	if input.CustomerID <= 0 {
		return nil, apperr.Validation("checkout.customer_required", "Customer ID is required.", nil)
	}
	if len(input.Items) == 0 {
		return nil, apperr.Validation("checkout.empty_cart", "Cannot checkout an empty cart.", nil)
	}

	now := time.Now().UTC()
	orderNumber := GenerateOrderNumber(now, now.UnixNano())

	// Group line items by vendor organization
	vendorMap := make(map[int64][]*OrderLine)
	var orderSubtotal money.Amount

	for _, item := range input.Items {
		if item.VendorOrgID <= 0 {
			return nil, apperr.Validation("item.vendor_required", "Vendor organization ID is required for each item.", nil)
		}
		if item.Quantity <= 0 {
			return nil, apperr.Validation("item.quantity_invalid", "Quantity must be positive.", nil)
		}

		lineSubtotal, err := item.UnitPrice.MulInt(int64(item.Quantity))
		if err != nil {
			return nil, apperr.Validation("item.price_overflow", "Total price overflow", nil)
		}

		lineTotal := lineSubtotal
		if item.DiscountAmount.IsPositive() && item.DiscountAmount.Minor() < lineSubtotal.Minor() {
			if sub, err := lineSubtotal.Sub(item.DiscountAmount); err == nil {
				lineTotal = sub
			}
		}

		line := &OrderLine{
			OrganizationID:   item.VendorOrgID,
			ProductID:        item.ProductID,
			ProductVariantID: item.ProductVariantID,
			ProductName:      item.ProductName,
			VariantName:      item.VariantName,
			SKU:              item.SKU,
			UnitPrice:        item.UnitPrice,
			Quantity:         item.Quantity,
			DiscountAmount:   item.DiscountAmount,
			TotalPrice:       lineTotal,
		}

		vendorMap[item.VendorOrgID] = append(vendorMap[item.VendorOrgID], line)
		var addErr error
		orderSubtotal, addErr = orderSubtotal.Add(lineTotal)
		if addErr != nil {
			return nil, apperr.Internal(addErr)
		}
	}

	var shipments []*OrderShipment
	var allLines []*OrderLine

	for vendorOrgID, lines := range vendorMap {
		var shipmentSubtotal money.Amount
		for _, line := range lines {
			shipmentSubtotal, _ = shipmentSubtotal.Add(line.TotalPrice)
			allLines = append(allLines, line)
		}

		shipments = append(shipments, &OrderShipment{
			OrganizationID: vendorOrgID,
			Status:         StatusPending,
			Subtotal:       shipmentSubtotal,
			ShippingFee:    money.Zero,
			TotalAmount:    shipmentSubtotal,
			Lines:          lines,
		})
	}

	order := &Order{
		OrderNumber:    orderNumber,
		CustomerID:     input.CustomerID,
		Status:         StatusPending,
		Subtotal:       orderSubtotal,
		DiscountAmount: money.Zero,
		ShippingFee:    money.Zero,
		TaxAmount:      money.Zero,
		TotalAmount:    orderSubtotal,
		PaymentMethod:  input.PaymentMethod,
		PaymentStatus:  PaymentUnpaid,
		Notes:          input.Notes,
		Shipments:      shipments,
	}

	if err := s.repo.CreateOrder(ctx, order, shipments, allLines); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "order created", "order_id", order.ID, "order_number", order.OrderNumber, "vendor_count", len(shipments))
	return order, nil
}

// GetOrder retrieves an order by primary key.
func (s *Service) GetOrder(ctx context.Context, id int64) (*Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

// TransitionOrderStatus validates and applies an order state change.
func (s *Service) TransitionOrderStatus(
	ctx context.Context,
	orderID int64,
	newStatus OrderStatus,
	changedByUserID *int64,
	notes string,
) error {
	history := OrderStatusHistory{
		OrderID:         orderID,
		ToStatus:        string(newStatus),
		Notes:           notes,
		ChangedByUserID: changedByUserID,
	}
	return s.repo.UpdateOrderStatus(ctx, orderID, newStatus, history)
}

// CancelOrder transitions an order to cancelled status.
func (s *Service) CancelOrder(ctx context.Context, orderID int64, changedByUserID *int64, reason string) error {
	return s.TransitionOrderStatus(ctx, orderID, StatusCancelled, changedByUserID, reason)
}

// ListCustomerOrders retrieves paginated orders for a customer.
func (s *Service) ListCustomerOrders(ctx context.Context, customerID int64, limit, offset int) ([]*Order, error) {
	return s.repo.ListOrdersByCustomer(ctx, customerID, limit, offset)
}

// ListVendorShipments retrieves paginated shipments for a vendor.
func (s *Service) ListVendorShipments(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error) {
	return s.repo.ListShipmentsByVendor(ctx, vendorOrgID, limit, offset)
}

// AddToWishlist adds a product to customer's wishlist.
func (s *Service) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return s.repo.AddToWishlist(ctx, userID, productID)
}

// RemoveFromWishlist removes a product from customer's wishlist.
func (s *Service) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return s.repo.RemoveFromWishlist(ctx, userID, productID)
}

// GetWishlist returns all wishlist items for a customer.
func (s *Service) GetWishlist(ctx context.Context, userID int64) ([]*WishlistItem, error) {
	return s.repo.ListWishlist(ctx, userID)
}

// GetCart retrieves or initializes a customer cart.
func (s *Service) GetCart(ctx context.Context, userID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// AddToCart adds or updates an item in the cart.
func (s *Service) AddToCart(ctx context.Context, userID int64, item *CartItem) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddToCartItem(ctx, cart.ID, item); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// RemoveFromCart removes an item from cart.
func (s *Service) RemoveFromCart(ctx context.Context, userID int64, variantID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RemoveCartItem(ctx, cart.ID, variantID); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// ClearCart empties the cart.
func (s *Service) ClearCart(ctx context.Context, userID int64) error {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.ClearCart(ctx, cart.ID)
}

// CreateQuoteRequest creates a new price inquiry from buyer to supplier.
func (s *Service) CreateQuoteRequest(ctx context.Context, q *QuoteRequest) (*QuoteRequest, error) {
	if q.OrganizationID <= 0 || q.CustomerOrgID <= 0 {
		return nil, apperr.Validation("quote.orgs_required", "Vendor and customer organization IDs are required.", nil)
	}
	if q.RequestedQuantity <= 0 {
		return nil, apperr.Validation("quote.qty_invalid", "Requested quantity must be positive.", nil)
	}
	q.Status = QuotePending
	if err := s.repo.CreateQuoteRequest(ctx, q); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "quote requested", "quote_id", q.ID, "vendor_org", q.OrganizationID, "customer_org", q.CustomerOrgID)
	return q, nil
}

// RespondToQuote allows vendor to provide quote unit price or reject.
func (s *Service) RespondToQuote(ctx context.Context, quoteID int64, status QuoteStatus, price money.Amount, notes string) error {
	return s.repo.UpdateQuoteStatus(ctx, quoteID, status, price, notes)
}

// ListQuoteRequests lists quotes for an organization.
func (s *Service) ListQuoteRequests(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error) {
	return s.repo.ListQuoteRequestsByOrg(ctx, orgID, isVendor, limit, offset)
}
