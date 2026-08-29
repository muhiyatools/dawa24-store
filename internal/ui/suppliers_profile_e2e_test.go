package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// mockCommerceRepoForSupplierCartTest implements commerce.Repository for cart tests.
type mockCommerceRepoForSupplierCartTest struct {
	carts     map[int64]*commerce.Cart
	cartItems map[int64][]*commerce.CartItem
	nextID    int64
}

func newMockCommerceRepo() *mockCommerceRepoForSupplierCartTest {
	return &mockCommerceRepoForSupplierCartTest{
		carts:     make(map[int64]*commerce.Cart),
		cartItems: make(map[int64][]*commerce.CartItem),
		nextID:    1,
	}
}

func (m *mockCommerceRepoForSupplierCartTest) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountOrders(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error) {
	return 0, nil
}
func (m *mockCommerceRepoForSupplierCartTest) MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *mockCommerceRepoForSupplierCartTest) MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateOrderStatus(ctx context.Context, id int64, status commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) CreateQuoteRequest(ctx context.Context, q *commerce.QuoteRequest) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, price money.Amount, notes string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, notes string, responderID *int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	for _, c := range m.carts {
		if c.UserID == userID {
			return c, nil
		}
	}
	c := &commerce.Cart{
		ID:        m.nextID,
		PublicID:  fmt.Sprintf("cart_%d", m.nextID),
		UserID:    userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.nextID++
	m.carts[c.ID] = c
	return c, nil
}
func (m *mockCommerceRepoForSupplierCartTest) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	c, ok := m.carts[cartID]
	if !ok {
		return nil, nil
	}
	items := m.cartItems[cartID]
	res := *c
	res.Items = items
	return &res, nil
}
func (m *mockCommerceRepoForSupplierCartTest) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	existing := m.cartItems[cartID]
	for _, it := range existing {
		if it.ProductVariantID == item.ProductVariantID {
			it.Quantity += item.Quantity
			it.UnitPrice = item.UnitPrice
			return nil
		}
	}
	item.CartID = cartID
	item.ID = int64(len(existing) + 1)
	m.cartItems[cartID] = append(m.cartItems[cartID], item)
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	for _, it := range m.cartItems[cartID] {
		if it.ProductVariantID == variantID {
			it.Quantity = quantity
			return nil
		}
	}
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	var kept []*commerce.CartItem
	for _, it := range m.cartItems[cartID] {
		if it.ProductVariantID != variantID {
			kept = append(kept, it)
		}
	}
	m.cartItems[cartID] = kept
	return nil
}
func (m *mockCommerceRepoForSupplierCartTest) ClearCart(ctx context.Context, cartID int64) error {
	delete(m.cartItems, cartID)
	return nil
}

type mockAvailabilityProbe struct {
	availQty int
}

func (m *mockAvailabilityProbe) Variant(ctx context.Context, variantID int64) (commerce.VariantAvailability, error) {
	return commerce.VariantAvailability{
		ID:             variantID,
		OrganizationID: 10,
		StockQty:       m.availQty,
		MinOrderQty:    1,
		Active:         true,
	}, nil
}
func (m *mockAvailabilityProbe) Vendor(ctx context.Context, orgID int64) (commerce.VendorAvailability, error) {
	return commerce.VendorAvailability{
		ID:       orgID,
		IsVendor: true,
		Approved: true,
	}, nil
}
func (m *mockAvailabilityProbe) CustomerBranch(ctx context.Context, branchID int64) (commerce.BranchAvailability, error) {
	lat := 30.0
	lon := 31.0
	return commerce.BranchAvailability{
		ID:             branchID,
		OrganizationID: 99,
		Latitude:       &lat,
		Longitude:      &lon,
	}, nil
}
func (m *mockAvailabilityProbe) VendorCovers(ctx context.Context, vendorOrgID int64, lat, lon float64, day time.Weekday) (bool, error) {
	return true, nil
}

// TestSupplierProfileData_AvailabilityAndStock verifies view model helpers.
func TestSupplierProfileData_AvailabilityAndStock(t *testing.T) {
	v1 := &catalog.ProductVariant{
		ID:          101,
		ProductID:   1,
		StockQty:    50,
		MinOrderQty: 5,
		Price:       money.FromMajor(120),
	}
	v2 := &catalog.ProductVariant{
		ID:          102,
		ProductID:   2,
		StockQty:    0,
		MinOrderQty: 1,
		Price:       money.FromMajor(80),
	}

	data := &pages.SupplierProfileData{
		Org: &org.Organization{
			ID:        10,
			LegalName: "United Pharma Co",
			TradeName: i18n.Text{"ar": "شركة المتحدون للأدوية", "en": "United Pharma"},
		},
		Variants: []*catalog.ProductVariant{v1, v2},
		VariantMeta: map[int64]pages.SupplierVariantMeta{
			101: {
				AvailableStock: 50,
				MinOrderQty:    5,
				IsCovered:      true,
				CanAddToCart:   true,
			},
			102: {
				AvailableStock: 0,
				MinOrderQty:    1,
				IsCovered:      false,
				CoverageReason: "خارج نطاق التغطية",
				CanAddToCart:   false,
			},
		},
	}

	// Verify v1 (in stock, covered)
	if stock := data.GetAvailableStock(v1); stock != 50 {
		t.Errorf("expected available stock 50, got %d", stock)
	}
	if minQty := data.GetMinOrderQty(v1); minQty != 5 {
		t.Errorf("expected min order qty 5, got %d", minQty)
	}
	if !data.IsVariantCovered(v1) {
		t.Errorf("expected v1 to be covered")
	}
	if !data.CanAddToCart(v1) {
		t.Errorf("expected v1 to be purchasable")
	}

	// Verify v2 (0 stock, not covered)
	if stock := data.GetAvailableStock(v2); stock != 0 {
		t.Errorf("expected available stock 0, got %d", stock)
	}
	if data.IsVariantCovered(v2) {
		t.Errorf("expected v2 not to be covered")
	}
	if data.GetCoverageReason(v2) != "خارج نطاق التغطية" {
		t.Errorf("unexpected coverage reason: %s", data.GetCoverageReason(v2))
	}
	if data.CanAddToCart(v2) {
		t.Errorf("expected v2 NOT to be purchasable")
	}
}

// TestSupplierProfile_RenderingE2E verifies that SupplierProfile template renders stock badge and add to cart form.
func TestSupplierProfile_RenderingE2E(t *testing.T) {
	exp := time.Now().Add(365 * 24 * time.Hour)
	v := &catalog.ProductVariant{
		ID:          501,
		ProductID:   200,
		Name:        i18n.Text{"ar": "كونجستال 20 قرص", "en": "Congestal 20 Tabs"},
		Price:       money.FromMajor(35),
		StockQty:    75,
		MinOrderQty: 2,
		BatchNumber: "BCH-2026-99",
		ExpiryDate:  &exp,
		Unit:        "علبة",
	}

	p := &catalog.Product{
		ID:             200,
		Name:           i18n.Text{"ar": "كونجستال أقراص", "en": "Congestal Tabs"},
		Price:          money.FromMajor(45),
		DosageForm:     "أقراص",
		ScientificName: "Paracetamol 500mg",
	}

	data := pages.SupplierProfileData{
		Org: &org.Organization{
			ID:        10,
			LegalName: "Al-Ahram Pharma Distribution",
			TradeName: i18n.Text{"ar": "مؤسسة الأهرام للتوزيع الدوائي"},
			Status:    org.StatusApproved,
		},
		Variants:    []*catalog.ProductVariant{v},
		ProductsMap: map[int64]*catalog.Product{200: p},
		VariantMeta: map[int64]pages.SupplierVariantMeta{
			501: {
				AvailableStock: 75,
				MinOrderQty:    2,
				IsCovered:      true,
				CanAddToCart:   true,
			},
		},
		CurrentPage: 1,
		TotalPages:  1,
	}

	var buf bytes.Buffer
	err := pages.SupplierProfile("ar", "rtl", data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render SupplierProfile: %v", err)
	}

	html := buf.String()

	// Verify product name and details
	if !strings.Contains(html, "كونجستال 20 قرص") {
		t.Errorf("rendered HTML missing product variant name")
	}
	if !strings.Contains(html, "BCH-2026-99") {
		t.Errorf("rendered HTML missing batch number")
	}

	// Verify in-stock badge
	if !strings.Contains(html, "متوفر (75 عبوة)") {
		t.Errorf("rendered HTML missing available stock badge (75 عبوة)")
	}

	// Verify Add to Cart form attributes: action, variant_id, min, max
	if !strings.Contains(html, `action="/cart/add"`) {
		t.Errorf("rendered HTML missing cart add form action")
	}
	if !strings.Contains(html, `value="501"`) {
		t.Errorf("rendered HTML missing variant ID input")
	}
	if !strings.Contains(html, `max="75"`) {
		t.Errorf("rendered HTML missing max stock limit constraint on quantity input")
	}
	if !strings.Contains(html, `+ إضافة للسلة`) {
		t.Errorf("rendered HTML missing + إضافة للسلة button")
	}
}

// TestAddToCartSubmit_HTMX_And_Persistence tests the AddToCartSubmit handler with HTMX.
func TestAddToCartSubmit_HTMX_And_Persistence(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	commRepo := newMockCommerceRepo()
	commSvc := commerce.NewService(commRepo, log)

	commSvc.SetAvailabilityProbe(&mockAvailabilityProbe{availQty: 100})

	handler := ui.NewUIHandler(
		nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log,
	)

	branchID := int64(123)
	// Create customer actor context
	customerActor := authctx.Actor{
		UserID:         42,
		Email:          "pharmacy@example.com",
		Role:           "customer",
		OrgType:        "customer",
		OrganizationID: 99,
		BranchID:       &branchID,
	}
	ctx := authctx.WithActor(context.Background(), customerActor)

	// Prepare HTMX POST request
	form := url.Values{}
	form.Set("variant_id", "501")
	form.Set("product_id", "200")
	form.Set("vendor_org_id", "10")
	form.Set("quantity", "4")
	form.Set("return_to", "/suppliers/10")

	req := httptest.NewRequest(http.MethodPost, "/cart/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.AddToCartSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK for HTMX add to cart, got %d", res.StatusCode)
	}

	// Verify HX-Trigger header contains showToast and cartUpdated
	hxTrigger := res.Header.Get("HX-Trigger")
	if !strings.Contains(hxTrigger, "showToast") || !strings.Contains(hxTrigger, "cartUpdated") {
		t.Errorf("expected HX-Trigger to contain showToast and cartUpdated, got: %s", hxTrigger)
	}

	// Verify item was saved to database cart
	cart, err := commSvc.GetCart(ctx, 42)
	if err != nil {
		t.Fatalf("failed to retrieve cart: %v", err)
	}
	if cart == nil || len(cart.Items) == 0 {
		t.Fatalf("cart is empty in database, item was not persisted")
	}
	if cart.Items[0].ProductVariantID != 501 || cart.Items[0].Quantity != 4 {
		t.Errorf("expected variant 501 with qty 4, got variant %d qty %d", cart.Items[0].ProductVariantID, cart.Items[0].Quantity)
	}
}
