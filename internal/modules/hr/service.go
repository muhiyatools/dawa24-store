package hr

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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

// ListPublishedJobs returns published vacancies for the public job board.
func (s *Service) ListPublishedJobs(ctx context.Context, limit, offset int) ([]*JobOffer, error) {
	return s.repo.ListPublishedJobs(ctx, limit, offset)
}

// GetJobOffer returns one vacancy.
func (s *Service) GetJobOffer(ctx context.Context, id int64) (*JobOffer, error) {
	return s.repo.GetJobOfferByID(ctx, id)
}

// CreateJobOffer publishes a vacancy for an organization.
func (s *Service) CreateJobOffer(ctx context.Context, orgID int64, j *JobOffer) (*JobOffer, error) {
	j.OrganizationID = orgID
	if j.Status == "" {
		j.Status = "published"
	}
	if err := s.repo.CreateJobOffer(ctx, j); err != nil {
		return nil, err
	}
	return j, nil
}

// ListOrgJobs returns a tenant's own postings.
func (s *Service) ListOrgJobs(ctx context.Context, orgID int64, limit, offset int) ([]*JobOffer, error) {
	return s.repo.ListJobsByOrg(ctx, orgID, limit, offset)
}

// ApplyToJob records an application for a vacancy.
func (s *Service) ApplyToJob(ctx context.Context, a *JobApplication) error {
	if a.ApplicantName == "" || a.ApplicantEmail == "" {
		return apperr.Validation("job.apply_required", "Name and email are required.", nil)
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	return s.repo.CreateJobApplication(ctx, a)
}

// ListApplicationsByOffer returns applications for a specific job offer.
func (s *Service) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByOffer(ctx, offerID, limit, offset)
}

// ListApplicationsByUser returns applications submitted by a user.
func (s *Service) ListApplicationsByUser(ctx context.Context, userID int64) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByUser(ctx, userID)
}

// UpdateApplicationStatus updates application review status.
func (s *Service) UpdateApplicationStatus(ctx context.Context, appID int64, status, notes string) error {
	return s.repo.UpdateApplicationStatus(ctx, appID, status, notes)
}

// ListApplications returns applications for a vacancy.
func (s *Service) ListApplications(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error) {
	return s.repo.ListApplicationsByOffer(ctx, offerID, limit, offset)
}

// GetJobSeekerProfile retrieves the seeker profile for a user.
func (s *Service) GetJobSeekerProfile(ctx context.Context, userID int64) (*JobSeekerProfile, error) {
	return s.repo.GetJobSeekerProfile(ctx, userID)
}

// SaveJobSeekerProfile saves or updates a job seeker profile.
func (s *Service) SaveJobSeekerProfile(ctx context.Context, p *JobSeekerProfile) error {
	if p.UserID <= 0 {
		return apperr.Validation("seeker.user_required", "User ID is required.", nil)
	}
	return s.repo.UpsertJobSeekerProfile(ctx, p)
}
