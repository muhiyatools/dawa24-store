package ui_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminOrgImport_SavingWorkflow_ResumableSteps(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	adminActor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "superadmin",
		Permissions: []string{"catalog.org_import.view", "catalog.org_import.run"},
	}

	validXLSX := makeTestXLSX(t, [][]any{
		{"اسم الدواء", "الباركود", "السعر", "الكمية"},
		{"بنادول اكسترا 500 ملغ", "6281001", 15.5, 100},
		{"أدول 500 ملغ أقراص", "6281002", 12.0, 50},
	})

	targetOrgID := int64(42)

	// Step 1: Upload spreadsheet for target organization 42
	uploadRec := doMultipartPOST(t, r, fmt.Sprintf("/admin/organizations/import/%d/saving/upload", targetOrgID), "file", "saving_inventory.xlsx", validXLSX, map[string]string{
		"match_choice": "fuzzy",
	}, adminActor)

	require.Equal(t, http.StatusSeeOther, uploadRec.Code)
	redirectLoc := uploadRec.Header().Get("Location")
	require.Contains(t, redirectLoc, fmt.Sprintf("/admin/organizations/import/%d/runs/", targetOrgID))
	require.Contains(t, redirectLoc, "/mapping")

	parts := strings.Split(redirectLoc, "/")
	var runID string
	for i, part := range parts {
		if part == "runs" && i+1 < len(parts) {
			runID = parts[i+1]
			break
		}
	}
	require.NotEmpty(t, runID, "runID must be extracted from redirect URL")

	// Step 2: GET mapping page - persistent URL
	mapGetRec := doGET(t, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetOrgID, runID), adminActor)
	assert.Equal(t, http.StatusOK, mapGetRec.Code)
	assert.Contains(t, mapGetRec.Body.String(), "اسم الدواء")

	// Verify page refresh loses nothing (resumable URL)
	mapRefreshRec := doGET(t, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetOrgID, runID), adminActor)
	assert.Equal(t, http.StatusOK, mapRefreshRec.Code)

	// Step 3: POST column mapping
	form := url.Values{}
	form.Set("col_name", "اسم الدواء")
	form.Set("col_sku", "الباركود")
	form.Set("col_price", "السعر")
	form.Set("col_qty", "الكمية")
	form.Set("match_choice", "fuzzy")

	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetOrgID, runID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor))
	mapPostRec := httptest.NewRecorder()
	r.ServeHTTP(mapPostRec, req)

	require.Equal(t, http.StatusSeeOther, mapPostRec.Code)
	reviewLoc := mapPostRec.Header().Get("Location")
	assert.Equal(t, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", targetOrgID, runID), reviewLoc)

	// Step 4: GET review page
	revGetRec := doGET(t, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", targetOrgID, runID), adminActor)
	assert.Equal(t, http.StatusOK, revGetRec.Code)
	assert.Contains(t, revGetRec.Body.String(), "بنادول اكسترا 500 ملغ")

	// Verify review page refresh loses nothing
	revRefreshRec := doGET(t, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", targetOrgID, runID), adminActor)
	assert.Equal(t, http.StatusOK, revRefreshRec.Code)

	// Step 5: Backwards-compatible session redirect
	aliasRec := doGET(t, r, fmt.Sprintf("/admin/organizations/import/saving/%s", runID), adminActor)
	assert.Equal(t, http.StatusSeeOther, aliasRec.Code)
	assert.Equal(t, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/review", targetOrgID, runID), aliasRec.Header().Get("Location"))
}

func TestAdminOrgImport_TempWarehouse_MultiFileStreaming(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	adminActor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "superadmin",
		Permissions: []string{"catalog.org_import.run"},
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add target organization parameter
	_ = writer.WriteField("org_id", "55")
	_ = writer.WriteField("supplier_name", "مستودع الأمل التجريبي")

	// Stream multiple valid CSV files (simulating batch upload)
	for i := 1; i <= 5; i++ {
		filename := fmt.Sprintf("warehouse_batch_%d.csv", i)
		part, err := writer.CreateFormFile("files", filename)
		require.NoError(t, err)
		csvContent := fmt.Sprintf("كود الصنف,اسم الصنف,السعر,الكمية\nSKU-%d,صنف تجريبي رقم %d,25.5,100\n", i, i)
		_, err = part.Write([]byte(csvContent))
		require.NoError(t, err)
	}
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/organizations/import/temp-warehouse/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(authctx.WithActor(req.Context(), adminActor))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// When compareSvc is nil, it safely redirects with notice without crashing or memory blowup
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/admin/organizations/import")
}

func TestAdminOrgImport_UnauthorizedAccess(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(h)

	unauthActor := authctx.Actor{
		UserID:         99,
		OrganizationID: 99,
		OrgID:          99,
		Role:           "staff",
		Permissions:    []string{"orders.view"}, // missing catalog.org_import.*
	}

	rec := doGET(t, r, "/admin/organizations/import", unauthActor)
	assert.True(t, rec.Code == http.StatusForbidden || rec.Code == http.StatusSeeOther)

	recUpload := doMultipartPOST(t, r, "/admin/organizations/import/42/saving/upload", "file", "test.xlsx", []byte("bad"), nil, unauthActor)
	assert.True(t, recUpload.Code == http.StatusForbidden || recUpload.Code == http.StatusSeeOther)
}
