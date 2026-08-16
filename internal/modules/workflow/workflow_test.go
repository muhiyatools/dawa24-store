package workflow

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockWorkflowRepo struct {
	requests map[int64]*PurchasePriorityRequest
	coverage map[int64][]*WeeklyCoverage
	issues   map[int64]*ReportIssue
	nextID   int64
}

func newMockWorkflowRepo() *mockWorkflowRepo {
	return &mockWorkflowRepo{
		requests: map[int64]*PurchasePriorityRequest{},
		coverage: map[int64][]*WeeklyCoverage{},
		issues:   map[int64]*ReportIssue{},
		nextID:   1,
	}
}

func (m *mockWorkflowRepo) CreatePriorityRequest(_ context.Context, r *PurchasePriorityRequest) error {
	r.ID = m.nextID
	m.nextID++
	m.requests[r.ID] = r
	return nil
}

func (m *mockWorkflowRepo) GetPriorityRequestByID(_ context.Context, id int64) (*PurchasePriorityRequest, error) {
	r, ok := m.requests[id]
	if !ok {
		return nil, apperr.NotFound("request")
	}
	return r, nil
}

func (m *mockWorkflowRepo) SaveWeeklyCoverage(_ context.Context, c *WeeklyCoverage) error {
	c.ID = m.nextID
	m.nextID++
	m.coverage[c.BranchID] = append(m.coverage[c.BranchID], c)
	return nil
}

func (m *mockWorkflowRepo) ListWeeklyCoverage(_ context.Context, branchID int64) ([]*WeeklyCoverage, error) {
	return m.coverage[branchID], nil
}

func (m *mockWorkflowRepo) CreateIssue(_ context.Context, i *ReportIssue) error {
	i.ID = m.nextID
	m.nextID++
	m.issues[i.ID] = i
	return nil
}

func (m *mockWorkflowRepo) GetIssueByID(_ context.Context, id int64) (*ReportIssue, error) {
	i, ok := m.issues[id]
	if !ok {
		return nil, apperr.NotFound("issue")
	}
	return i, nil
}

func (m *mockWorkflowRepo) ListIssues(_ context.Context, limit, offset int) ([]*ReportIssue, error) {
	var list []*ReportIssue
	for _, i := range m.issues {
		list = append(list, i)
	}
	return list, nil
}

func TestWorkflowPurchasePriorityAndCoverage(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 20)
	repo := newMockWorkflowRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Priority Request
	req, err := svc.CreatePriorityRequest(ctx, 100, &PurchasePriorityRequest{
		PriorityHighestDiscount: true,
	})
	if err != nil {
		t.Fatalf("CreatePriorityRequest failed: %v", err)
	}
	if req.RequestNumber == "" || req.Status != "pending" {
		t.Errorf("unexpected request state: %+v", req)
	}

	// 2. Set Weekly Coverage
	err = svc.SetWeeklyCoverage(ctx, &WeeklyCoverage{
		BranchID:       5,
		DayOfWeek:      1, // Monday
		CoverageFrom:   "09:00",
		CoverageTo:     "17:00",
		Address:        "Nasr City Zone 1",
		DistanceMeters: 3000,
	})
	if err != nil {
		t.Fatalf("SetWeeklyCoverage failed: %v", err)
	}

	coverageList, err := svc.GetBranchCoverage(ctx, 5)
	if err != nil || len(coverageList) != 1 {
		t.Fatalf("GetBranchCoverage failed: %v", err)
	}

	// 3. Issues & Support
	issue, err := svc.ReportIssue(ctx, &ReportIssue{
		ReportedBy:  100,
		IssueType:   "damaged_item",
		Description: "One box arrived damaged",
		Priority:    "high",
	})
	if err != nil {
		t.Fatalf("ReportIssue failed: %v", err)
	}
	if issue.Status != "pending" {
		t.Errorf("got status %s, want pending", issue.Status)
	}

	issues, err := svc.ListIssues(ctx, 10, 0)
	if err != nil || len(issues) != 1 {
		t.Fatalf("ListIssues failed: %v", err)
	}
}
