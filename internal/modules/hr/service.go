package hr

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Service coordinates HR operations, payroll configuration, and business hours.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new HR service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateEmployee onboards a new staff member.
func (s *Service) CreateEmployee(ctx context.Context, e *Employee) (*Employee, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	e.OrganizationID = orgID

	if err := e.Validate(); err != nil {
		return nil, err
	}
	if e.Status == "" {
		e.Status = "active"
	}

	if err := s.repo.CreateEmployee(ctx, e); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "employee onboarded", "employee_id", e.ID, "code", e.EmployeeCode)
	return e, nil
}

// GetEmployee retrieves an employee by ID.
func (s *Service) GetEmployee(ctx context.Context, id int64) (*Employee, error) {
	return s.repo.GetEmployeeByID(ctx, id)
}

// ListEmployees retrieves paginated employees for the tenant.
func (s *Service) ListEmployees(ctx context.Context, limit, offset int) ([]*Employee, error) {
	return s.repo.ListEmployees(ctx, limit, offset)
}

// SaveWorkTimes updates weekly business shifts.
func (s *Service) SaveWorkTimes(ctx context.Context, times []*WorkTime) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	for _, wt := range times {
		wt.OrganizationID = orgID
	}
	return s.repo.SaveWorkTimes(ctx, times)
}

// ListWorkTimes returns business operating hours.
func (s *Service) ListWorkTimes(ctx context.Context) ([]*WorkTime, error) {
	return s.repo.ListWorkTimes(ctx)
}
