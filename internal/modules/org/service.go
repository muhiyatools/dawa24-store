package org

import (
	"context"
	"fmt"
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

// UpdateOrganizationAICredentials updates the linked AI Gateway virtual key and user ID.
func (s *Service) UpdateOrganizationAICredentials(ctx context.Context, id int64, aiUserID, aiVirtualKey string) error {
	return s.repo.UpdateOrganizationAICredentials(ctx, id, aiUserID, aiVirtualKey)
}

// GetOrganization returns an organization by ID.
func (s *Service) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	return s.repo.GetOrganizationByID(ctx, id)
}

// GetSupplierProfile returns full commercial profile for a supplier/vendor organization.
func (s *Service) GetSupplierProfile(ctx context.Context, id int64) (*SupplierOrgProfile, error) {
	return s.repo.GetSupplierProfile(ctx, id)
}

// UpdateSupplierProfile updates commercial profile and limits for a supplier/vendor organization.
func (s *Service) UpdateSupplierProfile(ctx context.Context, p *SupplierOrgProfile) error {
	if p.NameAr == "" && p.NameEn == "" {
		return apperr.Validation("name", "Organization name cannot be empty", nil)
	}
	if p.MaxOrderPrice.Minor() < p.MinOrderPrice.Minor() {
		return apperr.Validation("max_order_price", "Maximum order price must be greater than or equal to minimum order price", nil)
	}
	return s.repo.UpdateSupplierProfile(ctx, p)
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

// ListOrganizationsWithTotal returns paginated organizations matching search & filters with total count.
func (s *Service) ListOrganizationsWithTotal(
	ctx context.Context,
	search string,
	orgType *OrganizationType,
	status *OrganizationStatus,
	limit, offset int,
) ([]*Organization, int, error) {
	return s.repo.ListOrganizationsWithTotal(ctx, search, orgType, status, limit, offset)
}

// AdminOrgStats returns aggregated platform metrics for organizations.
func (s *Service) AdminOrgStats(ctx context.Context) (AdminOrgStatsResult, error) {
	return s.repo.AdminOrgStats(ctx)
}

// CountBranchesByOrg returns branch counts grouped by organization.
func (s *Service) CountBranchesByOrg(ctx context.Context) (map[int64]int, error) {
	return s.repo.CountBranchesByOrg(ctx)
}

// CreateBranch adds a branch location, enforcing that only one branch has is_main = true.
//
// Organizations no longer get an auto-provisioned branch at registration, so the
// first branch an owner creates must become the main one even if the form did
// not tick "main" — otherwise the org would have branches but no default, and
// every "main branch" lookup would come back empty.
func (s *Service) CreateBranch(ctx context.Context, b *Branch) error {
	if err := b.Validate(); err != nil {
		return err
	}

	if !b.IsMain {
		existing, err := s.repo.ListBranchesByOrg(ctx, b.OrganizationID)
		if err == nil && len(existing) == 0 {
			b.IsMain = true
		}
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

// ListBranchesWithTotal returns paginated branches matching filter and total count.
func (s *Service) ListBranchesWithTotal(ctx context.Context, filter BranchFilter, limit, offset int) ([]*Branch, int, error) {
	return s.repo.ListBranchesWithTotal(ctx, filter, limit, offset)
}

// AdminBranchStats provides platform-wide branch and warehouse counts.
func (s *Service) AdminBranchStats(ctx context.Context) (AdminBranchStatsResult, error) {
	return s.repo.AdminBranchStats(ctx)
}

// ListEmployees returns all employees with user profile, role and branch manager status.
func (s *Service) ListEmployees(ctx context.Context, orgID int64) ([]*EmployeeView, error) {
	return s.repo.ListEmployees(ctx, orgID)
}

// ListEmployeesWithTotal returns paginated employees with user profile, role, branch manager status, and total count.
func (s *Service) ListEmployeesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*EmployeeView, int, error) {
	return s.repo.ListEmployeesWithTotal(ctx, orgID, limit, offset)
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
		m.RoleKey = "org_pharmacist"
	}
	if m.EmployeeCode == "" {
		m.EmployeeCode = fmt.Sprintf("EMP-%d", m.UserID)
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

// GetMemberByID returns one membership row scoped to its organization.
func (s *Service) GetMemberByID(ctx context.Context, orgID, memberID int64) (*Member, error) {
	if orgID <= 0 || memberID <= 0 {
		return nil, apperr.Validation("member.invalid", "Valid org and member IDs are required.", nil)
	}
	return s.repo.GetMemberByID(ctx, orgID, memberID)
}

// ToggleMemberStatus toggles a member's active status.
func (s *Service) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	if orgID <= 0 || memberID <= 0 {
		return apperr.Validation("member.invalid", "Valid org and member IDs are required.", nil)
	}
	return s.repo.ToggleMemberStatus(ctx, orgID, memberID)
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

// SavePolicies updates an organization's policies.
func (s *Service) SavePolicies(ctx context.Context, orgID int64, policies []*Policy) error {
	return s.repo.SavePolicies(ctx, orgID, policies)
}
