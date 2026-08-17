package workflow

import (
	"context"
)

// Repository defines storage operations for workflow tasks and schedules.
type Repository interface {
	CreatePriorityRequest(ctx context.Context, r *PurchasePriorityRequest) error
	GetPriorityRequestByID(ctx context.Context, id int64) (*PurchasePriorityRequest, error)

	SaveWeeklyCoverage(ctx context.Context, c *WeeklyCoverage) error
	ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*WeeklyCoverage, error)

	CreateIssue(ctx context.Context, i *ReportIssue) error
	GetIssueByID(ctx context.Context, id int64) (*ReportIssue, error)
	ListIssues(ctx context.Context, limit, offset int) ([]*ReportIssue, error)

	CreateRequest(ctx context.Context, r *Request) error
	GetRequestByID(ctx context.Context, id int64) (*Request, error)
	ListRequestsByOrg(ctx context.Context, orgID int64, status string, limit, offset int) ([]*Request, error)
	UpdateRequestStatus(ctx context.Context, id int64, status RequestStatus) error

	CreateService(ctx context.Context, s *InstitutionalService) error
	GetServiceByID(ctx context.Context, id int64) (*InstitutionalService, error)
	ListServices(ctx context.Context, parentID *int64) ([]*InstitutionalService, error)
}
