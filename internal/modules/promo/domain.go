// Package promo handles vendor discount offers, sponsored promotion packages,
// advertising campaigns, and engagement analytics.
package promo

import (
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
	Title          i18n.Text    `json:"title"`
	Description    i18n.Text    `json:"description,omitempty"`
	DiscountType   DiscountType `json:"discount_type"`
	DiscountValue  money.Amount `json:"discount_value"`
	MinOrderValue  money.Amount `json:"min_order_value"`
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
	if o.MinOrderValue.IsPositive() && subtotal.Minor() < o.MinOrderValue.Minor() {
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

// HighlightSection represents a curated section on the homepage or storefront.
type HighlightSection struct {
	ID           int64                  `json:"id"`
	PublicID     string                 `json:"public_id"`
	Title        i18n.Text              `json:"title"`
	Slug         string                 `json:"slug"`
	DisplayOrder int                    `json:"display_order"`
	IsActive     bool                   `json:"is_active"`
	Items        []HighlightSectionItem `json:"items,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// HighlightSectionItem represents an item within a highlight section.
type HighlightSectionItem struct {
	ID           int64  `json:"id"`
	SectionID    int64  `json:"section_id"`
	ProductID    *int64 `json:"product_id,omitempty"`
	OfferID      *int64 `json:"offer_id,omitempty"`
	DisplayOrder int    `json:"display_order"`
}
