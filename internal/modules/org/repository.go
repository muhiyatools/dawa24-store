package org

import (
	"context"
)

// Repository defines the storage contract for organization management.
type Repository interface {
	CreateOrganization(ctx context.Context, o *Organization) error
	GetOrganizationByID(ctx context.Context, id int64) (*Organization, error)
	GetSupplierProfile(ctx context.Context, id int64) (*SupplierOrgProfile, error)
	UpdateSupplierProfile(ctx context.Context, p *SupplierOrgProfile) error
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
	ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error
	ListMembersByOrg(ctx context.Context, orgID int64) ([]*Member, error)
	ListEmployees(ctx context.Context, orgID int64) ([]*EmployeeView, error)
	RemoveMember(ctx context.Context, orgID, userID int64) error

	// Custom Roles
	CreateRole(ctx context.Context, role *Role) error
	GetRoleByID(ctx context.Context, id int64) (*Role, error)
	UpdateRole(ctx context.Context, role *Role) error
	DeleteRole(ctx context.Context, id int64) error
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
	SavePolicies(ctx context.Context, orgID int64, policies []*Policy) error

	ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*SocialMedia, error)
	SaveSocialMedia(ctx context.Context, orgID int64, links []*SocialMedia) error

	// Institutional Works (الهيكل المؤسسي وأنواعه)
	CreateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error
	GetInstitutionalWorkByID(ctx context.Context, id int64) (*InstitutionalWork, error)
	UpdateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error
	DeleteInstitutionalWork(ctx context.Context, id int64) error
	ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error
	ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error)
	ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error)
	CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error)
	AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error
	GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*InstitutionalWork, error)

	// Employee Institutional Works (مجموعات العمل للمستخدمين والموظفين)
	AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error
	RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error
	ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*EmployeeInstitutionalWork, error)
	ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*EmployeeInstitutionalWork, error)
	GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error)
	GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error)
}
