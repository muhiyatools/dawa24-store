package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// TestBranchInstitutionalWorksReplaceSemantics verifies that saveBranchInstitutionalWorksTx
// executes full replace semantics:
// 1. Assigning [A, B] sets 2 works.
// 2. Updating with [A] removes B and leaves only A.
// 3. Updating with [] removes all works and leaves 0 rows.
// 4. Using slugs resolves institutional_work_id correctly.
func TestBranchInstitutionalWorksReplaceSemantics(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	repo := postgres.NewRepository(db)
	ctx := database.AsSystem(context.Background())

	now := time.Now().UnixNano()

	// Create test institutional works
	iw1 := &org.InstitutionalWork{
		Title:       i18n.Text{"ar": "نشاط اختباري 1", "en": "Test Work 1"},
		Description: i18n.Text{"ar": "", "en": ""},
		Slug:        fmt.Sprintf("test-work-1-%d", now),
	}
	if err := repo.CreateInstitutionalWork(ctx, iw1); err != nil {
		t.Fatalf("create iw1: %v", err)
	}
	defer func() { _ = repo.DeleteInstitutionalWork(ctx, iw1.ID) }()

	iw2 := &org.InstitutionalWork{
		Title:       i18n.Text{"ar": "نشاط اختباري 2", "en": "Test Work 2"},
		Description: i18n.Text{"ar": "", "en": ""},
		Slug:        fmt.Sprintf("test-work-2-%d", now),
	}
	if err := repo.CreateInstitutionalWork(ctx, iw2); err != nil {
		t.Fatalf("create iw2: %v", err)
	}
	defer func() { _ = repo.DeleteInstitutionalWork(ctx, iw2.ID) }()

	// Create test organization
	o := &org.Organization{
		LegalName:          fmt.Sprintf("Test Replace Semantics Org %d", now),
		CommercialRegister: fmt.Sprintf("CR-RS-%d", now),
		Type:               org.TypeVendor,
		Status:             org.StatusApproved,
	}
	if err := repo.CreateOrganization(ctx, o); err != nil {
		t.Fatalf("create org: %v", err)
	}
	defer func() { _ = repo.DeleteOrganization(ctx, o.ID) }()

	// 1. Create branch with both works [iw1, iw2]
	branch := &org.Branch{
		OrganizationID:     o.ID,
		Name:               i18n.Text{"ar": "فرع استبدال", "en": "Replace Branch"},
		Code:               fmt.Sprintf("BR-%d", now),
		Address:            "Test Address",
		Status:             "active",
		WarehouseType:      "warehouse",
		InstitutionalWorks: []string{fmt.Sprintf("%d", iw1.ID), fmt.Sprintf("%d", iw2.ID)},
	}
	if err := repo.CreateBranch(ctx, branch); err != nil {
		t.Fatalf("create branch with 2 works: %v", err)
	}
	defer func() { _ = repo.DeleteBranch(ctx, branch.ID, o.ID) }()

	works, err := repo.GetBranchInstitutionalWorks(ctx, branch.ID)
	if err != nil {
		t.Fatalf("get branch works after create: %v", err)
	}
	if len(works) != 2 {
		t.Fatalf("expected 2 works after create, got %d", len(works))
	}

	// 2. Update branch with only [iw1]
	branch.InstitutionalWorks = []string{fmt.Sprintf("%d", iw1.ID)}
	if err := repo.UpdateBranch(ctx, branch); err != nil {
		t.Fatalf("update branch with 1 work: %v", err)
	}

	works, err = repo.GetBranchInstitutionalWorks(ctx, branch.ID)
	if err != nil {
		t.Fatalf("get branch works after update to 1: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("expected 1 work after update, got %d", len(works))
	}
	if works[0].ID != iw1.ID {
		t.Fatalf("expected work ID %d, got %d", iw1.ID, works[0].ID)
	}

	// 3. Update branch with empty list [] -> must remove all rows
	branch.InstitutionalWorks = []string{}
	if err := repo.UpdateBranch(ctx, branch); err != nil {
		t.Fatalf("update branch with 0 works: %v", err)
	}

	works, err = repo.GetBranchInstitutionalWorks(ctx, branch.ID)
	if err != nil {
		t.Fatalf("get branch works after update to 0: %v", err)
	}
	if len(works) != 0 {
		t.Fatalf("expected 0 works after update to empty, got %d", len(works))
	}

	// 4. Update branch with slug fallback
	branch.InstitutionalWorks = []string{iw2.Slug}
	if err := repo.UpdateBranch(ctx, branch); err != nil {
		t.Fatalf("update branch with slug: %v", err)
	}

	works, err = repo.GetBranchInstitutionalWorks(ctx, branch.ID)
	if err != nil {
		t.Fatalf("get branch works after update with slug: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("expected 1 work after update with slug, got %d", len(works))
	}
	if works[0].ID != iw2.ID {
		t.Fatalf("expected resolved work ID %d, got %d", iw2.ID, works[0].ID)
	}
}
