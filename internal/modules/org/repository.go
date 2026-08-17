package org

import (
	"context"
)

// Repository defines the storage contract for organization management.
type Repository interface {
	CreateOrganization(ctx context.Context, o *Organization) error
	GetOrganizationByID(ctx context.Context, id int64) (*Organization, error)
	UpdateOrganizationStatus(ctx context.Context, id int64, status OrganizationStatus) error
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

	AddMember(ctx context.Context, m *Member) error
	UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error
	ListMembersByOrg(ctx context.Context, orgID int64) ([]*Member, error)
	RemoveMember(ctx context.Context, orgID, userID int64) error

	AddReview(ctx context.Context, r *Review) error
	ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Review, error)

	ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error)
	IsFollowing(ctx context.Context, orgID, userID int64) (bool, error)
	ListFollowedOrgs(ctx context.Context, userID int64) ([]*Organization, error)

	CreatePolicy(ctx context.Context, p *Policy) error
	ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*Policy, error)

	CreateHighlightSection(ctx context.Context, s *HighlightSection) error
	ListHighlightSections(ctx context.Context, orgID int64) ([]*HighlightSection, error)
	AddHighlightItem(ctx context.Context, item *HighlightSectionItem) error
	ListHighlightItems(ctx context.Context, sectionID int64) ([]*HighlightSectionItem, error)
}
