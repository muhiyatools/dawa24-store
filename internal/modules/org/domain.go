// Package org manages organization tenants, vendor onboarding, branches,
// organizational membership, vendor reviews, and policies.
package org

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrganizationType defines the classification of a tenant.
type OrganizationType string

const (
	TypeSupplier      OrganizationType = "supplier"
	TypePharmacy      OrganizationType = "pharmacy"
	TypeChainPharmacy OrganizationType = "chain_pharmacy"
)

// OrganizationStatus defines the approval state of an organization.
type OrganizationStatus string

const (
	StatusPending   OrganizationStatus = "pending"
	StatusApproved  OrganizationStatus = "approved"
	StatusSuspended OrganizationStatus = "suspended"
	StatusRejected  OrganizationStatus = "rejected"
)

// Organization represents a company or pharmacy organization tenant.
type Organization struct {
	ID                 int64              `json:"id"`
	PublicID           string             `json:"public_id"`
	LegalName          string             `json:"legal_name"`
	TradeName          i18n.Text          `json:"trade_name"`
	TaxNumber          string             `json:"tax_number"`
	CommercialRegister string             `json:"commercial_register"`
	Type               OrganizationType   `json:"type"`
	Status             OrganizationStatus `json:"status"`
	CreditLimit        money.Amount       `json:"credit_limit"`
	PaymentTermsDays   int                `json:"payment_terms_days"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// Branch represents a physical location or warehouse of an organization.
type Branch struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	Name           i18n.Text `json:"name"`
	Code           string    `json:"code"`
	Address        string    `json:"address"`
	CityID         *int64    `json:"city_id,omitempty"`
	IsMain         bool      `json:"is_main"`
	Phone          string    `json:"phone,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Member represents a user's membership and role in an organization.
type Member struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	RoleID         int64     `json:"role_id"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Review represents a customer rating and review for a vendor organization.
type Review struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	Rating         int       `json:"rating"`
	ReviewText     string    `json:"review_text,omitempty"`
	IsApproved     bool      `json:"is_approved"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Follower represents a customer following an organization for updates.
type Follower struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// SocialMedia represents an organization's social media profile link.
type SocialMedia struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	Platform       string    `json:"platform"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Policy represents organizational terms, delivery rules, or refund policies.
type Policy struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	PolicyType     string    `json:"policy_type"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate checks organization registration details.
func (o *Organization) Validate() error {
	if o.LegalName == "" {
		return apperr.Validation("org.legal_name_required", "Legal name is required.", nil)
	}
	if o.CommercialRegister == "" {
		return apperr.Validation("org.cr_required", "Commercial registration number is required.", nil)
	}
	if o.Type == "" {
		o.Type = TypePharmacy
	}
	if o.Status == "" {
		o.Status = StatusPending
	}
	return nil
}

// ValidateBranch ensures branch data is valid.
func (b *Branch) Validate() error {
	if b.OrganizationID <= 0 {
		return apperr.Validation("branch.org_required", "Organization ID is required.", nil)
	}
	if b.Code == "" {
		return apperr.Validation("branch.code_required", "Branch code is required.", nil)
	}
	return nil
}

// HighlightSection is a supplier-curated merchandising row on its storefront
// (legacy organization_highlight_sections).
type HighlightSection struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	Title          i18n.Text `json:"title"`
	Slug           string    `json:"slug,omitempty"`
	DisplayOrder   int       `json:"display_order"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// HighlightSectionItem is one product or offer inside a highlight section.
type HighlightSectionItem struct {
	ID           int64  `json:"id"`
	SectionID    int64  `json:"section_id"`
	ProductID    *int64 `json:"product_id,omitempty"`
	OfferID      *int64 `json:"offer_id,omitempty"`
	DisplayOrder int    `json:"display_order"`
}
