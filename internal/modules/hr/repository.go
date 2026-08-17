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
	ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*JobOffer, error)
	CreateJobApplication(ctx context.Context, a *JobApplication) error
	ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*JobApplication, error)
}
