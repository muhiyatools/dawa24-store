// Package catalog manages products, variants, categories, and brands for the
// pharmaceutical marketplace.
package catalog

import (
	"context"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ProductStatus defines lifecycle states for products and variants.
type ProductStatus string

const (
	StatusActive   ProductStatus = "active"
	StatusInactive ProductStatus = "inactive"
	StatusPending  ProductStatus = "pending"
	StatusRejected ProductStatus = "rejected"
)

// Category represents a hierarchical product category.
type Category struct {
	ID          int64      `json:"id"`
	PublicID    string     `json:"public_id"`
	ParentID    *int64     `json:"parent_id,omitempty"`
	Name        i18n.Text  `json:"name"`
	Description i18n.Text  `json:"description,omitempty"`
	Icon        string     `json:"icon,omitempty"`
	Image       string     `json:"image,omitempty"`
	Status      string     `json:"status"`
	SortOrder   int        `json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// Brand represents a pharmaceutical brand or manufacturer.
type Brand struct {
	ID          int64      `json:"id"`
	PublicID    string     `json:"public_id"`
	Name        i18n.Text  `json:"name"`
	Description i18n.Text  `json:"description,omitempty"`
	Image       string     `json:"image,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// Product represents a tenant-owned master catalogue product.
type Product struct {
	ID                     int64         `json:"id"`
	PublicID               string        `json:"public_id"`
	OrganizationID         int64         `json:"organization_id"`
	CategoryID             *int64        `json:"category_id,omitempty"`
	BrandID                *int64        `json:"brand_id,omitempty"`
	BranchID               *int64        `json:"branch_id,omitempty"`
	Name                   i18n.Text     `json:"name"`
	Description            i18n.Text     `json:"description,omitempty"`
	SKU                    string        `json:"sku,omitempty"`
	Barcode                string        `json:"barcode,omitempty"`
	Price                  money.Amount  `json:"price"`
	Discount               money.Amount  `json:"discount"`
	OldPrice               money.Amount  `json:"old_price"`
	Image                  string        `json:"image,omitempty"`
	ImageLink              string        `json:"image_link,omitempty"`
	Status                 ProductStatus `json:"status"`
	SoldTimes              int64         `json:"sold_times"`
	IsFeatured             bool          `json:"is_featured"`
	DosageForm             string        `json:"dosage_form,omitempty"`
	ScientificName         string        `json:"scientific_name,omitempty"`
	Pharmacology           string        `json:"pharmacology,omitempty"`
	Active                 string        `json:"active,omitempty"`
	Concentration          string        `json:"concentration,omitempty"`
	Unit                   string        `json:"unit,omitempty"`
	ManufacturingCompanies string        `json:"manufacturing_companies,omitempty"`
	InstitutionalWorkIDs   []int64       `json:"institutional_work_ids,omitempty"`
	CreatedAt              time.Time     `json:"created_at"`
	UpdatedAt              time.Time     `json:"updated_at"`
	DeletedAt              *time.Time    `json:"deleted_at,omitempty"`
}

// ProductVariant represents a distinct package, concentration, or SKU variation (was product_childerns).
type ProductVariant struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	ProductID      int64        `json:"product_id"`
	Name           i18n.Text    `json:"name"`
	SKU            string       `json:"sku,omitempty"`
	Barcode        string       `json:"barcode,omitempty"`
	Price          money.Amount `json:"price"`
	CostPrice      money.Amount `json:"cost_price"`
	Discount       money.Amount `json:"discount"`
	Unit           string       `json:"unit,omitempty"`
	Image          string       `json:"image,omitempty"`
	BatchNumber    string       `json:"batch_number,omitempty"`
	ExpiryDate     *time.Time   `json:"expiry_date,omitempty"`
	MinOrderQty    int          `json:"min_order_qty"`
	BranchID       *int64       `json:"branch_id,omitempty"`
	// StockQty is NOT persisted on this table. catalog.product_variants has no
	// stock column — stock lives in inventory.stocks against a warehouse. This
	// field is a write-side input (a supplier's opening quantity) and a
	// read-side projection; anything that reads it without populating it from
	// inventory is reading a permanent zero. That is why the old cart's stock
	// guard, `if variant.StockQty > 0 && ...`, never fired.
	StockQty             int           `json:"stock_qty"`
	Status               ProductStatus `json:"status"`
	IsFeatured           bool          `json:"is_featured"`
	InstitutionalWorkIDs []int64       `json:"institutional_work_ids,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	DeletedAt            *time.Time    `json:"deleted_at,omitempty"`
}

// EffectivePrice calculates the final customer price after discount.
func (p *Product) EffectivePrice() money.Amount {
	if p.Discount.IsPositive() && p.Discount.Minor() < p.Price.Minor() {
		eff, err := p.Price.Sub(p.Discount)
		if err == nil {
			return eff
		}
	}
	return p.Price
}

// PublicPrice returns the official retail public price (سعر الجمهور).
func (p *Product) PublicPrice() money.Amount {
	if p.OldPrice.IsPositive() {
		return p.OldPrice
	}
	return p.Price
}

// NetPrice returns the net pharmacy wholesale price (سعر الصيدلية بعد الخصم).
func (p *Product) NetPrice() money.Amount {
	return p.EffectivePrice()
}

// HasDiscount reports whether the product has an active discount.
func (p *Product) HasDiscount() bool {
	if p.Discount.IsPositive() {
		return true
	}
	if p.OldPrice.IsPositive() && p.OldPrice.Minor() > p.Price.Minor() {
		return true
	}
	return false
}

// DiscountPercent calculates the discount percentage relative to public price.
func (p *Product) DiscountPercent() int {
	pub := p.PublicPrice()
	if !pub.IsPositive() {
		return 0
	}
	net := p.NetPrice()
	diff := pub.Minor() - net.Minor()
	if diff <= 0 {
		return 0
	}
	return int((diff * 100) / pub.Minor())
}

// Validate ensures required product attributes are sound before persisting.
func (p *Product) Validate() error {
	if p.Name.IsEmpty() {
		return apperr.Validation("product.name_required", "اسم الصنف الدوائي بالعربية أو الإنجليزية مطلوب.", nil)
	}
	if p.Price.IsNegative() {
		return apperr.Validation("product.price_negative", "سعر الصنف لا يمكن أن يكون سالباً.", nil)
	}
	return nil
}

// CustomerProductMapping defines a per-customer pricing/discount row and, for
// import rows (071), the customer's own name for the product. Vendor-set rows
// carry customer_org_id; import rows carry NULL and a raw_name.
type CustomerProductMapping struct {
	ID               int64         `json:"id"`
	OrganizationID   int64         `json:"organization_id"`
	CustomerOrgID    *int64        `json:"customer_org_id,omitempty"`
	ProductID        int64         `json:"product_id"`
	ProductVariantID *int64        `json:"product_variant_id,omitempty"`
	RawName          string        `json:"raw_name"`
	BranchID         *int64        `json:"branch_id,omitempty"`
	Price            money.Amount  `json:"price"`
	Discount         *money.Amount `json:"discount,omitempty"` // percent, 2dp
	Source           string        `json:"source"`             // excel | csv | link | manual
	Status           string        `json:"status"`             // pending | processed | rejected
	IsActive         bool          `json:"is_active"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// SavingProduct represents a target-saving or tracked price product for a pharmacy or vendor.
type SavingProduct struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	UserID         *int64       `json:"user_id,omitempty"`
	OrganizationID int64        `json:"organization_id"`
	ProductID      *int64       `json:"product_id,omitempty"`
	NameProduct    string       `json:"name_product"`
	SKU            string       `json:"sku,omitempty"`
	Quantity       float64      `json:"quantity"`
	Price          money.Amount `json:"price"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty"`
}

// ProductAlert represents user notification triggers for price drops and restocks.
type ProductAlert struct {
	ID          int64        `json:"id"`
	PublicID    string       `json:"public_id"`
	UserID      int64        `json:"user_id"`
	ProductID   int64        `json:"product_id"`
	AlertType   string       `json:"alert_type"`
	TargetPrice money.Amount `json:"target_price"`
	IsTriggered bool         `json:"is_triggered"`
	TriggeredAt *time.Time   `json:"triggered_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

// FinderQuestion is one step of the guided product-finder questionnaire.
type FinderQuestion struct {
	ID        int64     `json:"id"`
	Question  i18n.Text `json:"question"`
	Type      string    `json:"type"` // choice, text, number
	IsFirst   bool      `json:"is_first"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FinderOption is an answer choice leading to the next question or a result.
type FinderOption struct {
	ID             int64     `json:"id"`
	QuestionID     int64     `json:"question_id"`
	Label          i18n.Text `json:"label"`
	NextQuestionID *int64    `json:"next_question_id,omitempty"`
	ResultID       *int64    `json:"result_id,omitempty"`
	SortOrder      int       `json:"sort_order"`
}

// FinderResult is the terminal recommendation of the questionnaire.
type FinderResult struct {
	ID          int64     `json:"id"`
	Title       i18n.Text `json:"title"`
	Description i18n.Text `json:"description"`
}

// InstitutionalGate resolves which institutional work ids a user may see products for.
// Implemented by the org module and injected at composition time in cmd/server/routes.go — modules must not import each other (ADR 0002).
type InstitutionalGate interface {
	AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error)
}

// InstitutionalGateFunc adapts a standard function to InstitutionalGate.
type InstitutionalGateFunc func(ctx context.Context, userID int64, mode int) ([]int64, error)

func (f InstitutionalGateFunc) AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error) {
	return f(ctx, userID, mode)
}

// ProductIndexItem represents a denormalized read-model record in catalog.product_index.
// Matches Laravel product_infos / product_search_index 25-column specification.
type ProductIndexItem struct {
	UniqueRowID          string       `json:"unique_row_id"`
	ProductID            int64        `json:"product_id"`
	VariantID            *int64       `json:"variant_id,omitempty"`
	SKU                  string       `json:"sku,omitempty"`
	NameAR               string       `json:"name_ar,omitempty"`
	NameEN               string       `json:"name_en,omitempty"`
	SearchText           string       `json:"search_text,omitempty"`
	SearchAR             string       `json:"search_ar,omitempty"`
	SearchEN             string       `json:"search_en,omitempty"`
	SearchSimple         string       `json:"search_simple,omitempty"`
	OrganizationName     string       `json:"organization_name,omitempty"`
	BranchCity           string       `json:"branch_city,omitempty"`
	ScientificName       string       `json:"scientific_name,omitempty"`
	Price                money.Amount `json:"price"`
	Discount             money.Amount `json:"discount"`
	StockQuantity        int          `json:"stock_quantity"`
	CategoryID           *int64       `json:"category_id,omitempty"`
	BrandID              *int64       `json:"brand_id,omitempty"`
	HasDiscount          bool         `json:"has_discount"`
	DiscountPercentage   float64      `json:"discount_percentage"`
	PriceAfterDiscount   money.Amount `json:"price_after_discount"`
	OrganizationID       int64        `json:"organization_id"`
	BranchID             *int64       `json:"branch_id,omitempty"`
	Status               string       `json:"status"`
	ProductType          string       `json:"product_type"` // parent, child
	InstitutionalWorkIDs []int64      `json:"institutional_work_ids"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// ComposeUniqueRowID builds the canonical deterministic key for a read-model item.
func ComposeUniqueRowID(productID int64, variantID, branchID *int64) string {
	if variantID != nil && *variantID > 0 {
		if branchID != nil && *branchID > 0 {
			return "p_" + strconv.FormatInt(productID, 10) + "_v_" + strconv.FormatInt(*variantID, 10) + "_b_" + strconv.FormatInt(*branchID, 10)
		}
		return "p_" + strconv.FormatInt(productID, 10) + "_v_" + strconv.FormatInt(*variantID, 10)
	}
	if branchID != nil && *branchID > 0 {
		return "p_" + strconv.FormatInt(productID, 10) + "_b_" + strconv.FormatInt(*branchID, 10)
	}
	return "p_" + strconv.FormatInt(productID, 10)
}
