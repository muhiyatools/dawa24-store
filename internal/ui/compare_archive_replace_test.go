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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestCompareTool_BulkUpload_AutoArchivesOldFilesAndReplaces(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	handler.RegisterAdminRoutes(r)

	userID := int64(99)
	orgID := int64(199)
	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"compare.view", "compare.upload"},
	}

	ctx := context.Background()

	// 1. Pre-seed 3 active compare files from an earlier session
	file1 := &compare.CompareFile{
		UserID:           userID,
		OrganizationID:   &orgID,
		SupplierName:     "المتحدة للتوزيع القديمة",
		OriginalFilename: "united_old.xlsx",
		Status:           compare.FileReady,
		RowCount:         50,
		CreatedAt:        time.Now().Add(-48 * time.Hour),
		UpdatedAt:        time.Now().Add(-48 * time.Hour),
	}
	file2 := &compare.CompareFile{
		UserID:           userID,
		OrganizationID:   &orgID,
		SupplierName:     "ابن سينا القديمة",
		OriginalFilename: "ibnsina_old.xlsx",
		Status:           compare.FileReady,
		RowCount:         40,
		CreatedAt:        time.Now().Add(-24 * time.Hour),
		UpdatedAt:        time.Now().Add(-24 * time.Hour),
	}
	file3 := &compare.CompareFile{
		UserID:           userID,
		OrganizationID:   &orgID,
		SupplierName:     "فارما أوفرسيز القديمة",
		OriginalFilename: "overseas_old.xlsx",
		Status:           compare.FileReady,
		RowCount:         30,
		CreatedAt:        time.Now().Add(-12 * time.Hour),
		UpdatedAt:        time.Now().Add(-12 * time.Hour),
	}
	_ = mockRepo.CreateFile(ctx, file1)
	_ = mockRepo.CreateFile(ctx, file2)
	_ = mockRepo.CreateFile(ctx, file3)

	activeBefore, _ := mockRepo.CountActiveFiles(ctx, userID, &orgID)
	if activeBefore != 3 {
		t.Fatalf("expected 3 active files before upload, got %d", activeBefore)
	}

	// 2. Perform bulk upload of 2 new supplier files
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p1, _ := writer.CreateFormFile("compare_files", "united_new_prices.csv")
	_, _ = p1.Write([]byte("كود,اسم,سعر,خصم\n101,بنادول,50.0,15.0\n"))
	p2, _ := writer.CreateFormFile("compare_files", "ibnsina_new_prices.csv")
	_, _ = p2.Write([]byte("كود,اسم,سعر,خصم\n102,كونجستال,35.0,20.0\n"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 on upload, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") {
		t.Fatalf("expected success redirect, got %s", loc)
	}
	// Verify notice mentions auto-archiving previous files
	if !strings.Contains(loc, url.QueryEscape("تمت أرشفة")) && !strings.Contains(loc, "أرشفة") {
		t.Errorf("expected redirect message to mention auto-archiving, got %s", loc)
	}

	// 3. Verify in repository that all 3 old files are archived, NOT deleted
	f1, _ := mockRepo.GetFileByID(ctx, file1.ID)
	f2, _ := mockRepo.GetFileByID(ctx, file2.ID)
	f3, _ := mockRepo.GetFileByID(ctx, file3.ID)

	if f1.Status != compare.FileArchived || f1.ArchivedAt == nil || f1.ArchiveReason == "" {
		t.Errorf("expected file 1 to be archived with timestamp and reason, got status=%s, reason=%s", f1.Status, f1.ArchiveReason)
	}
	if f2.Status != compare.FileArchived || f2.ArchivedAt == nil || f2.ArchiveReason == "" {
		t.Errorf("expected file 2 to be archived with timestamp and reason, got status=%s, reason=%s", f2.Status, f2.ArchiveReason)
	}
	if f3.Status != compare.FileArchived || f3.ArchivedAt == nil || f3.ArchiveReason == "" {
		t.Errorf("expected file 3 to be archived with timestamp and reason, got status=%s, reason=%s", f3.Status, f3.ArchiveReason)
	}

	// Verify old files were NOT deleted (DeletedAt must be nil)
	if f1.DeletedAt != nil || f2.DeletedAt != nil || f3.DeletedAt != nil {
		t.Errorf("archived files must not be deleted from database")
	}

	// 4. Verify Super Admin can view these archived files normally
	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "superadmin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"*"},
	}

	// Super Admin GET /admin/user/temparte-warehouses?status=archived
	reqAdmin := httptest.NewRequest(http.MethodGet, "/admin/user/temparte-warehouses?status=archived", nil)
	reqAdmin = reqAdmin.WithContext(authctx.WithActor(reqAdmin.Context(), adminActor))
	recAdmin := httptest.NewRecorder()
	r.ServeHTTP(recAdmin, reqAdmin)

	if recAdmin.Code != http.StatusOK {
		t.Fatalf("expected Super Admin page to return 200 OK, got %d", recAdmin.Code)
	}
	bodyAdmin := recAdmin.Body.String()
	if !strings.Contains(bodyAdmin, "المتحدة للتوزيع القديمة") {
		t.Errorf("expected Super Admin to see archived file 'المتحدة للتوزيع القديمة'")
	}
	if !strings.Contains(bodyAdmin, "مؤرشف") {
		t.Errorf("expected Super Admin page to show 'مؤرشف' badge")
	}

	// 5. Verify Compare Tool workspace GET /compare/tool only presents the 2 active files for comparison
	reqTool := httptest.NewRequest(http.MethodGet, "/compare/tool", nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), actor))
	recTool := httptest.NewRecorder()
	r.ServeHTTP(recTool, reqTool)

	if recTool.Code != http.StatusOK {
		t.Fatalf("expected Compare Tool page to return 200 OK, got %d", recTool.Code)
	}
	bodyTool := recTool.Body.String()
	if strings.Contains(bodyTool, "name=\"supplier_ids\" value=\""+fmt.Sprintf("%d", file1.ID)+"\"") {
		t.Errorf("archived file 1 should NOT be in active comparison selection checkbox")
	}
	if strings.Contains(bodyTool, "الكشوف المؤرشفة") {
		// Archived section accordion should be present
		t.Logf("verified archived files accordion is displayed in compare tool")
	}
}
