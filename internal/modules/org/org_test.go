package org

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockOrgRepo struct {
	orgs          map[int64]*Organization
	branches      map[int64][]*Branch
	members       map[int64][]*Member
	reviews       map[int64][]*Review
	policies      map[int64][]*Policy
	followers     map[int64]map[int64]bool
	deliveryBands map[int64][]*DeliveryBand
	roles         []*Role
	nextID        int64
}

func newMockOrgRepo() *mockOrgRepo {
	return &mockOrgRepo{
		orgs:          map[int64]*Organization{},
		branches:      map[int64][]*Branch{},
		members:       map[int64][]*Member{},
		reviews:       map[int64][]*Review{},
		policies:      map[int64][]*Policy{},
		followers:     map[int64]map[int64]bool{},
		deliveryBands: map[int64][]*DeliveryBand{},
		nextID:        1,
	}
}

func (m *mockOrgRepo) CreateOrganization(_ context.Context, o *Organization) error {
	o.ID = m.nextID
	m.nextID++
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*Organization, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, nil
	}
	return o, nil
}

func (m *mockOrgRepo) GetSupplierProfile(_ context.Context, id int64) (*SupplierOrgProfile, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, nil
	}
	return &SupplierOrgProfile{
		ID:                 o.ID,
		NameAr:             o.LegalName,
		NameEn:             o.LegalName,
		Type:               string(o.Type),
		MinOrderPrice:      o.MinOrderPrice,
		MaxOrderPrice:      o.MaxOrderPrice,
		OrganizationNumber: o.OrganizationNumber,
		TaxNumber:          o.TaxNumber,
		Status:             string(o.Status),
	}, nil
}

func (m *mockOrgRepo) UpdateSupplierProfile(_ context.Context, p *SupplierOrgProfile) error {
	o, ok := m.orgs[p.ID]
	if ok {
		o.LegalName = p.NameAr
		o.Type = OrganizationType(p.Type)
		o.MinOrderPrice = p.MinOrderPrice
		o.MaxOrderPrice = p.MaxOrderPrice
		o.OrganizationNumber = p.OrganizationNumber
		o.TaxNumber = p.TaxNumber
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganizationStatus(_ context.Context, id int64, status OrganizationStatus) error {
	o, ok := m.orgs[id]
	if ok {
		o.Status = status
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganizationAICredentials(_ context.Context, id int64, aiUserID, aiVirtualKey string) error {
	o, ok := m.orgs[id]
	if ok {
		o.AIUserID = aiUserID
		o.AIVirtualKey = aiVirtualKey
	}
	return nil
}

func (m *mockOrgRepo) ReviewOrganization(_ context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	o, ok := m.orgs[id]
	if ok {
		o.Status = status
		o.VerificationNotes = notes
		o.RejectionReason = rejectionReason
		o.ApprovedBy = &adminID
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganization(_ context.Context, o *Organization) error {
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) DeleteOrganization(_ context.Context, id int64) error {
	if o, ok := m.orgs[id]; ok {
		o.Status = StatusSuspended
	}
	return nil
}

func (m *mockOrgRepo) UpdateBranch(_ context.Context, b *Branch) error {
	return nil
}

func (m *mockOrgRepo) DeleteBranch(_ context.Context, id, orgID int64) error {
	return nil
}

func (m *mockOrgRepo) ListOrganizations(_ context.Context, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, error) {
	var list []*Organization
	for _, o := range m.orgs {
		if (orgType == nil || o.Type == *orgType) && (status == nil || o.Status == *status) {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrgRepo) CreateBranch(_ context.Context, b *Branch) error {
	b.ID = m.nextID
	m.nextID++
	m.branches[b.OrganizationID] = append(m.branches[b.OrganizationID], b)
	return nil
}

func (m *mockOrgRepo) GetBranchByID(_ context.Context, id int64) (*Branch, error) {
	for _, list := range m.branches {
		for _, b := range list {
			if b.ID == id {
				return b, nil
			}
		}
	}
	return nil, nil
}

func (m *mockOrgRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*Branch, error) {
	return m.branches[orgID], nil
}

func (m *mockOrgRepo) UnsetMainBranches(_ context.Context, orgID int64) error {
	for _, b := range m.branches[orgID] {
		b.IsMain = false
	}
	return nil
}

func (m *mockOrgRepo) AssignBranchManager(_ context.Context, orgID, branchID int64, managerUserID *int64) error {
	for _, b := range m.branches[orgID] {
		if b.ID == branchID {
			b.ManagerID = managerUserID
		}
	}
	return nil
}

func (m *mockOrgRepo) ListEmployees(_ context.Context, orgID int64) ([]*EmployeeView, error) {
	var list []*EmployeeView
	for _, mem := range m.members[orgID] {
		list = append(list, &EmployeeView{
			Member:    mem,
			UserName:  "Test User",
			UserEmail: "user@example.com",
		})
	}
	return list, nil
}

func (m *mockOrgRepo) AddMember(_ context.Context, mem *Member) error {

	mem.ID = m.nextID
	m.nextID++
	m.members[mem.OrganizationID] = append(m.members[mem.OrganizationID], mem)
	return nil
}

func (m *mockOrgRepo) UpdateMemberRole(_ context.Context, orgID, userID int64, role string) error {
	return nil
}

func (m *mockOrgRepo) RemoveMember(_ context.Context, orgID, userID int64) error {
	var remaining []*Member
	for _, mem := range m.members[orgID] {
		if mem.UserID != userID {
			remaining = append(remaining, mem)
		}
	}
	m.members[orgID] = remaining
	return nil
}

func (m *mockOrgRepo) ListMembersByOrg(_ context.Context, orgID int64) ([]*Member, error) {
	return m.members[orgID], nil
}

func (m *mockOrgRepo) AddReview(_ context.Context, r *Review) error {
	r.ID = m.nextID
	m.nextID++
	m.reviews[r.OrganizationID] = append(m.reviews[r.OrganizationID], r)
	return nil
}

func (m *mockOrgRepo) ListReviewsByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Review, error) {
	return m.reviews[orgID], nil
}

func (m *mockOrgRepo) ToggleFollower(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		m.followers[orgID] = map[int64]bool{}
	}
	current := m.followers[orgID][userID]
	m.followers[orgID][userID] = !current
	return !current, nil
}

func (m *mockOrgRepo) IsFollowing(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		return false, nil
	}
	return m.followers[orgID][userID], nil
}

func (m *mockOrgRepo) ListFollowedOrgs(_ context.Context, userID int64) ([]*Organization, error) {
	var list []*Organization
	for orgID, users := range m.followers {
		if users[userID] {
			if o := m.orgs[orgID]; o != nil {
				list = append(list, o)
			}
		}
	}
	return list, nil
}

func (m *mockOrgRepo) CreatePolicy(_ context.Context, p *Policy) error {
	p.ID = m.nextID
	m.nextID++
	m.policies[p.OrganizationID] = append(m.policies[p.OrganizationID], p)
	return nil
}

func (m *mockOrgRepo) ListPoliciesByOrg(_ context.Context, orgID int64) ([]*Policy, error) {
	return m.policies[orgID], nil
}

func (m *mockOrgRepo) SavePolicies(_ context.Context, orgID int64, policies []*Policy) error {
	m.policies[orgID] = policies
	return nil
}

func (m *mockOrgRepo) ListSocialMediaByOrg(_ context.Context, orgID int64) ([]*SocialMedia, error) {
	return nil, nil
}

func (m *mockOrgRepo) SaveSocialMedia(_ context.Context, orgID int64, links []*SocialMedia) error {
	return nil
}

func (m *mockOrgRepo) CreateRole(_ context.Context, role *Role) error {
	role.ID = 1
	return nil
}

func (m *mockOrgRepo) GetRole(_ context.Context, orgID, roleID int64) (*Role, error) {
	for _, r := range m.roles {
		if r.ID == roleID && r.OrganizationID == orgID {
			return r, nil
		}
	}
	return nil, apperr.NotFound("role")
}

func (m *mockOrgRepo) UpdateRole(_ context.Context, orgID int64, role *Role) error {
	for i, r := range m.roles {
		if r.ID == role.ID && r.OrganizationID == orgID {
			m.roles[i] = role
			return nil
		}
	}
	return apperr.NotFound("role")
}

func (m *mockOrgRepo) DeleteRole(_ context.Context, orgID, roleID int64) error {
	for i, r := range m.roles {
		if r.ID == roleID && r.OrganizationID == orgID {
			m.roles = append(m.roles[:i], m.roles[i+1:]...)
			return nil
		}
	}
	return apperr.NotFound("role")
}

func (m *mockOrgRepo) ListRoles(_ context.Context, orgID int64) ([]*Role, error) {
	var out []*Role
	for _, r := range m.roles {
		if r.OrganizationID == orgID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *mockOrgRepo) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockOrgRepo) AssignMemberRole(_ context.Context, _, _, _ int64) error { return nil }

func (m *mockOrgRepo) GetDeliveryBands(_ context.Context, orgID int64) ([]*DeliveryBand, error) {
	if m.deliveryBands != nil {
		return m.deliveryBands[orgID], nil
	}
	return nil, nil
}

func (m *mockOrgRepo) SaveDeliveryBands(_ context.Context, orgID int64, bands []*DeliveryBand) error {
	if m.deliveryBands == nil {
		m.deliveryBands = make(map[int64][]*DeliveryBand)
	}
	m.deliveryBands[orgID] = bands
	return nil
}

func (m *mockOrgRepo) AddReviewWithRatings(_ context.Context, r *Review, ratings []ReviewRating) error {
	r.ID = m.nextID
	m.nextID++
	r.Ratings = ratings
	m.reviews[r.OrganizationID] = append(m.reviews[r.OrganizationID], r)
	return nil
}

func (m *mockOrgRepo) GetReviewCriteria(_ context.Context, _ string) ([]*ReviewCriterion, error) {
	return nil, nil
}

func (m *mockOrgRepo) ReplyToReview(_ context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return nil
}

func (m *mockOrgRepo) CreateInstitutionalWork(_ context.Context, iw *InstitutionalWork) error {
	iw.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockOrgRepo) GetInstitutionalWorkByID(_ context.Context, id int64) (*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) UpdateInstitutionalWork(_ context.Context, _ *InstitutionalWork) error {
	return nil
}

func (m *mockOrgRepo) DeleteInstitutionalWork(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ToggleInstitutionalWorkStatus(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListInstitutionalWorks(_ context.Context, _ bool) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) ListAllFlatInstitutionalWorks(_ context.Context, _ bool) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) CanConnectInstitutionalWorks(_ context.Context, _, _ int64) (bool, error) {
	return true, nil
}

func (m *mockOrgRepo) AssignBranchInstitutionalWorks(_ context.Context, _ int64, _ []int64) error {
	return nil
}

func (m *mockOrgRepo) GetBranchInstitutionalWorks(_ context.Context, _ int64) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) AssignEmployeeInstitutionalWork(_ context.Context, _, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) RemoveEmployeeInstitutionalWork(_ context.Context, _, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListEmployeeInstitutionalWorks(_ context.Context, _ int64) ([]*EmployeeInstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) ListOrgEmployeeInstitutionalWorks(_ context.Context, _ int64) ([]*EmployeeInstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetUserInstitutionalWorkIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetConnectedInstitutionalWorkIDs(_ context.Context, _ []int64) ([]int64, error) {
	return nil, nil
}

func (m *mockOrgRepo) ToggleMemberStatus(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) CreateUserOrganization(_ context.Context, uo *UserOrganization) error {
	uo.ID = 1
	return nil
}

func (m *mockOrgRepo) GetUserOrganizationByID(_ context.Context, id int64) (*UserOrganization, error) {
	return &UserOrganization{ID: id, OrganizationNumber: "NUM1001", Status: UserOrgStatusApproved}, nil
}

func (m *mockOrgRepo) UpdateUserOrganization(_ context.Context, _ int64, _ string, _ UserOrganizationStatus, _ string) error {
	return nil
}

func (m *mockOrgRepo) DeleteUserOrganization(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListUserOrganizationsByUser(_ context.Context, _ int64) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func (m *mockOrgRepo) ListUserOrganizationsByVendor(_ context.Context, _ int64, _ string) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func (m *mockOrgRepo) ListAllUserOrganizations(_ context.Context, _ string) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func TestOrgLifecycleAndBranches(t *testing.T) {

	ctx := context.Background()
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Register Organization
	minOrder, _ := money.Parse("500.00")
	regInput := RegisterOrgInput{
		LegalName:          "Al-Amal Medical Distribution LLC",
		TradeName:          i18n.New("مخزن الأمل", "Al-Amal Warehouse"),
		TaxNumber:          "TX-883322",
		CommercialRegister: "CR-992211",
		Type:               TypeVendor,
		CreditLimit:        minOrder,
		PaymentTermsDays:   30,
	}

	createdOrg, err := svc.RegisterOrganization(ctx, regInput)
	if err != nil {
		t.Fatalf("RegisterOrganization failed: %v", err)
	}
	if createdOrg.Status != StatusPending {
		t.Errorf("got status %s, want pending", createdOrg.Status)
	}

	// 2. Admin Approve Org
	if err := svc.ApproveOrganization(ctx, createdOrg.ID); err != nil {
		t.Fatalf("ApproveOrganization failed: %v", err)
	}
	gotOrg, err := svc.GetOrganization(ctx, createdOrg.ID)
	if err != nil {
		t.Fatalf("GetOrganization failed: %v", err)
	}
	if gotOrg.Status != StatusApproved {
		t.Errorf("got status %s, want approved", gotOrg.Status)
	}

	// Reject & Suspend
	_ = svc.RejectOrganization(ctx, createdOrg.ID)
	_ = svc.SuspendOrganization(ctx, createdOrg.ID)

	// 3. Branches
	branch := &Branch{
		OrganizationID: createdOrg.ID,
		Name:           i18n.New("الفرع الرئيسي", "Main Branch"),
		Code:           "BR-001",
		Address:        "123 Nile Corniche",
		IsMain:         true,
	}
	if err := svc.CreateBranch(ctx, branch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	branches, err := svc.ListBranches(ctx, createdOrg.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if err := svc.UpdateBranch(ctx, branch); err != nil {
		t.Fatalf("UpdateBranch failed: %v", err)
	}
	if err := svc.DeleteBranch(ctx, branch.ID, createdOrg.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// 4. Members
	mem, err := svc.AddMember(ctx, createdOrg.ID, 100, 2)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	if mem.UserID != 100 {
		t.Errorf("got user id %d, want 100", mem.UserID)
	}
	members, err := svc.ListMembers(ctx, createdOrg.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if err := svc.UpdateMemberRole(ctx, createdOrg.ID, 100, "manager"); err != nil {
		t.Fatalf("UpdateMemberRole failed: %v", err)
	}
	if err := svc.RemoveMember(ctx, createdOrg.ID, 100); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	// 5. Follow & Unfollow
	following, err := svc.ToggleFollow(ctx, createdOrg.ID, 200)
	if err != nil || !following {
		t.Fatalf("ToggleFollow failed: %v", err)
	}

	// 6. Reviews
	rev, err := svc.AddReview(ctx, createdOrg.ID, 200, 5, "Fast delivery and genuine products")
	if err != nil {
		t.Fatalf("AddReview failed: %v", err)
	}
	if rev.Rating != 5 {
		t.Errorf("got rating %d, want 5", rev.Rating)
	}
	reviews, err := svc.ListReviews(ctx, createdOrg.ID, 10, 0)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("ListReviews failed: %v", err)
	}

	// 7. Update & Delete Org
	createdOrg.TaxNumber = "TX-999999"
	if err := svc.UpdateOrganization(ctx, createdOrg); err != nil {
		t.Fatalf("UpdateOrganization failed: %v", err)
	}
	if err := svc.DeleteOrganization(ctx, createdOrg.ID); err != nil {
		t.Fatalf("DeleteOrganization failed: %v", err)
	}
}

func TestUserOrganizationFlow(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Customer creates a link to vendor organization
	custOrgID := int64(10)
	uo, err := svc.CreateUserOrgLink(ctx, 42, &custOrgID, 100, "NUM10001", UserOrgStatusPending)
	if err != nil {
		t.Fatalf("CreateUserOrgLink failed: %v", err)
	}
	if uo.OrganizationNumber != "NUM10001" {
		t.Errorf("got org number %s, want NUM10001", uo.OrganizationNumber)
	}
	if uo.Status != UserOrgStatusPending {
		t.Errorf("got status %s, want pending", uo.Status)
	}

	// 2. Vendor approves the link
	if err := svc.ApproveUserOrgLink(ctx, uo.ID); err != nil {
		t.Fatalf("ApproveUserOrgLink failed: %v", err)
	}

	// 3. Vendor rejects / edits
	if err := svc.RejectUserOrgLink(ctx, uo.ID, "Invalid customer code"); err != nil {
		t.Fatalf("RejectUserOrgLink failed: %v", err)
	}

	if err := svc.UpdateUserOrgLink(ctx, uo.ID, "NUM10002", "Updated code"); err != nil {
		t.Fatalf("UpdateUserOrgLink failed: %v", err)
	}

	// 4. Delete link
	if err := svc.DeleteUserOrgLink(ctx, uo.ID); err != nil {
		t.Fatalf("DeleteUserOrgLink failed: %v", err)
	}
}

func (m *mockOrgRepo) CountOrganizations(_ context.Context, _ *OrganizationType, _ *OrganizationStatus) (int, error) {
	return len(m.orgs), nil
}

func (m *mockOrgRepo) GetOrganizationsByIDs(_ context.Context, ids []int64) ([]*Organization, error) {
	var out []*Organization
	for _, id := range ids {
		if o, ok := m.orgs[id]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m *mockOrgRepo) GetBranchesByIDs(_ context.Context, _ []int64) ([]*Branch, error) {
	return nil, nil
}

func TestCalculateDeliveryFee(t *testing.T) {
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	vendorOrgID := int64(10)

	// 1. When no bands configured -> returns zero, false
	fee, matched, err := svc.CalculateDeliveryFee(ctx, vendorOrgID, 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched || fee.IsPositive() {
		t.Errorf("expected no match and zero fee, got matched=%t fee=%s", matched, fee.String())
	}

	// 2. Configure 3 distance bands in meters:
	// Band 1: 0 - 5,000m (0-5km) -> 20.00 EGP
	// Band 2: 5,001 - 15,000m (5-15km) -> 40.00 EGP
	// Band 3: 15,001 - 30,000m (15-30km) -> 75.00 EGP
	bands := []*DeliveryBand{
		{ID: 1, OrganizationID: vendorOrgID, FromMeters: 0, ToMeters: 5000, Fee: money.FromMinor(2000), IsActive: true},
		{ID: 2, OrganizationID: vendorOrgID, FromMeters: 5001, ToMeters: 15000, Fee: money.FromMinor(4000), IsActive: true},
		{ID: 3, OrganizationID: vendorOrgID, FromMeters: 15001, ToMeters: 30000, Fee: money.FromMinor(7500), IsActive: true},
	}
	if err := svc.SaveDeliveryBands(ctx, vendorOrgID, bands); err != nil {
		t.Fatalf("failed to save delivery bands: %v", err)
	}

	// Test 3,000 meters -> falls into Band 1 (20.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 3000)
	if err != nil || !matched || fee.Minor() != 2000 {
		t.Errorf("expected 20.00 EGP for 3000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 10,000 meters -> falls into Band 2 (40.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 10000)
	if err != nil || !matched || fee.Minor() != 4000 {
		t.Errorf("expected 40.00 EGP for 10000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 25,000 meters -> falls into Band 3 (75.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 25000)
	if err != nil || !matched || fee.Minor() != 7500 {
		t.Errorf("expected 75.00 EGP for 25000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 45,000 meters (beyond max band) -> capped/matches highest tier (75.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 45000)
	if err != nil || !matched || fee.Minor() != 7500 {
		t.Errorf("expected 75.00 EGP for 45000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}
}
