package org

import (
	"time"
)

// EmployeeInstitutionalWork links a user to an institutional work group.
type EmployeeInstitutionalWork struct {
	ID                  int64     `json:"id"`
	OrganizationID      int64     `json:"organization_id"`
	UserID              int64     `json:"user_id"`
	InstitutionalWorkID int64     `json:"institutional_work_id"`
	WorkTitle           string    `json:"work_title,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// InstitutionalFilterMode selects Laravel's two documented filter semantics.
type InstitutionalFilterMode int

const (
	// FilterSimple: intersect the user's works directly with the product's,
	// and additionally allow products with no institutional restriction (empty array).
	// Laravel: applyInstitutionalWorksFilter_Simple — customer dashboard.
	FilterSimple InstitutionalFilterMode = iota

	// FilterWithConnections: resolve connections first, then intersect.
	// Products with no restriction are NOT visible.
	// Laravel: applyInstitutionalWorksFilter_WithConnections — purchase-request pages.
	FilterWithConnections
)

// UserOrganizationStatus represents the approval state of a customer-to-vendor organization linkage.
type UserOrganizationStatus string

const (
	UserOrgStatusPending  UserOrganizationStatus = "pending"
	UserOrgStatusApproved UserOrganizationStatus = "approved"
	UserOrgStatusRejected UserOrganizationStatus = "rejected"
)

// UserOrganization links a customer user (pharmacy) with a vendor organization using their organization number.
type UserOrganization struct {
	ID                 int64                  `json:"id"`
	UserID             int64                  `json:"user_id"`
	UserName           string                 `json:"user_name,omitempty"`
	UserEmail          string                 `json:"user_email,omitempty"`
	CustomerOrgID      *int64                 `json:"customer_org_id,omitempty"`
	CustomerOrgName    string                 `json:"customer_org_name,omitempty"`
	VendorOrgID        int64                  `json:"vendor_org_id"`
	VendorOrgName      string                 `json:"vendor_org_name,omitempty"`
	VendorOrgType      string                 `json:"vendor_org_type,omitempty"`
	OrganizationNumber string                 `json:"organization_number"`
	Status             UserOrganizationStatus `json:"status"`
	Notes              string                 `json:"notes,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}
