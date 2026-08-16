package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Service coordinates purchasing optimization, weekly coverage, and issue resolution.
type Service struct {
	repo Repository
	log  *slog.Logger
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
	return s.repo.SaveWeeklyCoverage(ctx, c)
}

// GetBranchCoverage lists coverage windows for a branch.
func (s *Service) GetBranchCoverage(ctx context.Context, branchID int64) ([]*WeeklyCoverage, error) {
	return s.repo.ListWeeklyCoverage(ctx, branchID)
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
