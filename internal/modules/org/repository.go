package org

import (
	"context"
)

// Repository defines the storage contract for organization management.
type Repository interface {
	CreateOrganization(ctx context.Context, o *Organization) error
	GetOrganizationByID(ctx context.Context, id int64) (*Organization, error)
	UpdateOrganizationStatus(ctx context.Context, id int64, status OrganizationStatus) error
	ReviewOrganization(ctx context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error
	UpdateOrganization(ctx context.Context, o *Organization) error
	DeleteOrganization(ctx context.Context, id int64) error
	ListOrganizations(ctx context.Context, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, error)
	CountOrganizations(ctx context.Context, orgType *OrganizationType, status *OrganizationStatus) (int, error)

	CreateBranch(ctx context.Context, b *Branch) error
	GetBranchByID(ctx context.Context, id int64) (*Branch, error)
	UpdateBranch(ctx context.Context, b *Branch) error
	DeleteBranch(ctx context.Context, id, orgID int64) error
	ListBranchesByOrg(ctx context.Context, orgID int64) ([]*Branch, error)
	UnsetMainBranches(ctx context.Context, orgID int64) error
	AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error

	AddMember(ctx context.Context, m *Member) error
	UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error
	ListMembersByOrg(ctx context.Context, orgID int64) ([]*Member, error)
	ListEmployees(ctx context.Context, orgID int64) ([]*EmployeeView, error)
	RemoveMember(ctx context.Context, orgID, userID int64) error


	// Custom Roles
	CreateRole(ctx context.Context, role *Role) error
	ListRolesByOrg(ctx context.Context, orgID int64) ([]*Role, error)

	// Delivery Bands
	GetDeliveryBands(ctx context.Context, orgID int64) ([]*DeliveryBand, error)
	SaveDeliveryBands(ctx context.Context, orgID int64, bands []*DeliveryBand) error

	// Multi-criteria Reviews
	AddReview(ctx context.Context, r *Review) error
	AddReviewWithRatings(ctx context.Context, r *Review, ratings []ReviewRating) error
	ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Review, error)
	GetReviewCriteria(ctx context.Context, contextType string) ([]*ReviewCriterion, error)
	ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error

	ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error)
	IsFollowing(ctx context.Context, orgID, userID int64) (bool, error)
	ListFollowedOrgs(ctx context.Context, userID int64) ([]*Organization, error)

	CreatePolicy(ctx context.Context, p *Policy) error
	ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*Policy, error)
}
