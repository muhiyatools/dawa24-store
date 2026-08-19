package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service coordinates purchasing optimization, weekly coverage, and issue resolution.
type Service struct {
	repo     Repository
	log      *slog.Logger
	instGate InstitutionalGate
}

// NewService creates a new workflow service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreatePriorityRequest starts a purchasing optimization computation.
func (s *Service) CreatePriorityRequest(ctx context.Context, userID int64, req *PurchasePriorityRequest) (*PurchasePriorityRequest, error) {
	req.UserID = userID
	orgID, ok := database.TenantFrom(ctx)
	if ok {
		req.OrganizationID = &orgID
	}
	req.Status = "pending"
	req.RequestNumber = fmt.Sprintf("REQ-%s-%04d", time.Now().Format("20060102"), time.Now().UnixNano()%10000)

	if err := s.repo.CreatePriorityRequest(ctx, req); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "purchase priority request submitted", "id", req.ID, "number", req.RequestNumber)
	return req, nil
}

// SetWeeklyCoverage configures geographic delivery windows for a branch.
func (s *Service) SetWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	c.OrganizationID = orgID
	c.IsActive = true
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.SaveWeeklyCoverage(ctx, c)
}

// CreateWeeklyCoverage creates a new branch weekly coverage entry.
func (s *Service) CreateWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.SaveWeeklyCoverage(ctx, c)
}

// UpdateWeeklyCoverage updates an existing branch weekly coverage entry.
func (s *Service) UpdateWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return s.repo.UpdateWeeklyCoverage(ctx, c)
}

// DeleteWeeklyCoverage deletes a weekly coverage entry.
func (s *Service) DeleteWeeklyCoverage(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.DeleteWeeklyCoverage(ctx, id)
}

// ToggleWeeklyCoverage toggles the active state of a coverage entry.
func (s *Service) ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error {
	if id <= 0 {
		return apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.ToggleWeeklyCoverage(ctx, id, isActive)
}

// GetWeeklyCoverage retrieves a weekly coverage record by ID.
func (s *Service) GetWeeklyCoverage(ctx context.Context, id int64) (*WeeklyCoverage, error) {
	if id <= 0 {
		return nil, apperr.Validation("coverage.id_invalid", "Invalid coverage ID.", nil)
	}
	return s.repo.GetWeeklyCoverageByID(ctx, id)
}

// GetBranchCoverage lists coverage windows for a branch.
func (s *Service) GetBranchCoverage(ctx context.Context, branchID int64) ([]*WeeklyCoverage, error) {
	return s.repo.ListWeeklyCoverage(ctx, branchID)
}

// ListCoverageForOrganization lists all weekly coverage records for an organization with joined branch names.
func (s *Service) ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*CoverageView, error) {
	return s.repo.ListCoverageForOrganization(ctx, orgID)
}

// ReportIssue logs a support inquiry or product defect.
func (s *Service) ReportIssue(ctx context.Context, i *ReportIssue) (*ReportIssue, error) {
	if err := i.Validate(); err != nil {
		return nil, err
	}
	i.Status = "pending"
	if i.Priority == "" {
		i.Priority = "medium"
	}

	if err := s.repo.CreateIssue(ctx, i); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "issue reported", "issue_id", i.ID, "type", i.IssueType)
	return i, nil
}

// ListIssues retrieves issues with pagination.
func (s *Service) ListIssues(ctx context.Context, limit, offset int) ([]*ReportIssue, error) {
	return s.repo.ListIssues(ctx, limit, offset)
}

// CreateRequest sends a document/action request to another organization.
func (s *Service) CreateRequest(ctx context.Context, fromUserID, fromOrgID, toOrgID int64, typ RequestType, title i18n.Text, description string) (*Request, error) {
	r := &Request{
		FromUserID:  fromUserID,
		FromOrgID:   fromOrgID,
		ToOrgID:     toOrgID,
		Type:        typ,
		Title:       title,
		Description: description,
		Status:      RequestPending,
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.CreateRequest(ctx, r); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "request created", "request_id", r.ID, "from", fromOrgID, "to", toOrgID)
	return r, nil
}

// ListInbox returns requests sent to or from the caller's organization.
func (s *Service) ListInbox(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, error) {
	return s.repo.ListRequestsByOrg(ctx, orgID, status, limit, offset)
}

// RespondRequest accepts or declines a request.
func (s *Service) RespondRequest(ctx context.Context, id int64, status RequestStatus) error {
	if status != RequestAccepted && status != RequestDeclined && status != RequestCancelled {
		return apperr.Validation("request.status_invalid", "Invalid request status.", nil)
	}
	return s.repo.UpdateRequestStatus(ctx, id, status)
}

// CreateService adds an institutional service to the catalogue.
func (s *Service) CreateService(ctx context.Context, svc *InstitutionalService) (*InstitutionalService, error) {
	if svc.Title.IsEmpty() {
		return nil, apperr.Validation("service.title_required", "A service title is required.", nil)
	}
	if svc.PricingType == "" {
		svc.PricingType = PricingFree
	}
	svc.IsActive = true
	if err := s.repo.CreateService(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// ListServices returns services at a level of the hierarchy (top-level when
// parentID is nil).
func (s *Service) ListServices(ctx context.Context, parentID *int64) ([]*InstitutionalService, error) {
	return s.repo.ListServices(ctx, parentID)
}

// GetService returns one institutional service.
func (s *Service) GetService(ctx context.Context, id int64) (*InstitutionalService, error) {
	return s.repo.GetServiceByID(ctx, id)
}
