package hr

import (
	"context"
)

// Repository defines storage operations for HR employee records and work shifts.
type Repository interface {
	CreateEmployee(ctx context.Context, e *Employee) error
	GetEmployeeByID(ctx context.Context, id int64) (*Employee, error)
	ListEmployees(ctx context.Context, limit, offset int) ([]*Employee, error)

	SaveWorkTimes(ctx context.Context, times []*WorkTime) error
	ListWorkTimes(ctx context.Context) ([]*WorkTime, error)

	ListPublishedJobs(ctx context.Context, limit, offset int) ([]*JobOffer, error)
	GetJobOfferByID(ctx context.Context, id int64) (*JobOffer, error)
	CreateJobOffer(ctx context.Context, j *JobOffer) error
	UpdateJobOffer(ctx context.Context, j *JobOffer) error
	DeleteJobOffer(ctx context.Context, orgID, jobID int64) error
	ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error
	ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*JobOffer, error)
	CreateJobApplication(ctx context.Context, a *JobApplication) error
	ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error)
	CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error)
	ListApplicationsByUser(ctx context.Context, userID int64) ([]*JobApplication, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status string, notes string) error

	GetJobSeekerProfile(ctx context.Context, userID int64) (*JobSeekerProfile, error)
	UpsertJobSeekerProfile(ctx context.Context, p *JobSeekerProfile) error
}
