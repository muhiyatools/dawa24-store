package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminTempWarehouse_100Files_BulkUpload_WithMappingWizardQueue(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	// Build a multipart form with 100 distinct warehouse files (.csv and .xlsx)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	totalFiles := 100
	itemsPerFile := 15
	expectedTotalItems := totalFiles * itemsPerFile

	startGen := time.Now()
	for i := 1; i <= totalFiles; i++ {
		filename := fmt.Sprintf("مستودع_المورد_المعتمد_%03d.csv", i)
		part, err := writer.CreateFormFile("files", filename)
		if err != nil {
			t.Fatalf("failed to create form file part %d: %v", i, err)
		}

		var csvContent strings.Builder
		csvContent.WriteString("كود الصنف (SKU),اسم المنتج,سعر الجمهور,نسبة الخصم %\n")
		for j := 1; j <= itemsPerFile; j++ {
			csvContent.WriteString(fmt.Sprintf("SKU-%03d-%02d,صنف تجريبي رقم %d-%d,120.00,18.0%%\n", i, j, i, j))
		}
		_, _ = part.Write([]byte(csvContent.String()))
	}
	_ = writer.Close()
	t.Logf("Generated 100 files multipart payload in %v (size: %.2f KB)", time.Since(startGen), float64(body.Len())/1024.0)

	// 1. Send bulk upload request with AJAX/JSON header
	req := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	adminActor := authctx.Actor{
		UserID:      1,
		Email:       "admin@dawa24.com",
		Role:        "super_admin",
		IsStaff:     true,
		IsOwner:     true,
		Permissions: []string{"inventory.warehouse.update", "inventory.temp_warehouse.view"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor))

	rec := httptest.NewRecorder()
	startProc := time.Now()
	r.ServeHTTP(rec, req)
	elapsed := time.Since(startProc)

	t.Logf("Processed 100 files bulk upload in %v", elapsed)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var jsonResp struct {
		Success         bool     `json:"success"`
		TotalFiles      int      `json:"total_files"`
		SuccessfulFiles int      `json:"successful_files"`
		FailedFiles     int      `json:"failed_files"`
		TotalItems      int64    `json:"total_items"`
		UploadedIDs     []string `json:"uploaded_ids"`
		SetupQueue      string   `json:"setup_queue"`
		Message         string   `json:"message"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &jsonResp); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v, raw: %s", err, rec.Body.String())
	}

	if !jsonResp.Success {
		t.Fatalf("expected success=true, got false")
	}
	if jsonResp.TotalFiles != 100 || jsonResp.SuccessfulFiles != 100 {
		t.Errorf("expected 100 successful files, got %d", jsonResp.SuccessfulFiles)
	}
	if int(jsonResp.TotalItems) != expectedTotalItems {
		t.Errorf("expected %d total items, got %d", expectedTotalItems, jsonResp.TotalItems)
	}
	if len(jsonResp.UploadedIDs) != 100 {
		t.Errorf("expected 100 uploaded IDs in setup queue, got %d", len(jsonResp.UploadedIDs))
	}

	// 2. Test Step-by-Step Setup Experience: GET /admin/temporary-warehouses/{id}/mapping-json
	firstID := jsonResp.UploadedIDs[0]
	reqMappingJSON := httptest.NewRequest(http.MethodGet, "/admin/temporary-warehouses/"+firstID+"/mapping-json", nil)
	reqMappingJSON = reqMappingJSON.WithContext(authctx.WithActor(reqMappingJSON.Context(), adminActor))
	recMappingJSON := httptest.NewRecorder()
	r.ServeHTTP(recMappingJSON, reqMappingJSON)

	if recMappingJSON.Code != http.StatusOK {
		t.Fatalf("expected status 200 for mapping JSON, got %d", recMappingJSON.Code)
	}

	var mappingResp struct {
		Success      bool     `json:"success"`
		ID           int64    `json:"id"`
		SupplierName string   `json:"supplier_name"`
		RowCount     int      `json:"row_count"`
		Headers      []string `json:"headers"`
		CodeCol      int      `json:"code_col"`
		NameCol      int      `json:"name_col"`
		PriceCol     int      `json:"price_col"`
		DiscountCol  int      `json:"discount_col"`
	}
	if err := json.Unmarshal(recMappingJSON.Body.Bytes(), &mappingResp); err != nil {
		t.Fatalf("failed to unmarshal mapping JSON: %v", err)
	}
	if !mappingResp.Success || mappingResp.RowCount != itemsPerFile {
		t.Errorf("expected successful mapping info with %d items, got %+v", itemsPerFile, mappingResp)
	}

	// 3. Test Step Submission: POST /admin/temporary-warehouses/{id}/mapping
	formSubmit := url.Values{}
	formSubmit.Set("supplier_name", "مستودع المورد الأول المحدّث")
	formSubmit.Set("col_code", "0")
	formSubmit.Set("col_name", "1")
	formSubmit.Set("col_price", "2")
	formSubmit.Set("col_discount", "3")
	formSubmit.Set("setup_queue", strings.Join(jsonResp.UploadedIDs[1:], ","))
	formSubmit.Set("step", "1")
	formSubmit.Set("total", "100")

	reqSubmit := httptest.NewRequest(http.MethodPost, "/admin/temporary-warehouses/"+firstID+"/mapping", strings.NewReader(formSubmit.Encode()))
	reqSubmit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSubmit.Header.Set("Accept", "application/json")
	reqSubmit.Header.Set("X-Requested-With", "XMLHttpRequest")
	reqSubmit = reqSubmit.WithContext(authctx.WithActor(reqSubmit.Context(), adminActor))
	recSubmit := httptest.NewRecorder()
	r.ServeHTTP(recSubmit, reqSubmit)

	if recSubmit.Code != http.StatusOK {
		t.Fatalf("expected status 200 for mapping submit, got %d", recSubmit.Code)
	}

	var submitResp struct {
		Success        bool   `json:"success"`
		RowCount       int    `json:"row_count"`
		NextFileID     int64  `json:"next_file_id"`
		RemainingQueue string `json:"remaining_queue"`
		Step           int    `json:"step"`
	}
	if err := json.Unmarshal(recSubmit.Body.Bytes(), &submitResp); err != nil {
		t.Fatalf("failed to unmarshal mapping submit response: %v", err)
	}
	if !submitResp.Success || submitResp.Step != 2 {
		t.Errorf("expected step=2 and success=true, got %+v", submitResp)
	}
}

type mockBillingRepoForCompare struct {
	billing.Repository
	plan *billing.Plan
}

func (m *mockBillingRepoForCompare) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	return nil, nil
}

func (m *mockBillingRepoForCompare) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	return &billing.Subscription{
		ID:             1,
		OrganizationID: &orgID,
		PlanID:         m.plan.ID,
		Status:         billing.SubActive,
	}, nil
}

func (m *mockBillingRepoForCompare) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	return m.plan, nil
}

func (m *mockBillingRepoForCompare) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	return m.plan, nil
}

func TestCompareUpload_SubscriptionLimit_Enforcement(t *testing.T) {
	mockRepo := newMockBulkCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	compareSvc := compare.NewService(mockRepo, logger)

	// Create plan with limit of 2 files
	limitedPlan := &billing.Plan{
		ID:        10,
		Slug:      "silver-pharmacy",
		Features:  map[string]string{"max_compare_files": "2"},
		IsActive:  true,
		IsDefault: true,
	}
	mockBillRepo := &mockBillingRepoForCompare{plan: limitedPlan}
	billSvc := billing.NewService(mockBillRepo, logger)

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, billSvc, nil, nil, nil, nil, logger,
	)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)

	customerActor := authctx.Actor{
		UserID:         15,
		OrganizationID: 25,
		Role:           "customer",
		Permissions:    []string{"pharmacy.compare.view"},
	}

	// 1. Upload 3 files in one batch (exceeds limit 2) -> Should be blocked
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i := 1; i <= 3; i++ {
		part, _ := writer.CreateFormFile("files", fmt.Sprintf("supplier_%d.csv", i))
		_, _ = part.Write([]byte("كود,اسم,سعر,خصم\nSKU1,دواء,100,10%\n"))
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/compare/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), customerActor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 for quota block, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice_type=error") && !strings.Contains(loc, "error") {
		t.Errorf("expected error notice in redirect URL, got %s", loc)
	}

	// 2. Upload 1 file (within limit 2) -> Should succeed
	bodyOk := &bytes.Buffer{}
	writerOk := multipart.NewWriter(bodyOk)
	partOk, _ := writerOk.CreateFormFile("files", "supplier_valid.csv")
	_, _ = partOk.Write([]byte("كود,اسم,سعر,خصم\nSKU1,دواء,100,10%\n"))
	_ = writerOk.Close()

	reqOk := httptest.NewRequest(http.MethodPost, "/compare/upload", bodyOk)
	reqOk.Header.Set("Content-Type", writerOk.FormDataContentType())
	reqOk = reqOk.WithContext(authctx.WithActor(reqOk.Context(), customerActor))

	recOk := httptest.NewRecorder()
	r.ServeHTTP(recOk, reqOk)

	if recOk.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303 on success, got %d", recOk.Code)
	}
	locOk := recOk.Header().Get("Location")
	if strings.Contains(locOk, "notice_type=error") {
		t.Errorf("expected success redirect, got error notice: %s", locOk)
	}
}
