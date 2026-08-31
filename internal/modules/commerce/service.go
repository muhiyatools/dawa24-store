package commerce

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

var orderCounter atomic.Int64

func init() {
	orderCounter.Store(time.Now().Unix() % 10000)
}

// Service manages shopping carts, order checkouts, and state machine transitions.
type Service struct {
	repo Repository
	log  *slog.Logger

	reqDocs RequiredDocsChecker
	// availability answers the stock / coverage / branch questions that live in
	// other modules. Injected at the composition root; see AvailabilityProbe.
	availability AvailabilityProbe
}

// SetAvailabilityProbe installs the cross-module availability probe. Without
// it CheckAvailability fails closed, because a purchase that cannot be
// verified must not be allowed through.
func (s *Service) SetAvailabilityProbe(p AvailabilityProbe) {
	s.availability = p
}

// SetRequiredDocsChecker installs the §4.2 documents gate. When set, Checkout
// refuses to proceed for organizations with missing mandatory documents.
func (s *Service) SetRequiredDocsChecker(fn RequiredDocsChecker) {
	s.reqDocs = fn
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
	VendorOrgID            int64         `json:"vendor_org_id"`
	ProductID              *int64        `json:"product_id,omitempty"`
	ProductVariantID       *int64        `json:"product_variant_id,omitempty"`
	ProductName            i18n.Text     `json:"product_name"`
	VariantName            i18n.Text     `json:"variant_name,omitempty"`
	SKU                    string        `json:"sku,omitempty"`
	OfferProductID         *int64        `json:"offer_product_id,omitempty"` // offer_product line sold under (063)
	UnitPrice              money.Amount  `json:"unit_price"`
	Quantity               int           `json:"quantity"`
	DiscountAmount         money.Amount  `json:"discount_amount"`
	CostPrice              *money.Amount `json:"cost_price,omitempty"`
	CostDiscountPercentage float64       `json:"cost_discount_percentage"`
	ListPrice              money.Amount  `json:"list_price,omitempty"`        // pre-discount strike price (063)
	OriginalPrice          money.Amount  `json:"original_price,omitempty"`    // legacy price snapshot (063)
	OriginalDiscount       money.Amount  `json:"original_discount,omitempty"` // legacy discount snapshot (063)
	IsNegotiated           bool          `json:"is_negotiated"`
	ProposedUnitPrice      money.Amount  `json:"proposed_unit_price,omitempty"`
}

// CheckoutInput contains all details required to finalize a purchase.
type CheckoutInput struct {
	CustomerID         int64                  `json:"customer_id"`
	OfferID            int64                  `json:"offer_id"`                   // the offer this order belongs to (063)
	BranchID           *int64                 `json:"branch_id,omitempty"`        // customer branch buying for
	VendorBranchID     *int64                 `json:"vendor_branch_id,omitempty"` // fulfilling vendor branch
	UserAddressID      *int64                 `json:"user_address_id,omitempty"`  // delivery address (063)
	PaymentMethod      string                 `json:"payment_method"`
	ShippingFee        money.Amount           `json:"shipping_fee,omitempty"`
	VendorShippingFees map[int64]money.Amount `json:"vendor_shipping_fees,omitempty"` // distance delivery fee per vendor shipment
	TaxAmount          money.Amount           `json:"tax_amount,omitempty"`
	Notes              string                 `json:"notes,omitempty"`
	Items              []CheckoutLineItem     `json:"items"`
	// MinOrderAmount is the approved offer's minimum order amount. It is
	// supplied by the caller (which owns the offer data) and enforced here so
	// every checkout path is gated in one place.
	MinOrderAmount money.Amount `json:"min_order_amount,omitempty"`
	// CustomerOrgID is the buyer's organization. The documents gate
	// (Rebuild V2 §4.2) checks it; the API/UI layers fill it from the actor.
	CustomerOrgID    int64  `json:"customer_org_id,omitempty"`
	IsNegotiation    bool   `json:"is_negotiation"`
	NegotiationNotes string `json:"negotiation_notes,omitempty"`
}

// RequiredDocsChecker is injected from composition root (Rebuild V2 §4.2): it
// must return an error when the organization cannot trade (missing mandatory
// documents). The commerce module cannot import the attachments module, so the
// checker is a plain function set by cmd/server.
type RequiredDocsChecker func(ctx context.Context, orgID int64, orgType string) error

// Checkout processes an order and partitions it into vendor shipments with exact price snapshots.
func (s *Service) Checkout(ctx context.Context, input CheckoutInput) (*Order, error) {
	if input.CustomerID <= 0 {
		return nil, apperr.Validation("checkout.customer_required", "Customer ID is required.", nil)
	}
	if len(input.Items) == 0 {
		return nil, apperr.Validation("checkout.empty_cart", "Cannot checkout an empty cart.", nil)
	}
	if s.reqDocs != nil && input.CustomerOrgID > 0 {
		if err := s.reqDocs(ctx, input.CustomerOrgID, "customer"); err != nil {
			return nil, err
		}
	}

	// Re-check every line at order time. The cart may have been sitting open
	// while another pharmacy bought the last unit, or while the supplier
	// withdrew coverage — availability at add-to-cart time is not a promise.
	if err := s.revalidateCheckoutLines(ctx, input); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	seq := orderCounter.Add(1)
	orderNumber := GenerateOrderNumber(now, seq)

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
			sub, err := lineSubtotal.Sub(item.DiscountAmount)
			if err != nil {
				s.log.WarnContext(ctx, "failed to subtract discount from line subtotal", "error", err)
			} else {
				lineTotal = sub
			}
		}

		line := &OrderLine{
			OrganizationID:         item.VendorOrgID,
			ProductID:              item.ProductID,
			ProductVariantID:       item.ProductVariantID,
			ProductName:            item.ProductName,
			VariantName:            item.VariantName,
			SKU:                    item.SKU,
			OfferProductID:         item.OfferProductID,
			UnitPrice:              item.UnitPrice,
			Quantity:               item.Quantity,
			DiscountAmount:         item.DiscountAmount,
			TotalPrice:             lineTotal,
			CostPrice:              item.CostPrice,
			CostDiscountPercentage: item.CostDiscountPercentage,
			ListPrice:              item.ListPrice,
			OriginalPrice:          item.OriginalPrice,
			OriginalDiscount:       item.OriginalDiscount,
			IsNegotiated:           item.IsNegotiated,
			ProposedUnitPrice:      item.ProposedUnitPrice,
		}

		vendorMap[item.VendorOrgID] = append(vendorMap[item.VendorOrgID], line)
		var addErr error
		orderSubtotal, addErr = orderSubtotal.Add(lineTotal)
		if addErr != nil {
			return nil, apperr.Internal(addErr)
		}
	}

	// Enforce the offer's minimum order amount (063). The sanctioned minimum is
	// supplied by the caller from the approved offer; every checkout path is
	// gated here, server-side.
	if input.MinOrderAmount.IsPositive() && orderSubtotal.Minor() < input.MinOrderAmount.Minor() {
		return nil, apperr.Validation("checkout.min_order_not_met",
			"Order total is below the offer's minimum order amount.", map[string]string{
				"order_total":     orderSubtotal.String(),
				"min_order_total": input.MinOrderAmount.String(),
			})
	}

	var shipments []*OrderShipment
	var allLines []*OrderLine
	var calculatedShippingFee money.Amount

	for vendorOrgID, lines := range vendorMap {
		var shipmentSubtotal money.Amount
		for _, line := range lines {
			var addErr error
			shipmentSubtotal, addErr = shipmentSubtotal.Add(line.TotalPrice)
			if addErr != nil {
				return nil, apperr.Internal(addErr)
			}
			allLines = append(allLines, line)
		}

		shipmentShippingFee := money.Zero
		if input.VendorShippingFees != nil {
			if f, ok := input.VendorShippingFees[vendorOrgID]; ok && f.IsPositive() {
				shipmentShippingFee = f
			}
		}

		shipmentTotal := shipmentSubtotal
		if shipmentShippingFee.IsPositive() {
			shipmentTotal, _ = shipmentTotal.Add(shipmentShippingFee)
			calculatedShippingFee, _ = calculatedShippingFee.Add(shipmentShippingFee)
		}

		shipments = append(shipments, &OrderShipment{
			OrganizationID: vendorOrgID,
			BranchID:       input.VendorBranchID,
			Status:         StatusPending,
			Subtotal:       shipmentSubtotal,
			ShippingFee:    shipmentShippingFee,
			TotalAmount:    shipmentTotal,
			Lines:          lines,
		})
	}

	// Calculate overall shipping fee (combines vendor shipping fees if provided)
	shippingFee := input.ShippingFee
	if !shippingFee.IsPositive() && calculatedShippingFee.IsPositive() {
		shippingFee = calculatedShippingFee
	} else if input.VendorShippingFees != nil && calculatedShippingFee.IsPositive() {
		shippingFee = calculatedShippingFee
	}

	// Offer discounts over the lines (063): the offer's TotalDiscount reproduces
	// the invoice, FinalPrice is what the customer pays net.
	var totalDiscount money.Amount
	for _, line := range allLines {
		var addErr error
		totalDiscount, addErr = totalDiscount.Add(line.DiscountAmount)
		if addErr != nil {
			return nil, apperr.Internal(addErr)
		}
	}
	// Subtotal is already net of line discounts (legacy behaviour). TotalDiscount
	// reproduces the invoice's discount total (063); FinalPrice is what the
	// customer pays: the net total plus freight and tax.
	finalPrice := orderSubtotal
	if shippingFee.IsPositive() {
		finalPrice, _ = finalPrice.Add(shippingFee)
	}
	if input.TaxAmount.IsPositive() {
		finalPrice, _ = finalPrice.Add(input.TaxAmount)
	}

	negStatus := "none"
	if input.IsNegotiation {
		negStatus = "pending"
	}

	var custOrgID *int64
	if input.CustomerOrgID > 0 {
		custOrgID = &input.CustomerOrgID
	}

	order := &Order{
		OrderNumber:       orderNumber,
		CustomerID:        input.CustomerID,
		OrganizationID:    custOrgID,
		OfferID:           input.OfferID,
		BranchID:          input.BranchID,
		VendorBranchID:    input.VendorBranchID,
		UserAddressID:     input.UserAddressID,
		Status:            StatusPending,
		Subtotal:          orderSubtotal,
		DiscountAmount:    money.Zero,
		TotalDiscount:     totalDiscount,
		ShippingFee:       shippingFee,
		TaxAmount:         input.TaxAmount,
		TotalAmount:       finalPrice,
		FinalPrice:        finalPrice,
		PaymentMethod:     input.PaymentMethod,
		PaymentStatus:     PaymentUnpaid,
		Notes:             input.Notes,
		IsNegotiation:     input.IsNegotiation,
		NegotiationStatus: negStatus,
		NegotiationNotes:  input.NegotiationNotes,
		Shipments:         shipments,
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

// GetOrderByNumber retrieves an order by public order number.
func (s *Service) GetOrderByNumber(ctx context.Context, number string) (*Order, error) {
	return s.repo.GetOrderByNumber(ctx, number)
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
