package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockCompareRepoE2E struct {
	files    map[int64]*compare.CompareFile
	fileRows map[int64][]*compare.CompareFileRow
	nextID   int64
}

func newMockCompareRepoE2E() *mockCompareRepoE2E {
	return &mockCompareRepoE2E{
		files:    make(map[int64]*compare.CompareFile),
		fileRows: make(map[int64][]*compare.CompareFileRow),
		nextID:   1,
	}
}

func (m *mockCompareRepoE2E) ListPlans(ctx context.Context, onlyPublic bool) ([]*compare.Plan, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) GetPlanByID(ctx context.Context, id int64) (*compare.Plan, error) {
	return nil, apperr.NotFound("plan")
}
func (m *mockCompareRepoE2E) GetPlanBySlug(ctx context.Context, slug string) (*compare.Plan, error) {
	return nil, apperr.NotFound("plan")
}
func (m *mockCompareRepoE2E) CreatePlan(ctx context.Context, p *compare.Plan) error { return nil }
func (m *mockCompareRepoE2E) UpdatePlan(ctx context.Context, p *compare.Plan) error { return nil }
func (m *mockCompareRepoE2E) DeletePlan(ctx context.Context, id int64) error         { return nil }
func (m *mockCompareRepoE2E) ListPlanFeatures(ctx context.Context, planID int64) ([]*compare.PlanFeature, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) SetPlanFeature(ctx context.Context, feature *compare.PlanFeature) error {
	return nil
}
func (m *mockCompareRepoE2E) DeletePlanFeature(ctx context.Context, id int64) error { return nil }
func (m *mockCompareRepoE2E) CreatePlanRequest(ctx context.Context, r *compare.PlanRequest) error {
	return nil
}
func (m *mockCompareRepoE2E) GetPlanRequestByID(ctx context.Context, id int64) (*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListPendingPlanRequests(ctx context.Context) ([]*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ReviewPlanRequest(ctx context.Context, id int64, status compare.PlanRequestStatus, reviewerID int64, notes string) error {
	return nil
}
func (m *mockCompareRepoE2E) CreateSubscription(ctx context.Context, s *compare.Subscription) error {
	return nil
}
func (m *mockCompareRepoE2E) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpdateSubscription(ctx context.Context, s *compare.Subscription) error {
	return nil
}
func (m *mockCompareRepoE2E) AssignSubscriptionUser(ctx context.Context, subID, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) RevokeSubscriptionUser(ctx context.Context, subID, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) ListSubscriptionUsers(ctx context.Context, subID int64) ([]*compare.SubscriptionUser, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpsertUserSession(ctx context.Context, s *compare.UserSession) error {
	return nil
}
func (m *mockCompareRepoE2E) CountActiveUserSessions(ctx context.Context, userID int64) (int, error) {
	return 1, nil
}
func (m *mockCompareRepoE2E) EvictOldestSessions(ctx context.Context, userID int64, keep int) error {
	return nil
}
func (m *mockCompareRepoE2E) ListActiveSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) TerminateSession(ctx context.Context, sessionID string) error { return nil }
func (m *mockCompareRepoE2E) DeactivateUserSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockCompareRepoE2E) TerminateUserSessions(ctx context.Context, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	f.ID = m.nextID
	m.nextID++
	m.files[f.ID] = f
	return nil
}
func (m *mockCompareRepoE2E) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	if f, ok := m.files[id]; ok {
		return f, nil
	}
	return nil, apperr.NotFound("compare file")
}
func (m *mockCompareRepoE2E) GetFileByPublicID(ctx context.Context, pid string) (*compare.CompareFile, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		list = append(list, f)
	}
	return list, nil
}
func (m *mockCompareRepoE2E) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	return len(m.files), nil
}
func (m *mockCompareRepoE2E) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.files[f.ID] = f
	return nil
}
func (m *mockCompareRepoE2E) RenameFile(ctx context.Context, id int64, name string) error {
	if f, ok := m.files[id]; ok {
		f.SupplierName = name
	}
	return nil
}
func (m *mockCompareRepoE2E) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keep int, reason string) ([]string, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ArchiveFile(ctx context.Context, id int64, reason string) error {
	return nil
}
func (m *mockCompareRepoE2E) UnarchiveFile(ctx context.Context, id int64) error { return nil }
func (m *mockCompareRepoE2E) DeleteFile(ctx context.Context, id int64) error    { return nil }
func (m *mockCompareRepoE2E) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	for _, r := range rows {
		m.fileRows[r.FileID] = append(m.fileRows[r.FileID], r)
	}
	return nil
}
func (m *mockCompareRepoE2E) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	return m.fileRows[fileID], nil
}
func (m *mockCompareRepoE2E) DeleteFileRows(ctx context.Context, fileID int64) error {
	delete(m.fileRows, fileID)
	return nil
}
func (m *mockCompareRepoE2E) GetSubscriptionByID(ctx context.Context, id int64) (*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpdateSubscriptionStatus(ctx context.Context, id int64, status compare.SubscriptionStatus) error {
	return nil
}
func (m *mockCompareRepoE2E) RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error) {
	return true, nil
}
func (m *mockCompareRepoE2E) TouchUserSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockCompareRepoE2E) ListActiveUserSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	return nil
}
func (m *mockCompareRepoE2E) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	return nil
}
func (m *mockCompareRepoE2E) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	return nil, nil
}

func TestCompareUploadSubmit_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Prepare multipart form with CSV file
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("supplier_name", "شركة الفتح لتوزيع الأدوية")

	part, err := writer.CreateFormFile("compare_file", "prices.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	csvData := "كود الصنف,اسم الصنف الدوائي,السعر الرسمي,نسبة الخصم\n1001,بانادول اكسترا 24 قرص,45.00,18.5\n1002,اوجمنتين 1 جم 14 قرص,135.00,12.0\n"
	_, _ = part.Write([]byte(csvData))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/compare/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Attach authenticated actor
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 200,
		OrgType:        "customer",
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	h.CompareUploadSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", res.StatusCode)
	}

	loc := res.Header.Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("invalid redirect url: %v", err)
	}
	if u.Path != "/compare/tool" {
		t.Errorf("expected redirect to /compare/tool, got %s", u.Path)
	}
	if u.Query().Get("notice") != "success" {
		t.Errorf("expected success notice, got notice=%s msg=%s", u.Query().Get("notice"), u.Query().Get("msg"))
	}

	// Verify file and rows in repository
	if len(repo.files) != 1 {
		t.Fatalf("expected 1 file stored in repo, got %d", len(repo.files))
	}
	var uploadedFile *compare.CompareFile
	for _, f := range repo.files {
		uploadedFile = f
	}
	if uploadedFile.RowCount != 2 {
		t.Errorf("expected 2 rows extracted, got %d", uploadedFile.RowCount)
	}
	if uploadedFile.SupplierName != "شركة الفتح لتوزيع الأدوية" {
		t.Errorf("unexpected supplier name: %s", uploadedFile.SupplierName)
	}
	if len(repo.fileRows[uploadedFile.ID]) != 2 {
		t.Errorf("expected 2 file rows stored, got %d", len(repo.fileRows[uploadedFile.ID]))
	}

	// Test CompareToolPage renders the file and success notice
	reqTool := httptest.NewRequest("GET", "/compare/tool?"+u.RawQuery, nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), actor))
	recTool := httptest.NewRecorder()
	h.CompareToolPage(recTool, reqTool)

	toolBody := recTool.Body.String()
	if !strings.Contains(toolBody, "شركة الفتح لتوزيع الأدوية") {
		t.Errorf("expected CompareToolPage to contain supplier name, got: %s", toolBody)
	}
	if !strings.Contains(toolBody, "2 صنف جاهز") {
		t.Errorf("expected CompareToolPage to contain row count badge")
	}
}

func TestCompareSampleDownload_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	req := httptest.NewRequest("GET", "/compare/sample", nil)
	rec := httptest.NewRecorder()
	h.CompareSampleDownload(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "spreadsheetml") {
		t.Errorf("expected excel spreadsheet Content-Type, got %s", ct)
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected non-empty Excel template response body")
	}
}

func TestCompareFileMappingModal_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockCompareRepoE2E()
	compareSvc := compare.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	h.SetCompareService(compareSvc)

	// Create test file with rows
	nameCol := 1
	priceCol := 2
	file := &compare.CompareFile{
		SupplierName:     "شركة الدواء الحديث للتوزيع",
		OriginalFilename: "prices.xlsx",
		UserID:           100,
		RowCount:         3,
		Status:           compare.FileReady,
		MappingConfig: compare.MappingConfig{
			NameCol:  &nameCol,
			PriceCol: &priceCol,
		},
	}
	_ = repo.CreateFile(context.Background(), file)

	// Add sample rows to repo
	_ = repo.InsertFileRows(context.Background(), []*compare.CompareFileRow{
		{FileID: file.ID, SKU: "101", RawName: "بانادول 24 قرص", NormalizedName: "بانادول 24 قرص", Price: money.FromMinor(4500), Discount: 15.0},
		{FileID: file.ID, SKU: "102", RawName: "كونجستال 20 قرص", NormalizedName: "كونجستال 20 قرص", Price: money.FromMinor(3100), Discount: 20.0},
	})

	actor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "customer"}

	// Test GET /compare/files/{id}/mapping-modal
	reqModal := httptest.NewRequest("GET", fmt.Sprintf("/compare/files/%d/mapping-modal", file.ID), nil)
	reqModal = reqModal.WithContext(authctx.WithActor(reqModal.Context(), actor))
	recModal := httptest.NewRecorder()

	// Use chi routing context to provide URL param {id}
	rCtx := chi.NewRouteContext()
	rCtx.URLParams.Add("id", fmt.Sprintf("%d", file.ID))
	reqModal = reqModal.WithContext(context.WithValue(reqModal.Context(), chi.RouteCtxKey, rCtx))

	h.CompareFileMappingModal(recModal, reqModal)

	if recModal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for mapping modal, got %d", recModal.Code)
	}

	body := recModal.Body.String()
	if !strings.Contains(body, "شركة الدواء الحديث للتوزيع") {
		t.Errorf("expected modal to contain supplier name")
	}
	if !strings.Contains(body, "اسم الصنف الدوائي") {
		t.Errorf("expected modal to contain product name field")
	}
	if !strings.Contains(body, "حفظ وإعادة معالجة الأصناف فورياً") {
		t.Errorf("expected modal to contain submit button")
	}
}
