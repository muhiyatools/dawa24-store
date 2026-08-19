package org_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type institutionalMockRepo struct {
	items       map[int64]*org.InstitutionalWork
	connections map[int64]map[int64]bool
	nextID      int64
}

func newInstMockRepo() *institutionalMockRepo {
	return &institutionalMockRepo{
		items:       make(map[int64]*org.InstitutionalWork),
		connections: make(map[int64]map[int64]bool),
		nextID:      1,
	}
}

func (m *institutionalMockRepo) CreateInstitutionalWork(_ context.Context, iw *org.InstitutionalWork) error {
	iw.ID = m.nextID
	m.nextID++
	m.items[iw.ID] = iw

	m.connections[iw.ID] = make(map[int64]bool)
	for _, toID := range iw.AllowedConnections {
		m.connections[iw.ID][toID] = true
	}
	return nil
}

func (m *institutionalMockRepo) GetInstitutionalWorkByID(_ context.Context, id int64) (*org.InstitutionalWork, error) {
	iw, ok := m.items[id]
	if !ok {
		return nil, nil
	}
	return iw, nil
}

func (m *institutionalMockRepo) UpdateInstitutionalWork(_ context.Context, iw *org.InstitutionalWork) error {
	m.items[iw.ID] = iw
	m.connections[iw.ID] = make(map[int64]bool)
	for _, toID := range iw.AllowedConnections {
		m.connections[iw.ID][toID] = true
	}
	return nil
}

func (m *institutionalMockRepo) DeleteInstitutionalWork(_ context.Context, id int64) error {
	delete(m.items, id)
	delete(m.connections, id)
	return nil
}

func (m *institutionalMockRepo) ToggleInstitutionalWorkStatus(_ context.Context, id int64) error {
	if iw, ok := m.items[id]; ok {
		iw.IsActive = !iw.IsActive
	}
	return nil
}

func (m *institutionalMockRepo) ListInstitutionalWorks(_ context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	var list []*org.InstitutionalWork
	for _, iw := range m.items {
		if !onlyActive || iw.IsActive {
			list = append(list, iw)
		}
	}
	return list, nil
}

func (m *institutionalMockRepo) ListAllFlatInstitutionalWorks(_ context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return m.ListInstitutionalWorks(context.Background(), onlyActive)
}

func (m *institutionalMockRepo) CanConnectInstitutionalWorks(_ context.Context, fromID, toID int64) (bool, error) {
	if targets, ok := m.connections[fromID]; ok {
		return targets[toID], nil
	}
	return false, nil
}

func (m *institutionalMockRepo) AssignBranchInstitutionalWorks(_ context.Context, _ int64, _ []int64) error {
	return nil
}

func (m *institutionalMockRepo) GetBranchInstitutionalWorks(_ context.Context, _ int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}

// Unused organization methods for interface satisfaction
func (m *institutionalMockRepo) CreateOrganization(_ context.Context, _ *org.Organization) error { return nil }
func (m *institutionalMockRepo) GetOrganizationByID(_ context.Context, _ int64) (*org.Organization, error) { return nil, nil }
func (m *institutionalMockRepo) UpdateOrganizationStatus(_ context.Context, _ int64, _ org.OrganizationStatus) error { return nil }
func (m *institutionalMockRepo) ReviewOrganization(_ context.Context, _ int64, _ org.OrganizationStatus, _, _ string, _ int64) error { return nil }
func (m *institutionalMockRepo) UpdateOrganization(_ context.Context, _ *org.Organization) error { return nil }
func (m *institutionalMockRepo) DeleteOrganization(_ context.Context, _ int64) error { return nil }
func (m *institutionalMockRepo) ListOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus, _, _ int) ([]*org.Organization, error) { return nil, nil }
func (m *institutionalMockRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) { return 0, nil }
func (m *institutionalMockRepo) CreateBranch(_ context.Context, _ *org.Branch) error { return nil }
func (m *institutionalMockRepo) GetBranchByID(_ context.Context, _ int64) (*org.Branch, error) { return nil, nil }
func (m *institutionalMockRepo) UpdateBranch(_ context.Context, _ *org.Branch) error { return nil }
func (m *institutionalMockRepo) DeleteBranch(_ context.Context, _, _ int64) error { return nil }
func (m *institutionalMockRepo) ListBranchesByOrg(_ context.Context, _ int64) ([]*org.Branch, error) { return nil, nil }
func (m *institutionalMockRepo) UnsetMainBranches(_ context.Context, _ int64) error { return nil }
func (m *institutionalMockRepo) AssignBranchManager(_ context.Context, _, _ int64, _ *int64) error { return nil }
func (m *institutionalMockRepo) AddMember(_ context.Context, _ *org.Member) error { return nil }
func (m *institutionalMockRepo) UpdateMemberRole(_ context.Context, _, _ int64, _ string) error { return nil }
func (m *institutionalMockRepo) ListMembersByOrg(_ context.Context, _ int64) ([]*org.Member, error) { return nil, nil }
func (m *institutionalMockRepo) ListEmployees(_ context.Context, _ int64) ([]*org.EmployeeView, error) { return nil, nil }
func (m *institutionalMockRepo) RemoveMember(_ context.Context, _, _ int64) error { return nil }
func (m *institutionalMockRepo) CreateRole(_ context.Context, _ *org.Role) error { return nil }
func (m *institutionalMockRepo) ListRolesByOrg(_ context.Context, _ int64) ([]*org.Role, error) { return nil, nil }
func (m *institutionalMockRepo) GetDeliveryBands(_ context.Context, _ int64) ([]*org.DeliveryBand, error) { return nil, nil }
func (m *institutionalMockRepo) SaveDeliveryBands(_ context.Context, _ int64, _ []*org.DeliveryBand) error { return nil }
func (m *institutionalMockRepo) AddReview(_ context.Context, _ *org.Review) error { return nil }
func (m *institutionalMockRepo) AddReviewWithRatings(_ context.Context, _ *org.Review, _ []org.ReviewRating) error { return nil }
func (m *institutionalMockRepo) ListReviewsByOrg(_ context.Context, _ int64, _, _ int) ([]*org.Review, error) { return nil, nil }
func (m *institutionalMockRepo) GetReviewCriteria(_ context.Context, _ string) ([]*org.ReviewCriterion, error) { return nil, nil }
func (m *institutionalMockRepo) ReplyToReview(_ context.Context, _, _ int64, _ string, _ int64) error { return nil }
func (m *institutionalMockRepo) ToggleFollower(_ context.Context, _, _ int64) (bool, error) { return false, nil }
func (m *institutionalMockRepo) IsFollowing(_ context.Context, _, _ int64) (bool, error) { return false, nil }
func (m *institutionalMockRepo) ListFollowedOrgs(_ context.Context, _ int64) ([]*org.Organization, error) { return nil, nil }
func (m *institutionalMockRepo) CreatePolicy(_ context.Context, _ *org.Policy) error { return nil }
func (m *institutionalMockRepo) ListPoliciesByOrg(_ context.Context, _ int64) ([]*org.Policy, error) { return nil, nil }

func TestInstitutionalWorkHierarchyAndConnections(t *testing.T) {
	ctx := context.Background()
	repo := newInstMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := org.NewService(repo, logger)

	// 1. Create Root Category: Wholesale (جملة جملة)
	wholesale := &org.InstitutionalWork{
		Title:       i18n.New("جملة جملة", "Wholesale - Wholesale"),
		Description: i18n.New("كبار الموردين والمستودعات المركزية", "Primary wholesalers"),
		Icon:        "truck",
		PricingType: org.PricingSubscription,
		IsActive:    true,
		ViewType:    1,
	}
	if err := svc.CreateInstitutionalWork(ctx, wholesale); err != nil {
		t.Fatalf("Create wholesale failed: %v", err)
	}

	// 2. Create Sub-entity under Wholesale: Sector (قطاع)
	sector := &org.InstitutionalWork{
		Title:       i18n.New("قطاع", "Sector"),
		Description: i18n.New("قطاعات التوزيع الإقليمية", "Regional sectors"),
		Icon:        "layers",
		PricingType: org.PricingSubscription,
		IsActive:    true,
		ViewType:    1,
		ParentID:    &wholesale.ID,
	}
	if err := svc.CreateInstitutionalWork(ctx, sector); err != nil {
		t.Fatalf("Create sector failed: %v", err)
	}

	// 3. Create Multi-level Child under Sector: Factory (مصنع)
	factory := &org.InstitutionalWork{
		Title:              i18n.New("مصنع", "Factory"),
		Description:        i18n.New("مصانع الأدوية المعتمدة", "Pharmaceutical manufacturing plants"),
		Icon:               "package",
		PricingType:        org.PricingPaid,
		IsActive:           true,
		ViewType:           1,
		ParentID:           &sector.ID,
		AllowedConnections: []int64{wholesale.ID},
	}
	if err := svc.CreateInstitutionalWork(ctx, factory); err != nil {
		t.Fatalf("Create factory failed: %v", err)
	}

	// 4. Verify Connection: Factory can connect to Wholesale
	canConnect, err := svc.CanConnectInstitutionalWorks(ctx, factory.ID, wholesale.ID)
	if err != nil {
		t.Fatalf("CanConnectInstitutionalWorks check failed: %v", err)
	}
	if !canConnect {
		t.Errorf("expected Factory to connect to Wholesale, got false")
	}

	// Verify Factory cannot connect to non-connected ID
	canConnectSector, err := svc.CanConnectInstitutionalWorks(ctx, factory.ID, sector.ID)
	if err != nil {
		t.Fatalf("CanConnectInstitutionalWorks check failed: %v", err)
	}
	if canConnectSector {
		t.Errorf("expected Factory not to connect to Sector, got true")
	}

	// 5. Test CanConnectTo domain method
	if !factory.CanConnectTo(wholesale.ID) {
		t.Errorf("factory.CanConnectTo(wholesale.ID) returned false, want true")
	}

	// 6. Test Status Toggle
	if err := svc.ToggleInstitutionalWorkStatus(ctx, factory.ID); err != nil {
		t.Fatalf("Toggle status failed: %v", err)
	}
	if factory.IsActive {
		t.Errorf("expected factory to be inactive after toggle")
	}
}
