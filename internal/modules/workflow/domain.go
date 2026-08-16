// Package workflow handles purchasing priority optimization, branch route weekly coverage,
// and issue resolution tracking.
package workflow

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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
