// Package promo handles vendor discount offers, sponsored promotion packages,
// advertising campaigns, and engagement analytics.
package promo

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// DiscountType specifies how an offer discount is applied.
type DiscountType string

const (
	DiscountPercentage DiscountType = "percentage"
	DiscountFixed      DiscountType = "fixed"
)

// Offer represents a promotional discount campaign configured by a vendor.
type Offer struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	BranchID       *int64       `json:"branch_id,omitempty"` // publishing vendor branch (062)
	Title          i18n.Text    `json:"title"`
	Description    i18n.Text    `json:"description,omitempty"`
	DiscountType   DiscountType `json:"discount_type"`
	DiscountValue  money.Amount `json:"discount_value"`
	MinOrderAmount money.Amount `json:"min_order_amount"` // offer minimum order value (062)
	AdminStatus    string       `json:"admin_status"`     // pending | approved | rejected (062)
	AdminNotes     string       `json:"admin_notes,omitempty"`
	ApprovedAt     *time.Time   `json:"approved_at,omitempty"`
	ApprovedBy     int64        `json:"approved_by,omitempty"`
	RejectedAt     *time.Time   `json:"rejected_at,omitempty"`
	RejectedBy     int64        `json:"rejected_by,omitempty"`
	StartsAt       time.Time    `json:"starts_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	IsActive       bool         `json:"is_active"`
	ViewsCount     int64        `json:"views_count"`
	ClicksCount    int64        `json:"clicks_count"`
	ProductIDs     []int64      `json:"product_ids,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty"`
}

// OfferProductWithOffer carries an offer line together with the offer it
// belongs to — the shape storefront lookups return.
type OfferProductWithOffer struct {
	Product *OfferProduct
	Offer   *Offer
}

// VisibleOffer is an offer the pharmacy branch can actually buy: the vendor
// branch's weekly coverage circle contains the pharmacy branch's coordinates,
// on the requested day.
type VisibleOffer struct {
	Offer          *Offer
	VendorBranchID int64
	Metres         int
}

// IsApproved reports whether the platform cleared the offer for commerce.
func (o *Offer) IsApproved() bool { return o.AdminStatus == "approved" }

// OfferProduct is one line of an offer: the product (or variant) being sold
// and the custom pricing the vendor set for it (062).
type OfferProduct struct {
	ID                    int64         `json:"id"`
	OfferID               int64         `json:"offer_id"`
	ProductID             int64         `json:"product_id"`
	VariantID             *int64        `json:"variant_id,omitempty"`
	CustomPrice           *money.Amount `json:"custom_price,omitempty"`           // full override of the list price
	CustomDiscountPercent *float64      `json:"custom_discount_percentage,omitempty"` // percent, e.g. 15.00 = 15%
	CustomDiscountAmount  *money.Amount `json:"custom_discount_amount,omitempty"`
	CustomQty             int           `json:"custom_qty"`
	MaxQtyPerOrder        *int          `json:"max_qty_per_order,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
}

// OfferPackage represents a tier for sponsoring and promoting offers.
type OfferPackage struct {
	ID           int64        `json:"id"`
	PublicID     string       `json:"public_id"`
	Name         i18n.Text    `json:"name"`
	Price        money.Amount `json:"price"`
	DurationDays int          `json:"duration_days"`
	MaxOffers    int          `json:"max_offers"`
	IsActive     bool         `json:"is_active"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// OfferSponsorship represents an active sponsored campaign for an offer.
type OfferSponsorship struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	OfferID        int64     `json:"offer_id"`
	PackageID      int64     `json:"package_id"`
	StartsAt       time.Time `json:"starts_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// Ad represents a display banner advertisement.
type Ad struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID *int64    `json:"organization_id,omitempty"`
	Title          string    `json:"title"`
	ImageURL       string    `json:"image_url"`
	TargetURL      string    `json:"target_url"`
	Position       string    `json:"position"`
	IsActive       bool      `json:"is_active"`
	StartsAt       time.Time `json:"starts_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Impressions    int64     `json:"impressions"`
	Clicks         int64     `json:"clicks"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CalculateDiscount computes the discount amount for a given order subtotal.
func (o *Offer) CalculateDiscount(subtotal money.Amount) (money.Amount, error) {
	if !o.IsActive || o.DiscountValue.IsZero() {
		return money.Zero, nil
	}
	if o.MinOrderAmount.IsPositive() && subtotal.Minor() < o.MinOrderAmount.Minor() {
		return money.Zero, nil
	}

	if o.DiscountType == DiscountPercentage {
		// Value stored in minor units as basis points or percent: e.g. 15.00 -> 1500 bps
		bps := o.DiscountValue.Minor()
		discount := subtotal.ApplyPercent(bps)
		return discount, nil
	}

	// Fixed discount
	if o.DiscountValue.Minor() > subtotal.Minor() {
		return subtotal, nil
	}
	return o.DiscountValue, nil
}

// SpecialOffer represents the organization's special offer with product & geographic coverage rules.
type SpecialOffer struct {
	ID                 int64                   `json:"id"`
	PublicID           string                  `json:"public_id"`
	OrganizationID     int64                   `json:"organization_id"`
	BranchID           *int64                  `json:"branch_id,omitempty"`
	BranchName         string                  `json:"branch_name,omitempty"`
	Title              i18n.Text               `json:"title"`
	Description        i18n.Text               `json:"description,omitempty"`
	DiscountPercentage float64                 `json:"discount_percentage"`
	DiscountAmount     money.Amount            `json:"discount_amount"`
	MinOrderAmount     money.Amount            `json:"min_order_amount"`
	TotalPrice         money.Amount            `json:"total_price"`
	StartDate          *time.Time              `json:"start_date,omitempty"`
	EndDate            *time.Time              `json:"end_date,omitempty"`
	Status             string                  `json:"status"`       // active, inactive, expired, draft
	AdminStatus        string                  `json:"admin_status"` // pending, approved, rejected
	Image              string                  `json:"image,omitempty"`
	Products           []*SpecialOfferProduct  `json:"products,omitempty"`
	Locations          []*SpecialOfferLocation `json:"locations,omitempty"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

// SpecialOfferProduct represents a specific product/variant in a special offer.
type SpecialOfferProduct struct {
	ID                 int64        `json:"id"`
	OfferID            int64        `json:"offer_id"`
	VariantID          int64        `json:"variant_id"`
	VariantName        string       `json:"variant_name,omitempty"`
	OriginalPrice      money.Amount `json:"original_price"`
	CustomPrice        money.Amount `json:"custom_price"`
	DiscountPercentage float64      `json:"discount_percentage"`
	DiscountAmount     money.Amount `json:"discount_amount"`
	Quantity           int          `json:"quantity"`
	CreatedAt          time.Time    `json:"created_at"`
}

// SpecialOfferLocation represents geographic coverage rules for a special offer.
type SpecialOfferLocation struct {
	ID          int64     `json:"id"`
	OfferID     int64     `json:"offer_id"`
	CityID      *int64    `json:"city_id,omitempty"`
	CityName    string    `json:"city_name,omitempty"`
	AddressAr   string    `json:"address_ar"`
	AddressEn   string    `json:"address_en"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Radius      int       `json:"radius"`       // in meters
	DayOfWeek   int       `json:"day_of_week"`  // 1=Saturday ... 7=Friday
	TimeFrom    string    `json:"time_from,omitempty"`
	TimeTo      string    `json:"time_to,omitempty"`
	Status      string    `json:"status"`       // active, inactive
	AdminStatus string    `json:"admin_status"` // pending, approved, rejected
	CreatedAt   time.Time `json:"created_at"`
}


// Validate ensures dates and discount amounts are sound.
func (o *Offer) Validate() error {
	if o.OrganizationID <= 0 {
		return apperr.Validation("offer.org_required", "Organization ID is required.", nil)
	}
	if o.Title.IsEmpty() {
		return apperr.Validation("offer.title_required", "Offer title is required.", nil)
	}
	if o.ExpiresAt.Before(o.StartsAt) {
		return apperr.Validation("offer.dates_invalid", "Expiration date must be after start date.", nil)
	}
	return nil
}

// HighlightSection represents a curated section on the homepage or storefront
// (066: one table serves both the platform and organization-owned rows).
type HighlightSection struct {
	ID             int64                  `json:"id"`
	PublicID       string                 `json:"public_id"`
	OwnerType      string                 `json:"owner_type"`       // platform | organization (066)
	OrganizationID *int64                 `json:"organization_id,omitempty"` // owner when organization (066)
	Title          i18n.Text              `json:"title"`
	Slug           string                 `json:"slug"`
	DisplayOrder   int                    `json:"display_order"`
	IsActive       bool                   `json:"is_active"`
	Items          []HighlightSectionItem `json:"items,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// HighlightSectionItem represents an item within a highlight section.
type HighlightSectionItem struct {
	ID           int64  `json:"id"`
	SectionID    int64  `json:"section_id"`
	ProductID    *int64 `json:"product_id,omitempty"`
	OfferID      *int64 `json:"offer_id,omitempty"`
	DisplayOrder int    `json:"display_order"`
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

