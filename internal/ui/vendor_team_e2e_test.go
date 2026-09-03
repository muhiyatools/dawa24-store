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
	}, nil
}

func (m *mockOrgRepoTeamTest) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
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
	})

	// 2. POST /vendor/team/new adds new employee
	t.Run("POST /vendor/team/new creates employee and links to org", func(t *testing.T) {
		form := url.Values{}
		form.Set("name", "د. أحمد جمال")
		form.Set("email", "ahmed@supplier.com")
		form.Set("phone", "01099887766")
		form.Set("job_title", "مسؤول مبيعات وتوريد")
		form.Set("employee_code", "EMP-901")
		form.Set("role_key", "org_employee")
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
	})

	// 3. POST /vendor/team/new with existing email links cleanly
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

	// 4. POST /vendor/team/{id}/toggle toggles status
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

	// 5. POST /vendor/team/{id}/delete removes member
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
