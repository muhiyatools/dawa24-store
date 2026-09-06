package ui_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockIdentityRepoTeamTest struct {
	identity.Repository
	users  map[string]*identity.User
	byID   map[int64]*identity.User
	nextID int64
}

func newMockIdentityRepoTeamTest() *mockIdentityRepoTeamTest {
	return &mockIdentityRepoTeamTest{
		users:  make(map[string]*identity.User),
		byID:   make(map[int64]*identity.User),
		nextID: 100,
	}
}

func (m *mockIdentityRepoTeamTest) CreateUser(ctx context.Context, u *identity.User) error {
	m.nextID++
	u.ID = m.nextID
	m.users[strings.ToLower(u.Email)] = u
	m.byID[u.ID] = u
	return nil
}

func (m *mockIdentityRepoTeamTest) CreateSecurity(ctx context.Context, s *identity.UserSecurity) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) GetPermissionsForUser(ctx context.Context, userID int64, orgID int64) ([]string, error) {
	return []string{"user"}, nil
}

func (m *mockIdentityRepoTeamTest) CreateSession(ctx context.Context, s *identity.Session) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	u, ok := m.users[strings.ToLower(email)]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockIdentityRepoTeamTest) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	u, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockIdentityRepoTeamTest) UpdateUser(ctx context.Context, u *identity.User) error {
	m.users[strings.ToLower(u.Email)] = u
	m.byID[u.ID] = u
	return nil
}

type mockOrgRepoTeamTest struct {
	org.Repository
	members  map[int64]*org.Member
	branches []*org.Branch
	nextMID  int64
}

func newMockOrgRepoTeamTest() *mockOrgRepoTeamTest {
	return &mockOrgRepoTeamTest{
		members: make(map[int64]*org.Member),
		branches: []*org.Branch{
			{ID: 1, Name: i18n.New("مستودع القاهرة الرئيسي", "Main Warehouse")},
		},
		nextMID: 10,
	}
}

func (m *mockOrgRepoTeamTest) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return m.branches, nil
}

func (m *mockOrgRepoTeamTest) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	var list []*org.Member
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			list = append(list, mem)
		}
	}
	return list, nil
}

func (m *mockOrgRepoTeamTest) AddMember(ctx context.Context, mem *org.Member) error {
	for _, existing := range m.members {
		if existing.OrganizationID == mem.OrganizationID && existing.UserID == mem.UserID {
			existing.BranchID = mem.BranchID
			existing.RoleID = mem.RoleID
			existing.OrgRoleID = mem.OrgRoleID
			existing.RoleKey = mem.RoleKey
			existing.EmployeeCode = mem.EmployeeCode
			existing.JobTitle = mem.JobTitle
			existing.IsActive = mem.IsActive
			mem.ID = existing.ID
			return nil
		}
	}
	m.nextMID++
	mem.ID = m.nextMID
	m.members[mem.ID] = mem
	return nil
}

func (m *mockOrgRepoTeamTest) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	var views []*org.EmployeeView
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			views = append(views, &org.EmployeeView{
				Member:     mem,
				UserName:   "د. أحمد جمال",
				UserEmail:  "ahmed@supplier.com",
				UserPhone:  "01099887766",
				UserStatus: "active",
				RoleName:   "موظف مبيعات وتوريد",
				BranchName: "مستودع القاهرة الرئيسي",
			})
		}
	}
	return views, nil
}

func (m *mockOrgRepoTeamTest) ListEmployeesWithTotal(ctx context.Context, orgID int64, _, _ int) ([]*org.EmployeeView, int, error) {
	views, err := m.ListEmployees(ctx, orgID)
	return views, len(views), err
}

func (m *mockOrgRepoTeamTest) ListBranchesWithTotal(_ context.Context, _ org.BranchFilter, _, _ int) ([]*org.Branch, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepoTeamTest) AdminBranchStats(_ context.Context) (org.AdminBranchStatsResult, error) {
	return org.AdminBranchStatsResult{}, nil
}

func (m *mockOrgRepoTeamTest) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		mem.IsActive = !mem.IsActive
	}
	return nil
}

func (m *mockOrgRepoTeamTest) GetMemberByID(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		return mem, nil
	}
	return nil, nil
}

func (m *mockOrgRepoTeamTest) RemoveMember(ctx context.Context, orgID, userID int64) error {
	// Mirrors the real repository: deletion is by organization + user id.
	for id, mem := range m.members {
		if mem.OrganizationID == orgID && mem.UserID == userID {
			delete(m.members, id)
		}
	}
	return nil
}

// The team page offers a role selector now, so it reads the company's roles.
// The mock embeds org.Repository as a nil interface, which panics on any
// method it does not implement — these two are what the page asks for.
func (m *mockOrgRepoTeamTest) ListRoles(_ context.Context, orgID int64) ([]*org.Role, error) {
	return []*org.Role{
		{ID: 1, OrganizationID: orgID, Key: "org_owner", Name: i18n.New("مالك المنشأة", "Owner"), IsSystem: true, IsOwner: true},
		{ID: 2, OrganizationID: orgID, Key: "org_manager", Name: i18n.New("مدير", "Manager"), IsSystem: true},
		{ID: 3, OrganizationID: orgID, Key: "org_warehouse", Name: i18n.New("أمين مخزن", "Warehouse"), IsSystem: true},
		{ID: 4, OrganizationID: orgID, Key: "org_employee", Name: i18n.New("موظف مبيعات وتوريد", "Sales Rep"), IsSystem: true},
	}, nil
}

func (m *mockOrgRepoTeamTest) AssignBranchManager(_ context.Context, orgID, branchID int64, managerUserID *int64) error {
	return nil
}

func (m *mockOrgRepoTeamTest) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockOrgRepoTeamTest) AssignMemberRole(_ context.Context, orgID, memberID, roleID int64) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		mem.RoleID = roleID
		mem.OrgRoleID = &roleID
		for _, r := range []*org.Role{
			{ID: 1, Key: "org_owner"},
			{ID: 2, Key: "org_manager"},
			{ID: 3, Key: "org_warehouse"},
			{ID: 4, Key: "org_employee"},
		} {
			if r.ID == roleID {
				mem.RoleKey = r.Key
				break
			}
		}
	}
	return nil
}

func (m *mockOrgRepoTeamTest) UpdateMember(_ context.Context, orgID, memberID int64, patch org.MemberPatch) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		if patch.JobTitle != nil {
			mem.JobTitle = *patch.JobTitle
		}
		if patch.EmployeeCode != nil {
			mem.EmployeeCode = *patch.EmployeeCode
		}
		if patch.BranchID != nil {
			mem.BranchID = patch.BranchID
		}
		if patch.RoleKey != nil {
			mem.RoleKey = *patch.RoleKey
		}
		if patch.OrgRoleID != nil {
			mem.OrgRoleID = patch.OrgRoleID
			mem.RoleID = *patch.OrgRoleID
		}
		if patch.IsActive != nil {
			mem.IsActive = *patch.IsActive
		}
	}
	return nil
}

func TestVendorTeam_CompleteOverhaul_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	idRepo := newMockIdentityRepoTeamTest()
	idSvc := identity.NewService(idRepo, nil, logger)
	orgRepo := newMockOrgRepoTeamTest()
	orgSvc := org.NewService(orgRepo, logger)

	h := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	h.RegisterVendorRoutes(r)

	vendorActor := authctx.Actor{UserID: 1, OrganizationID: 55, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// 1. GET /vendor/team renders page
	t.Run("GET /vendor/team renders 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/team", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "فريق العمل وصلاحيات الموظفين") {
			t.Errorf("expected body to contain 'فريق العمل وصلاحيات الموظفين'")
		}
		if !strings.Contains(body, "مستودع القاهرة الرئيسي") {
			t.Errorf("expected branch option in select dropdown")
		}
		// Verify dynamic role options are rendered from the database
		if !strings.Contains(body, `value="2"`) || !strings.Contains(body, "مدير") {
			t.Errorf("expected dynamic role 'مدير' (ID: 2) to be in rendered page options")
		}
		if !strings.Contains(body, `value="3"`) || !strings.Contains(body, "أمين مخزن") {
			t.Errorf("expected dynamic role 'أمين مخزن' (ID: 3) to be in rendered page options")
		}
		if !strings.Contains(body, `value="4"`) || !strings.Contains(body, "موظف مبيعات وتوريد") {
			t.Errorf("expected dynamic role 'موظف مبيعات وتوريد' (ID: 4) to be in rendered page options")
		}
	})

	// 2. POST /vendor/team/new adds new employee with dynamic role_id
	t.Run("POST /vendor/team/new creates employee and links to org with dynamic role_id", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "د. أحمد جمال")
		form.Set("email", "ahmed@supplier.com")
		form.Set("phone", "01099887766")
		form.Set("job_title", "مسؤول مبيعات وتوريد")
		form.Set("employee_code", "EMP-901")
		form.Set("role_id", "3") // Dynamic role from DB: أمين مخزن (org_warehouse)
		form.Set("password", "SecurePassword123!")
		form.Set("branch_id", "1")

		req := httptest.NewRequest("POST", "/vendor/team/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		if len(orgRepo.members) == 0 {
			t.Fatalf("expected 1 member in org, got 0")
		}

		var createdMember *org.Member
		for _, m := range orgRepo.members {
			createdMember = m
			break
		}

		if createdMember.JobTitle != "مسؤول مبيعات وتوريد" {
			t.Errorf("expected JobTitle 'مسؤول مبيعات وتوريد', got %s", createdMember.JobTitle)
		}
		if createdMember.EmployeeCode != "EMP-901" {
			t.Errorf("expected EmployeeCode 'EMP-901', got %s", createdMember.EmployeeCode)
		}
		if createdMember.BranchID == nil || *createdMember.BranchID != 1 {
			t.Errorf("expected BranchID 1, got %v", createdMember.BranchID)
		}
		if createdMember.RoleID != 3 {
			t.Errorf("expected RoleID 3, got %d", createdMember.RoleID)
		}
		if createdMember.OrgRoleID == nil || *createdMember.OrgRoleID != 3 {
			t.Errorf("expected OrgRoleID 3, got %v", createdMember.OrgRoleID)
		}
		if createdMember.RoleKey != "org_warehouse" {
			t.Errorf("expected RoleKey 'org_warehouse', got %s", createdMember.RoleKey)
		}
	})

	// 3. POST /vendor/team/{id}/edit updates employee with new dynamic role_id
	t.Run("POST /vendor/team/{id}/edit updates employee role to manager", func(t *testing.T) {
		var memberID int64
		for id := range orgRepo.members {
			memberID = id
			break
		}

		editForm := url.Values{}
		editForm.Set("name", "د. أحمد جمال بعد الترقية")
		editForm.Set("phone", "01099887766")
		editForm.Set("job_title", "مدير العمليات")
		editForm.Set("employee_code", "EMP-901")
		editForm.Set("role_id", "2") // Promote to: مدير (org_manager)
		editForm.Set("branch_id", "1")
		editForm.Set("is_active", "true")

		req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/edit", memberID), strings.NewReader(editForm.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		updated := orgRepo.members[memberID]
		if updated.RoleID != 2 {
			t.Errorf("expected updated RoleID 2, got %d", updated.RoleID)
		}
		if updated.OrgRoleID == nil || *updated.OrgRoleID != 2 {
			t.Errorf("expected updated OrgRoleID 2, got %v", updated.OrgRoleID)
		}
		if updated.RoleKey != "org_manager" {
			t.Errorf("expected updated RoleKey 'org_manager', got %s", updated.RoleKey)
		}
	})

	// 4. POST /vendor/team/new with existing email links cleanly
	t.Run("POST /vendor/team/new with existing email links existing user", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "د. أحمد جمال المحدث")
		form.Set("email", "ahmed@supplier.com") // Already exists
		form.Set("phone", "01099887766")
		form.Set("job_title", "مدير عمليات")
		form.Set("role_key", "org_manager")
		form.Set("password", "SecurePassword123!")

		req := httptest.NewRequest("POST", "/vendor/team/new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
	})

	// 5. POST /vendor/team/{id}/toggle toggles status
	t.Run("POST /vendor/team/{id}/toggle toggles status", func(t *testing.T) {
		var memberID int64
		for id := range orgRepo.members {
			memberID = id
			break
		}

		req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/toggle", memberID), nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
	})

	// 6. POST /vendor/team/{id}/delete removes member
	t.Run("POST /vendor/team/{id}/delete removes member", func(t *testing.T) {
		var memberIDs []int64
		for id := range orgRepo.members {
			memberIDs = append(memberIDs, id)
		}

		for _, id := range memberIDs {
			req := httptest.NewRequest("POST", fmt.Sprintf("/vendor/team/%d/delete", id), nil)
			req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("expected 303 redirect, got %d", rec.Code)
			}
		}

		if len(orgRepo.members) != 0 {
			t.Errorf("expected 0 members remaining, got %d", len(orgRepo.members))
		}
	})
}
