package workflow_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockWorkflowRepo struct {
	requests map[int64]*workflow.PurchasePriorityRequest
	coverage map[int64][]*workflow.WeeklyCoverage
	issues   map[int64]*workflow.ReportIssue
	nextID   int64
}

func newMockWorkflowRepo() *mockWorkflowRepo {
	return &mockWorkflowRepo{
		requests: map[int64]*workflow.PurchasePriorityRequest{},
		coverage: map[int64][]*workflow.WeeklyCoverage{},
		issues:   map[int64]*workflow.ReportIssue{},
		nextID:   1,
	}
}

func (m *mockWorkflowRepo) CreatePriorityRequest(_ context.Context, r *workflow.PurchasePriorityRequest) error {
	r.ID = m.nextID
	m.nextID++
	m.requests[r.ID] = r
	return nil
}

func (m *mockWorkflowRepo) GetPriorityRequestByID(_ context.Context, id int64) (*workflow.PurchasePriorityRequest, error) {
	r, ok := m.requests[id]
	if !ok {
		return nil, apperr.NotFound("request")
	}
	return r, nil
}

func (m *mockWorkflowRepo) SaveWeeklyCoverage(_ context.Context, c *workflow.WeeklyCoverage) error {
	c.ID = m.nextID
	m.nextID++
	m.coverage[c.BranchID] = append(m.coverage[c.BranchID], c)
	return nil
}

func (m *mockWorkflowRepo) ListWeeklyCoverage(_ context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	return m.coverage[branchID], nil
}

func (m *mockWorkflowRepo) CreateIssue(_ context.Context, i *workflow.ReportIssue) error {
	i.ID = m.nextID
	m.nextID++
	m.issues[i.ID] = i
	return nil
}

func (m *mockWorkflowRepo) GetIssueByID(_ context.Context, id int64) (*workflow.ReportIssue, error) {
	i, ok := m.issues[id]
	if !ok {
		return nil, apperr.NotFound("issue")
	}
	return i, nil
}

func (m *mockWorkflowRepo) ListIssues(_ context.Context, limit, offset int) ([]*workflow.ReportIssue, error) {
	var list []*workflow.ReportIssue
	for _, i := range m.issues {
		list = append(list, i)
	}
	return list, nil
}

func TestWorkflowPurchasePriorityAndCoverage(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 20)
	repo := newMockWorkflowRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := workflow.NewService(repo, logger)

	// 1. Create Priority Request
	req, err := svc.CreatePriorityRequest(ctx, 100, &workflow.PurchasePriorityRequest{
		PriorityHighestDiscount: true,
	})
	if err != nil {
		t.Fatalf("CreatePriorityRequest failed: %v", err)
	}
	if req.RequestNumber == "" || req.Status != "pending" {
		t.Errorf("unexpected request state: %+v", req)
	}

	// 2. Set Weekly Coverage
	err = svc.SetWeeklyCoverage(ctx, &workflow.WeeklyCoverage{
		BranchID:       5,
		DayOfWeek:      1, // Monday
		CoverageFrom:   "09:00",
		CoverageTo:     "17:00",
		Address:        "Maadi, Cairo",
		DistanceMeters: 5000,
	})
	if err != nil {
		t.Fatalf("SetWeeklyCoverage failed: %v", err)
	}

	cov, _ := svc.GetBranchCoverage(ctx, 5)
	if len(cov) != 1 || cov[0].Address != "Maadi, Cairo" {
		t.Errorf("unexpected coverage data: %+v", cov)
	}
}
