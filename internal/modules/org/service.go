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

// GetOrganization returns an organization by ID.
func (s *Service) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	return s.repo.GetOrganizationByID(ctx, id)
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

// ListBranches returns all branches for an organization.
func (s *Service) ListBranches(ctx context.Context, orgID int64) ([]*Branch, error) {
	return s.repo.ListBranchesByOrg(ctx, orgID)
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
		IsApproved:     true,
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
