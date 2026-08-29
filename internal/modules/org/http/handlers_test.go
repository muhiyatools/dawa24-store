package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("CreateOrganization")
	return nil
}
func (r stubRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	r.fail("GetOrganizationByID")
	return nil, nil
}
func (r stubRepo) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	r.fail("GetSupplierProfile")
	return nil, nil
}
func (r stubRepo) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	r.fail("UpdateSupplierProfile")
	return nil
}
func (r stubRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	r.fail("UpdateOrganizationStatus")
	return nil
}
func (r stubRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("UpdateOrganization")
	return nil
}
func (r stubRepo) DeleteOrganization(ctx context.Context, id int64) error {
	r.fail("DeleteOrganization")
	return nil
}
func (r stubRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	r.fail("ListOrganizations")
	return nil, nil
}
func (r stubRepo) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	r.fail("GetOrganizationsByIDs")
	return nil, nil
}

func (r stubRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("CreateBranch")
	return nil
}
func (r stubRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	r.fail("GetBranchByID")
	return nil, nil
}
func (r stubRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("UpdateBranch")
	return nil
}
func (r stubRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	r.fail("DeleteBranch")
	return nil
}
func (r stubRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	r.fail("ListBranchesByOrg")
	return nil, nil
}
func (r stubRepo) GetBranchesByIDs(ctx context.Context, ids []int64) ([]*org.Branch, error) {
	r.fail("GetBranchesByIDs")
	return nil, nil
}
func (r stubRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	r.fail("UnsetMainBranches")
	return nil
}

func (r stubRepo) AddMember(ctx context.Context, m *org.Member) error {
	r.fail("AddMember")
	return nil
}
func (r stubRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	r.fail("UpdateMemberRole")
	return nil
}
func (r stubRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	r.fail("ListMembersByOrg")
	return nil, nil
}
func (r stubRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	r.fail("RemoveMember")
	return nil
}

func (r stubRepo) AddReview(ctx context.Context, rev *org.Review) error {
	r.fail("AddReview")
	return nil
}
func (r stubRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	r.fail("ListReviewsByOrg")
	return nil, nil
}
func (r stubRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("ToggleFollower")
	return false, nil
}
func (r stubRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("IsFollowing")
	return false, nil
}
func (r stubRepo) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	r.fail("ListFollowedOrgs")
	return nil, nil
}
func (r stubRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	r.fail("CreatePolicy")
	return nil
}
func (r stubRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	r.fail("ListPoliciesByOrg")
	return nil, nil
}

func (r stubRepo) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	r.fail("ReviewOrganization")
	return nil
}
func (r stubRepo) CreateRole(ctx context.Context, role *org.Role) error {
	r.fail("CreateRole")
	return nil
}
func (r stubRepo) GetRole(ctx context.Context, orgID, roleID int64) (*org.Role, error) {
	r.fail("GetRole")
	return nil, nil
}

func (r stubRepo) UpdateRole(ctx context.Context, orgID int64, role *org.Role) error {
	r.fail("UpdateRole")
	return nil
}

func (r stubRepo) DeleteRole(ctx context.Context, orgID, roleID int64) error {
	r.fail("DeleteRole")
	return nil
}

func (r stubRepo) ListRoles(ctx context.Context, orgID int64) ([]*org.Role, error) {
	r.fail("ListRoles")
	return nil, nil
}

func (r stubRepo) CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error) {
	r.fail("CountRoleMembers")
	return nil, nil
}

func (r stubRepo) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	r.fail("AssignMemberRole")
	return nil
}
func (r stubRepo) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	r.fail("GetDeliveryBands")
	return nil, nil
}
func (r stubRepo) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	r.fail("SaveDeliveryBands")
	return nil
}
func (r stubRepo) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	r.fail("AddReviewWithRatings")
	return nil
}
func (r stubRepo) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	r.fail("GetReviewCriteria")
	return nil, nil
}
func (r stubRepo) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	r.fail("ReplyToReview")
	return nil
}
func (r stubRepo) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	r.fail("AssignBranchManager")
	return nil
}
func (r stubRepo) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	r.fail("ListEmployees")
	return nil, nil
}
func (r stubRepo) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	r.fail("CreateInstitutionalWork")
	return nil
}
func (r stubRepo) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	r.fail("GetInstitutionalWorkByID")
	return nil, nil
}
func (r stubRepo) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	r.fail("UpdateInstitutionalWork")
	return nil
}
func (r stubRepo) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	r.fail("DeleteInstitutionalWork")
	return nil
}
func (r stubRepo) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	r.fail("ToggleInstitutionalWorkStatus")
	return nil
}
func (r stubRepo) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	r.fail("ListInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	r.fail("ListAllFlatInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	r.fail("CanConnectInstitutionalWorks")
	return true, nil
}
func (r stubRepo) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	r.fail("AssignBranchInstitutionalWorks")
	return nil
}
func (r stubRepo) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	r.fail("GetBranchInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	r.fail("AssignEmployeeInstitutionalWork")
	return nil
}
func (r stubRepo) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	r.fail("RemoveEmployeeInstitutionalWork")
	return nil
}
func (r stubRepo) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	r.fail("ListEmployeeInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	r.fail("ListOrgEmployeeInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error) {
	r.fail("GetUserInstitutionalWorkIDs")
	return nil, nil
}
func (r stubRepo) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	r.fail("GetConnectedInstitutionalWorkIDs")
	return nil, nil
}
func (r stubRepo) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	r.fail("ToggleMemberStatus")
	return nil
}
func (r stubRepo) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	r.fail("SavePolicies")
	return nil
}
func (r stubRepo) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	r.fail("ListSocialMediaByOrg")
	return nil, nil
}
func (r stubRepo) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	r.fail("SaveSocialMedia")
	return nil
}

type happyRepo struct{}

func (happyRepo) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	return &org.SupplierOrgProfile{
		ID:     id,
		NameAr: "Happy Supplier",
		Type:   "supplier",
	}, nil
}
func (happyRepo) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	return nil
}
func (happyRepo) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	return nil
}
func (happyRepo) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	return nil, nil
}
func (happyRepo) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	return nil
}

func (happyRepo) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	return nil
}
func (happyRepo) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	return nil
}
func (happyRepo) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	return nil, nil
}
func (happyRepo) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	iw.ID = 1
	return nil
}
func (happyRepo) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return nil
}
func (happyRepo) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	return true, nil
}
func (happyRepo) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	return nil
}
func (happyRepo) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return nil
}
func (happyRepo) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	return nil
}
func (happyRepo) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}
func (happyRepo) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	return nil, nil
}

func (happyRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {

	o.ID = 1
	return nil
}
func (happyRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{ID: id, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Type: org.TypeCustomer, Status: org.StatusApproved}, nil
}
func (happyRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	return nil
}
func (happyRepo) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	return nil
}
func (happyRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	return nil
}
func (happyRepo) DeleteOrganization(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, nil
}
func (happyRepo) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	return nil, nil
}
func (happyRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	b.ID = 1
	return nil
}
func (happyRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	return &org.Branch{ID: id, OrganizationID: 1, Code: "BR-01", Name: i18n.Text{"en": "Main"}}, nil
}
func (happyRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return nil
}
func (happyRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return []*org.Branch{{ID: 1, OrganizationID: orgID, Code: "BR-01", Name: i18n.Text{"en": "Main"}}}, nil
}
func (happyRepo) GetBranchesByIDs(ctx context.Context, ids []int64) ([]*org.Branch, error) {
	return nil, nil
}
func (happyRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) AddMember(ctx context.Context, m *org.Member) error {
	m.ID = 1
	return nil
}
func (happyRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return nil
}
func (happyRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	return []*org.Member{{ID: 1, OrganizationID: orgID, UserID: 1, RoleID: 1}}, nil
}
func (happyRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return nil
}
func (happyRepo) AddReview(ctx context.Context, rev *org.Review) error {
	rev.ID = 1
	return nil
}
func (happyRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	return []*org.Review{{ID: 1, OrganizationID: orgID, UserID: 1, Rating: 5}}, nil
}
func (happyRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, nil
}
func (happyRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	p.ID = 1
	return nil
}
func (happyRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	return []*org.Policy{{ID: 1, OrganizationID: orgID, Title: "Refund Policy"}}, nil
}

func (happyRepo) CreateRole(ctx context.Context, role *org.Role) error {
	role.ID = 1
	return nil
}
func (happyRepo) GetRole(ctx context.Context, orgID, roleID int64) (*org.Role, error) {
	return &org.Role{ID: roleID, OrganizationID: orgID}, nil
}

func (happyRepo) UpdateRole(ctx context.Context, orgID int64, role *org.Role) error { return nil }

func (happyRepo) DeleteRole(ctx context.Context, orgID, roleID int64) error { return nil }

func (happyRepo) ListRoles(ctx context.Context, orgID int64) ([]*org.Role, error) { return nil, nil }

func (happyRepo) CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (happyRepo) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	return nil
}
func (happyRepo) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	return nil, nil
}
func (happyRepo) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return nil
}
func (happyRepo) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	rev.ID = 1
	return nil
}
func (happyRepo) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	return nil, nil
}
func (happyRepo) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return nil
}
func (happyRepo) CreateUserOrganization(ctx context.Context, uo *org.UserOrganization) error {
	return nil
}
func (happyRepo) GetUserOrganizationByID(ctx context.Context, id int64) (*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status org.UserOrganizationStatus, notes string) error {
	return nil
}
func (happyRepo) DeleteUserOrganization(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (happyRepo) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) CreateUserOrganization(ctx context.Context, uo *org.UserOrganization) error {
	return nil
}
func (stubRepo) GetUserOrganizationByID(ctx context.Context, id int64) (*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status org.UserOrganizationStatus, notes string) error {
	return nil
}
func (stubRepo) DeleteUserOrganization(ctx context.Context, id int64) error {
	return nil
}
func (stubRepo) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}
func (stubRepo) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*org.UserOrganization, error) {
	return nil, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo org.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "org.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/org/organizations"},
	{http.MethodGet, "/api/v1/org/organizations/1"},
	{http.MethodPut, "/api/v1/org/organizations/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1"},
	{http.MethodGet, "/api/v1/org/organizations"},
	{http.MethodPost, "/api/v1/org/organizations/1/status"},
	{http.MethodPost, "/api/v1/org/organizations/1/branches"},
	{http.MethodGet, "/api/v1/org/organizations/1/branches"},
	{http.MethodPut, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/members"},
	{http.MethodGet, "/api/v1/org/organizations/1/members"},
	{http.MethodPut, "/api/v1/org/organizations/1/members/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/members/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/reviews"},
	{http.MethodGet, "/api/v1/org/organizations/1/reviews"},
	{http.MethodPost, "/api/v1/org/organizations/1/follow"},
	{http.MethodGet, "/api/v1/admin/org/pending"},
	{http.MethodPost, "/api/v1/admin/org/1/approve"},
	{http.MethodPost, "/api/v1/admin/org/1/reject"},
	{http.MethodPost, "/api/v1/admin/org/1/suspend"},
	{http.MethodPut, "/api/v1/admin/org/1"},
	{http.MethodGet, "/api/v1/admin/org/members"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/organizations", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestOrgHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"RegisterOrg", http.MethodPost, "/api/v1/org/organizations", `{"legal_name":"Al-Amal","commercial_register":"CR-101","type":"customer","credit_limit":"1000.00","payment_terms_days":30}`, http.StatusCreated},
		{"GetOrg", http.MethodGet, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"UpdateOrg", http.MethodPut, "/api/v1/org/organizations/1", `{"legal_name":"Al-Amal Updated","commercial_register":"CR-101"}`, http.StatusOK},
		{"DeleteOrg", http.MethodDelete, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"ListOrgs", http.MethodGet, "/api/v1/org/organizations?limit=10&offset=0", "", http.StatusOK},
		{"UpdateStatus", http.MethodPost, "/api/v1/org/organizations/1/status", `{"status":"approved"}`, http.StatusOK},
		{"CreateBranch", http.MethodPost, "/api/v1/org/organizations/1/branches", `{"code":"BR-01","name":{"en":"Main Branch"}}`, http.StatusCreated},
		{"ListBranches", http.MethodGet, "/api/v1/org/organizations/1/branches", "", http.StatusOK},
		{"UpdateBranch", http.MethodPut, "/api/v1/org/organizations/1/branches/1", `{"code":"BR-01","name":{"en":"Main Branch Updated"}}`, http.StatusOK},
		{"DeleteBranch", http.MethodDelete, "/api/v1/org/organizations/1/branches/1", "", http.StatusOK},
		{"AddMember", http.MethodPost, "/api/v1/org/organizations/1/members", `{"user_id":2,"role_id":1}`, http.StatusCreated},
		{"ListMembers", http.MethodGet, "/api/v1/org/organizations/1/members", "", http.StatusOK},
		{"UpdateMemberRole", http.MethodPut, "/api/v1/org/organizations/1/members/1", `{"role":"manager"}`, http.StatusOK},
		{"RemoveMember", http.MethodDelete, "/api/v1/org/organizations/1/members/1", "", http.StatusOK},
		{"AddReview", http.MethodPost, "/api/v1/org/organizations/1/reviews", `{"rating":5,"review_text":"Great service"}`, http.StatusCreated},
		{"ListReviews", http.MethodGet, "/api/v1/org/organizations/1/reviews?limit=10&offset=0", "", http.StatusOK},
		{"ToggleFollow", http.MethodPost, "/api/v1/org/organizations/1/follow", `{"user_id":1}`, http.StatusOK},
		{"AdminPending", http.MethodGet, "/api/v1/admin/org/pending", "", http.StatusOK},
		{"AdminApprove", http.MethodPost, "/api/v1/admin/org/1/approve", "", http.StatusOK},
		{"AdminReject", http.MethodPost, "/api/v1/admin/org/1/reject", "", http.StatusOK},
		{"AdminSuspend", http.MethodPost, "/api/v1/admin/org/1/suspend", "", http.StatusOK},
		{"AdminUpdate", http.MethodPut, "/api/v1/admin/org/1", `{"legal_name":"Admin Org Name","commercial_register":"CR-101"}`, http.StatusOK},
		{"AdminMembers", http.MethodGet, "/api/v1/admin/org/members", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (r stubRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	r.fail("CountOrganizations")
	return 0, nil
}

func (r stubRepo) UpdateOrganizationAICredentials(_ context.Context, _ int64, _, _ string) error {
	r.fail("UpdateOrganizationAICredentials")
	return nil
}

func (happyRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	return 2, nil
}

func (happyRepo) UpdateOrganizationAICredentials(_ context.Context, _ int64, _, _ string) error {
	return nil
}
