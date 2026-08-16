// Package catalog manages products, variants, categories, and brands for the
// pharmaceutical marketplace.
package catalog

import (
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
	CreatedAt              time.Time     `json:"created_at"`
	UpdatedAt              time.Time     `json:"updated_at"`
	DeletedAt              *time.Time    `json:"deleted_at,omitempty"`
}

// ProductVariant represents a distinct package, concentration, or SKU variation (was product_childerns).
type ProductVariant struct {
	ID             int64         `json:"id"`
	PublicID       string        `json:"public_id"`
	OrganizationID int64         `json:"organization_id"`
	ProductID      int64         `json:"product_id"`
	Name           i18n.Text     `json:"name"`
	SKU            string        `json:"sku,omitempty"`
	Barcode        string        `json:"barcode,omitempty"`
	Price          money.Amount  `json:"price"`
	CostPrice      money.Amount  `json:"cost_price"`
	Discount       money.Amount  `json:"discount"`
	Unit           string        `json:"unit,omitempty"`
	Image          string        `json:"image,omitempty"`
	Status         ProductStatus `json:"status"`
	IsFeatured     bool          `json:"is_featured"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	DeletedAt      *time.Time    `json:"deleted_at,omitempty"`
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

// Validate ensures required product attributes are sound before persisting.
func (p *Product) Validate() error {
	if p.OrganizationID <= 0 {
		return apperr.Validation("product.org_required", "Organization ID is required.", nil)
	}
	if p.Name.IsEmpty() {
		return apperr.Validation("product.name_required", "Product name in Arabic or English is required.", nil)
	}
	if p.Price.IsNegative() {
		return apperr.Validation("product.price_negative", "Product price cannot be negative.", nil)
	}
	return nil
}

// CustomerProductMapping defines customer-specific custom pricing and discount terms.
type CustomerProductMapping struct {
	ID               int64        `json:"id"`
	OrganizationID   int64        `json:"organization_id"`
	CustomerOrgID    int64        `json:"customer_org_id"`
	ProductID        int64        `json:"product_id"`
	ProductVariantID *int64       `json:"product_variant_id,omitempty"`
	CustomPrice      money.Amount `json:"custom_price"`
	DiscountBps      *int         `json:"discount_bps,omitempty"`
	IsActive         bool         `json:"is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
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

// SavingProduct represents promotional bundled deals.
type SavingProduct struct {
	ID             int64        `json:"id"`
	OrganizationID int64        `json:"organization_id"`
	ProductID      int64        `json:"product_id"`
	BundleQuantity int          `json:"bundle_quantity"`
	BundleDiscount money.Amount `json:"bundle_discount"`
	IsActive       bool         `json:"is_active"`
	CreatedAt      time.Time    `json:"created_at"`
}
