// Package workflow handles purchasing priority optimization, branch route weekly coverage,
// and issue resolution tracking.
package workflow

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// PurchasePriorityRequest captures parameters for automated bulk purchasing optimization.
type PurchasePriorityRequest struct {
	ID                             int64          `json:"id"`
	PublicID                       string         `json:"public_id"`
	UserID                         int64          `json:"user_id"`
	OrganizationID                 *int64         `json:"organization_id,omitempty"`
	RequestNumber                  string         `json:"request_number"`
	Status                         string         `json:"status"`
	PriorityHighestDiscount        bool           `json:"priority_highest_discount"`
	PriorityLowestPrice            bool           `json:"priority_lowest_price"`
	PriorityFastestDelivery        bool           `json:"priority_fastest_delivery"`
	PriorityPreferredSuppliersOnly bool           `json:"priority_preferred_suppliers_only"`
	BudgetConstraint               money.Amount   `json:"budget_constraint"`
	Parameters                     map[string]any `json:"parameters,omitempty"`
	Recommendations                map[string]any `json:"recommendations,omitempty"`
	ProcessedAt                    *time.Time     `json:"processed_at,omitempty"`
	CreatedAt                      time.Time      `json:"created_at"`
	UpdatedAt                      time.Time      `json:"updated_at"`
}

// WeeklyCoverage defines branch geographic delivery schedules.
type WeeklyCoverage struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	BranchID       int64     `json:"branch_id"`
	DayOfWeek      int       `json:"day_of_week"` // 0 = Sunday
	CoverageFrom   string    `json:"coverage_from,omitempty"`
	CoverageTo     string    `json:"coverage_to,omitempty"`
	Address        string    `json:"address,omitempty"`
	DistanceMeters int       `json:"distance_meters"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ReportIssue tracks customer support and quality tickets.
type ReportIssue struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	ReportedBy     int64     `json:"reported_by"`
	OrganizationID *int64    `json:"organization_id,omitempty"`
	OrderID        *int64    `json:"order_id,omitempty"`
	IssueType      string    `json:"issue_type"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	Priority       string    `json:"priority"`
	ResponseNotes  string    `json:"response_notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate ensures issue reports have required details.
func (r *ReportIssue) Validate() error {
	if r.ReportedBy <= 0 {
		return apperr.Validation("issue.user_required", "User ID is required.", nil)
	}
	if r.Description == "" {
		return apperr.Validation("issue.description_required", "Description is required.", nil)
	}
	return nil
}

// RequestType classifies a document/action request.
type RequestType string

const (
	RequestDocument RequestType = "document"
	RequestAction   RequestType = "action"
	RequestApproval RequestType = "approval"
)

// RequestStatus is the lifecycle state of a request.
type RequestStatus string

const (
	RequestPending   RequestStatus = "pending"
	RequestAccepted  RequestStatus = "accepted"
	RequestDeclined  RequestStatus = "declined"
	RequestCancelled RequestStatus = "cancelled"
)

// Request is a document/action request between parties (legacy ask_fors).
type Request struct {
	ID          int64         `json:"id"`
	PublicID    string        `json:"public_id"`
	Type        RequestType   `json:"type"`
	Title       i18n.Text     `json:"title"`
	Description string        `json:"description"`
	Status      RequestStatus `json:"status"`
	ActionURL   string        `json:"action_url,omitempty"`
	FromUserID  int64         `json:"from_user_id"`
	FromOrgID   int64         `json:"from_org_id"`
	ToOrgID     int64         `json:"to_org_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Validate ensures a request names both parties and a type.
func (r *Request) Validate() error {
	if r.FromOrgID <= 0 || r.ToOrgID <= 0 {
		return apperr.Validation("request.org_required", "Both request parties are required.", nil)
	}
	if r.FromOrgID == r.ToOrgID {
		return apperr.Validation("request.same_org", "Cannot send a request to your own organization.", nil)
	}
	if r.Type == "" {
		r.Type = RequestDocument
	}
	return nil
}

// PricingType classifies an institutional service.
type PricingType string

const (
	PricingFree         PricingType = "free"
	PricingPaid         PricingType = "paid"
	PricingSubscription PricingType = "subscription"
)

// InstitutionalService is a hierarchical institutional service (legacy
// institutional_works).
type InstitutionalService struct {
	ID          int64       `json:"id"`
	PublicID    string      `json:"public_id"`
	Title       i18n.Text   `json:"title"`
	Description i18n.Text   `json:"description"`
	Icon        string      `json:"icon,omitempty"`
	PricingType PricingType `json:"pricing_type"`
	ParentID    *int64      `json:"parent_id,omitempty"`
	IsActive    bool        `json:"is_active"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
