package workflow

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestAutomationRequestFullPipeline(t *testing.T) {
	repo := newMockWorkflowRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	svc.SetInstitutionalGate(&dummyInstGate{works: []int64{101}})
	ctx := context.Background()

	csvContent := `اسم المنتج,الكمية,سعر التنبيه,نسبة الخصم
Panadol Extra,10,85.00,20%
`
	budget := money.MustParse("1000.00")
	prefs := Priorities{
		PriorityHighestDiscount: true,
		PriorityLowestPrice:     true,
		BudgetConstraint:        &budget,
	}

	orgID := int64(10)
	created, err := svc.CreateAutomationRequest(ctx, 100, &orgID, "order_list.csv", []byte(csvContent), prefs)
	if err != nil {
		t.Fatalf("CreateAutomationRequest failed: %v", err)
	}

	if created.RequestNumber == "" {
		t.Errorf("expected request number to be set")
	}
	if created.TotalProducts != 1 {
		t.Errorf("expected 1 product line parsed, got %d", created.TotalProducts)
	}

	lat := 30.0444
	lng := 31.2357
	processed, err := svc.ProcessAutomationRequest(ctx, created.ID, 100, &lat, &lng, 50.0)
	if err != nil {
		t.Fatalf("ProcessAutomationRequest failed: %v", err)
	}

	if processed.Status != AutomationStatusApproved && processed.Status != AutomationStatusCompleted {
		t.Errorf("unexpected status: %s", processed.Status)
	}
	if processed.MatchedProducts != 1 {
		t.Errorf("expected 1 matched product, got %d", processed.MatchedProducts)
	}
	if len(processed.VendorMatches) != 1 {
		t.Fatalf("expected 1 vendor match entry, got %d", len(processed.VendorMatches))
	}

	// Verify get request by ID
	retrieved, err := svc.GetAutomationRequest(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAutomationRequest failed: %v", err)
	}
	if retrieved.ID != created.ID {
		t.Errorf("got ID %d, want %d", retrieved.ID, created.ID)
	}

	// Verify list requests
	list, err := svc.ListAutomationRequests(ctx, 100, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAutomationRequests failed: %v", err)
	}
}
