package org_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockOrgRepo struct {
	orgs      map[int64]*org.Organization
	branches  map[int64][]*org.Branch
	members   map[int64][]*org.Member
	reviews   map[int64][]*org.Review
	followers map[int64]map[int64]bool
	nextID    int64
}

func newMockOrgRepo() *mockOrgRepo {
	return &mockOrgRepo{
		orgs:      map[int64]*org.Organization{},
		branches:  map[int64][]*org.Branch{},
		members:   map[int64][]*org.Member{},
		reviews:   map[int64][]*org.Review{},
		followers: map[int64]map[int64]bool{},
		nextID:    1,
	}
}

func (m *mockOrgRepo) CreateOrganization(_ context.Context, o *org.Organization) error {
	o.ID = m.nextID
	m.nextID++
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, apperr.NotFound("organization")
	}
	return o, nil
}

func (m *mockOrgRepo) UpdateOrganizationStatus(_ context.Context, id int64, status org.OrganizationStatus) error {
	o, ok := m.orgs[id]
	if !ok {
		return apperr.NotFound("organization")
	}
	o.Status = status
	return nil
}

func (m *mockOrgRepo) UpdateOrganization(_ context.Context, o *org.Organization) error {
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) DeleteOrganization(_ context.Context, id int64) error {
	if o, ok := m.orgs[id]; ok {
		o.Status = org.StatusSuspended
	}
	return nil
}

func (m *mockOrgRepo) UpdateBranch(_ context.Context, b *org.Branch) error {
	return nil
}

func (m *mockOrgRepo) DeleteBranch(_ context.Context, id, orgID int64) error {
	return nil
}

func (m *mockOrgRepo) UpdateMemberRole(_ context.Context, orgID, userID int64, role string) error {
	return nil
}

func (m *mockOrgRepo) ListOrganizations(_ context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	var list []*org.Organization
	for _, o := range m.orgs {
		if (orgType == nil || o.Type == *orgType) && (status == nil || o.Status == *status) {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrgRepo) CreateBranch(_ context.Context, b *org.Branch) error {
	b.ID = m.nextID
	m.nextID++
	m.branches[b.OrganizationID] = append(m.branches[b.OrganizationID], b)
	return nil
}

func (m *mockOrgRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	for _, bList := range m.branches {
		for _, b := range bList {
			if b.ID == id {
				return b, nil
			}
		}
	}
	return nil, apperr.NotFound("branch")
}

func (m *mockOrgRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*org.Branch, error) {
	return m.branches[orgID], nil
}

func (m *mockOrgRepo) UnsetMainBranches(_ context.Context, orgID int64) error {
	for _, b := range m.branches[orgID] {
		b.IsMain = false
	}
	return nil
}

func (m *mockOrgRepo) AddMember(_ context.Context, mem *org.Member) error {
	mem.ID = m.nextID
	m.nextID++
	m.members[mem.OrganizationID] = append(m.members[mem.OrganizationID], mem)
	return nil
}

func (m *mockOrgRepo) ListMembersByOrg(_ context.Context, orgID int64) ([]*org.Member, error) {
	return m.members[orgID], nil
}

func (m *mockOrgRepo) RemoveMember(_ context.Context, orgID, userID int64) error {
	list := m.members[orgID]
	for i, mem := range list {
		if mem.UserID == userID {
			m.members[orgID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockOrgRepo) AddReview(_ context.Context, r *org.Review) error {
	r.ID = m.nextID
	m.nextID++
	m.reviews[r.OrganizationID] = append(m.reviews[r.OrganizationID], r)
	return nil
}

func (m *mockOrgRepo) ListReviewsByOrg(_ context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	return m.reviews[orgID], nil
}

func (m *mockOrgRepo) ToggleFollower(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		m.followers[orgID] = map[int64]bool{}
	}
	curr := m.followers[orgID][userID]
	m.followers[orgID][userID] = !curr
	return !curr, nil
}

func (m *mockOrgRepo) IsFollowing(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		return false, nil
	}
	return m.followers[orgID][userID], nil
}

func (m *mockOrgRepo) CreatePolicy(_ context.Context, p *org.Policy) error {
	p.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockOrgRepo) ListPoliciesByOrg(_ context.Context, orgID int64) ([]*org.Policy, error) {
	return nil, nil
}

func TestOrganizationLifecycleAndBranches(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := org.NewService(repo, logger)

	// 1. Register
	o, err := svc.RegisterOrganization(ctx, org.RegisterOrgInput{
		LegalName:          "شركة النور للأدوية ش.م.م",
		TradeName:          i18n.New("النور فارما", "Al-Nour Pharma"),
		TaxNumber:          "123-456-789",
		CommercialRegister: "CR-998877",
		Type:               org.TypeSupplier,
		CreditLimit:        money.MustParse("100000.00"),
		PaymentTermsDays:   30,
	})
	if err != nil {
		t.Fatalf("RegisterOrganization failed: %v", err)
	}

	if o.Status != org.StatusPending {
		t.Errorf("expected StatusPending, got %s", o.Status)
	}

	// 2. Approve
	err = svc.ApproveOrganization(ctx, o.ID)
	if err != nil {
		t.Fatalf("ApproveOrganization failed: %v", err)
	}

	updated, _ := svc.GetOrganization(ctx, o.ID)
	if updated.Status != org.StatusApproved {
		t.Errorf("expected StatusApproved, got %s", updated.Status)
	}

	// 3. Create Branches and ensure single main branch
	b1 := &org.Branch{
		OrganizationID: o.ID,
		Name:           i18n.New("المخزن الرئيسي - القاهرة", "Main Warehouse - Cairo"),
		Code:           "CAI-01",
		IsMain:         true,
	}
	if err := svc.CreateBranch(ctx, b1); err != nil {
		t.Fatalf("CreateBranch b1 failed: %v", err)
	}

	b2 := &org.Branch{
		OrganizationID: o.ID,
		Name:           i18n.New("فرع الإسكندرية", "Alexandria Branch"),
		Code:           "ALX-01",
		IsMain:         true, // Should unset b1's is_main
	}
	if err := svc.CreateBranch(ctx, b2); err != nil {
		t.Fatalf("CreateBranch b2 failed: %v", err)
	}

	branches, _ := svc.ListBranches(ctx, o.ID)
	var mainCount int
	for _, b := range branches {
		if b.IsMain {
			mainCount++
		}
	}
	if mainCount != 1 {
		t.Errorf("expected exactly 1 main branch, got %d", mainCount)
	}
}
