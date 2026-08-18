package org

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// RegisterOrgInput specifies parameters for registering a tenant organization.
type RegisterOrgInput struct {
	LegalName          string
	TradeName          i18n.Text
	TaxNumber          string
	CommercialRegister string
	Type               OrganizationType
	CreditLimit        money.Amount
	PaymentTermsDays   int
}

// Service manages organization lifecycles, branch networks, and membership RBAC.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new organization service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// RegisterOrganization registers a new tenant awaiting approval.
func (s *Service) RegisterOrganization(ctx context.Context, input RegisterOrgInput) (*Organization, error) {
	o := &Organization{
		LegalName:          input.LegalName,
		TradeName:          input.TradeName,
		TaxNumber:          input.TaxNumber,
		CommercialRegister: input.CommercialRegister,
		Type:               input.Type,
		Status:             StatusPending,
		CreditLimit:        input.CreditLimit,
		PaymentTermsDays:   input.PaymentTermsDays,
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateOrganization(ctx, o); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "organization registered", "org_id", o.ID, "legal_name", o.LegalName, "type", o.Type)
	return o, nil
}

// ApproveOrganization approves an organization tenant.
func (s *Service) ApproveOrganization(ctx context.Context, id int64) error {
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusApproved); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization approved", "org_id", id)
	return nil
}

// RejectOrganization rejects an organization tenant.
func (s *Service) RejectOrganization(ctx context.Context, id int64) error {
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusRejected); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization rejected", "org_id", id)
	return nil
}

// SuspendOrganization suspends an active organization tenant.
func (s *Service) SuspendOrganization(ctx context.Context, id int64) error {
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusSuspended); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization suspended", "org_id", id)
	return nil
}

// ReviewOrganization handles full administrative review with notes, rejection reasons, and audit stamps.
func (s *Service) ReviewOrganization(ctx context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	if err := s.repo.ReviewOrganization(ctx, id, status, notes, rejectionReason, adminID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization reviewed", "org_id", id, "status", status, "admin_id", adminID)
	return nil
}

// GetOrganization returns an organization by ID.
func (s *Service) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	return s.repo.GetOrganizationByID(ctx, id)
}

// CountOrganizations returns the number of organizations matching a filter,
// for dashboards that need a total rather than a page.
func (s *Service) CountOrganizations(
	ctx context.Context,
	orgType *OrganizationType,
	status *OrganizationStatus,
) (int, error) {
	return s.repo.CountOrganizations(ctx, orgType, status)
}

// ListOrganizations lists filtered organizations.
func (s *Service) ListOrganizations(
	ctx context.Context,
	orgType *OrganizationType,
	status *OrganizationStatus,
	limit, offset int,
) ([]*Organization, error) {
	return s.repo.ListOrganizations(ctx, orgType, status, limit, offset)
}

// CreateBranch adds a branch location, enforcing that only one branch has is_main = true.
func (s *Service) CreateBranch(ctx context.Context, b *Branch) error {
	if err := b.Validate(); err != nil {
		return err
	}

	if b.IsMain {
		if err := s.repo.UnsetMainBranches(ctx, b.OrganizationID); err != nil {
			return err
		}
	}

	if err := s.repo.CreateBranch(ctx, b); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "branch created", "branch_id", b.ID, "org_id", b.OrganizationID, "code", b.Code, "is_main", b.IsMain)
	return nil
}

// GetBranch returns a single branch by ID.
func (s *Service) GetBranch(ctx context.Context, id int64) (*Branch, error) {
	return s.repo.GetBranchByID(ctx, id)
}

// UpdateBranch updates branch details.
func (s *Service) UpdateBranch(ctx context.Context, b *Branch) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if b.IsMain {
		if err := s.repo.UnsetMainBranches(ctx, b.OrganizationID); err != nil {
			return err
		}
	}
	if err := s.repo.UpdateBranch(ctx, b); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "branch updated", "branch_id", b.ID, "org_id", b.OrganizationID)
	return nil
}

// DeleteBranch removes a branch.
func (s *Service) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteBranch(ctx, id, orgID)
}

// AssignBranchManager assigns or unassigns an employee user as the branch manager.
func (s *Service) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	if err := s.repo.AssignBranchManager(ctx, orgID, branchID, managerUserID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "branch manager assigned", "org_id", orgID, "branch_id", branchID, "manager_user_id", managerUserID)
	return nil
}

// ListBranches returns all branches for an organization.
func (s *Service) ListBranches(ctx context.Context, orgID int64) ([]*Branch, error) {
	return s.repo.ListBranchesByOrg(ctx, orgID)
}

// ListEmployees returns all employees with user profile, role and branch manager status.
func (s *Service) ListEmployees(ctx context.Context, orgID int64) ([]*EmployeeView, error) {
	return s.repo.ListEmployees(ctx, orgID)
}


// AddMemberDirect adds or updates a member directly with full attributes.
func (s *Service) AddMemberDirect(ctx context.Context, m *Member) error {
	if m.OrganizationID <= 0 || m.UserID <= 0 {
		return apperr.Validation("member.invalid", "Valid org and user are required.", nil)
	}
	if m.RoleID <= 0 {
		m.RoleID = 1
	}
	if m.RoleKey == "" {
		m.RoleKey = "org_employee"
	}
	if err := s.repo.AddMember(ctx, m); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "member added direct", "org_id", m.OrganizationID, "user_id", m.UserID, "branch_id", m.BranchID)
	return nil
}

// AddMember adds or updates user role in an organization.
func (s *Service) AddMember(ctx context.Context, orgID, userID, roleID int64) (*Member, error) {

	if orgID <= 0 || userID <= 0 || roleID <= 0 {
		return nil, apperr.Validation("member.invalid", "Valid org, user, and role IDs are required.", nil)
	}

	m := &Member{
		OrganizationID: orgID,
		UserID:         userID,
		RoleID:         roleID,
		IsActive:       true,
	}

	if err := s.repo.AddMember(ctx, m); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "member added", "org_id", orgID, "user_id", userID, "role_id", roleID)
	return m, nil
}

// AddMemberByRoleKey adds a member with an organization role key.
func (s *Service) AddMemberByRoleKey(ctx context.Context, orgID, userID int64, roleKey string) (*Member, error) {
	if orgID <= 0 || userID <= 0 {
		return nil, apperr.Validation("member.invalid", "Valid org and user are required.", nil)
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	m := &Member{
		OrganizationID: orgID,
		UserID:         userID,
		RoleKey:        roleKey,
		IsActive:       true,
	}
	if err := s.repo.AddMember(ctx, m); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "member added", "org_id", orgID, "user_id", userID, "role", roleKey)
	return m, nil
}

// ListMembers returns all members of an organization.
func (s *Service) ListMembers(ctx context.Context, orgID int64) ([]*Member, error) {
	return s.repo.ListMembersByOrg(ctx, orgID)
}

// RemoveMember removes a user from an organization.
func (s *Service) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return s.repo.RemoveMember(ctx, orgID, userID)
}

// AddReview records a customer review for a vendor organization.
func (s *Service) AddReview(ctx context.Context, orgID, userID int64, rating int, text string) (*Review, error) {
	if rating < 1 || rating > 5 {
		return nil, apperr.Validation("review.rating_invalid", "Rating must be between 1 and 5.", nil)
	}

	rev := &Review{
		OrganizationID: orgID,
		UserID:         userID,
		Rating:         rating,
		ReviewText:     text,
		Status:         "approved",
		IsVerified:     true,
	}


	if err := s.repo.AddReview(ctx, rev); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "organization review added", "org_id", orgID, "user_id", userID, "rating", rating)
	return rev, nil
}

// ListReviews returns approved reviews for an organization.
func (s *Service) ListReviews(ctx context.Context, orgID int64, limit, offset int) ([]*Review, error) {
	return s.repo.ListReviewsByOrg(ctx, orgID, limit, offset)
}

// ToggleFollow toggles following status for an organization.
func (s *Service) ToggleFollow(ctx context.Context, orgID, userID int64) (bool, error) {
	return s.repo.ToggleFollower(ctx, orgID, userID)
}

// IsFollowing reports whether a user follows an organization.
func (s *Service) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	return s.repo.IsFollowing(ctx, orgID, userID)
}

// ListFollowedOrganizations returns the list of organizations followed by a user.
func (s *Service) ListFollowedOrganizations(ctx context.Context, userID int64) ([]*Organization, error) {
	return s.repo.ListFollowedOrgs(ctx, userID)
}

// ListPolicies returns an organization's active policies.
func (s *Service) ListPolicies(ctx context.Context, orgID int64) ([]*Policy, error) {
	return s.repo.ListPoliciesByOrg(ctx, orgID)
}

// UpdateOrganization updates organization details.
func (s *Service) UpdateOrganization(ctx context.Context, o *Organization) error {
	if err := o.Validate(); err != nil {
		return err
	}
	return s.repo.UpdateOrganization(ctx, o)
}

// DeleteOrganization deactivates an organization.
func (s *Service) DeleteOrganization(ctx context.Context, id int64) error {
	return s.repo.DeleteOrganization(ctx, id)
}

// UpdateMemberRole changes a member role.

func (s *Service) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return s.repo.UpdateMemberRole(ctx, orgID, userID, role)
}

// CreateRole adds a custom organization role.
func (s *Service) CreateRole(ctx context.Context, role *Role) error {
	if role.OrganizationID <= 0 || role.Key == "" {
		return apperr.Validation("role.invalid", "Organization and role key are required.", nil)
	}
	return s.repo.CreateRole(ctx, role)
}

// ListRoles retrieves all roles for an organization.
func (s *Service) ListRoles(ctx context.Context, orgID int64) ([]*Role, error) {
	return s.repo.ListRolesByOrg(ctx, orgID)
}

// GetDeliveryBands retrieves delivery bands for distance pricing.
func (s *Service) GetDeliveryBands(ctx context.Context, orgID int64) ([]*DeliveryBand, error) {
	return s.repo.GetDeliveryBands(ctx, orgID)
}

// SaveDeliveryBands updates the delivery bands for an organization.
func (s *Service) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*DeliveryBand) error {
	return s.repo.SaveDeliveryBands(ctx, orgID, bands)
}

// GetReviewCriteria returns review criteria for a given context.
func (s *Service) GetReviewCriteria(ctx context.Context, contextType string) ([]*ReviewCriterion, error) {
	return s.repo.GetReviewCriteria(ctx, contextType)
}

// AddReviewWithRatings adds a multi-criteria review with verified rating weights.
func (s *Service) AddReviewWithRatings(ctx context.Context, rev *Review, ratings []ReviewRating) error {
	if rev.Rating < 1 || rev.Rating > 5 {
		return apperr.Validation("review.rating_invalid", "Rating must be between 1 and 5.", nil)
	}
	return s.repo.AddReviewWithRatings(ctx, rev, ratings)
}

// SubmitReview records a review and its individual criteria ratings.
func (s *Service) SubmitReview(ctx context.Context, rev *Review) error {
	if rev.Rating < 1 {
		rev.Rating = 5
	}
	if rev.Rating > 5 {
		rev.Rating = 5
	}
	if err := s.repo.AddReview(ctx, rev); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "review submitted", "org_id", rev.OrganizationID, "user_id", rev.UserID, "rating", rev.Rating)
	return nil
}

// CreateInstitutionalWork creates a new institutional structure category.
func (s *Service) CreateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error {
	if err := s.repo.CreateInstitutionalWork(ctx, iw); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "institutional work created", "id", iw.ID, "title_ar", iw.Title.Get("ar"))
	return nil
}

// GetInstitutionalWork returns an institutional work by its ID.
func (s *Service) GetInstitutionalWork(ctx context.Context, id int64) (*InstitutionalWork, error) {
	return s.repo.GetInstitutionalWorkByID(ctx, id)
}

// UpdateInstitutionalWork updates an existing institutional category.
func (s *Service) UpdateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error {
	return s.repo.UpdateInstitutionalWork(ctx, iw)
}

// DeleteInstitutionalWork soft-deletes an institutional category.
func (s *Service) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return s.repo.DeleteInstitutionalWork(ctx, id)
}

// ToggleInstitutionalWorkStatus toggles active/inactive state of an institutional category.
func (s *Service) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleInstitutionalWorkStatus(ctx, id)
}

// ListInstitutionalWorks returns the full hierarchical tree of institutional categories.
func (s *Service) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error) {
	return s.repo.ListInstitutionalWorks(ctx, onlyActive)
}

// AssignBranchInstitutionalWorks assigns institutional categories to a branch.
func (s *Service) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	return s.repo.AssignBranchInstitutionalWorks(ctx, branchID, workIDs)
}

// GetBranchInstitutionalWorks returns all institutional categories assigned to a branch.
func (s *Service) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*InstitutionalWork, error) {
	return s.repo.GetBranchInstitutionalWorks(ctx, branchID)
}



