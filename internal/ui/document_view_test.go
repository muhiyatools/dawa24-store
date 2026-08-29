package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestDocumentViewHandler_SanitizesLocalhostAndShowsCleanError(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a dummy organization and document with a localhost URL
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية الاختبار للمستندات"}', 'صيدلية الاختبار للمستندات', '{"ar":"صيدلية الاختبار"}', 'TAX-DOC-123', 'CR-DOC-123', 'customer', 'pending')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert test org: %v", err)
	}

	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	var docID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO platform_admin.documents (organization_id, document_type, title, file_url, storage_key, original_name, status, mime_type)
		VALUES ($1, 'commercial_register', 'السجل التجاري', 'http://localhost:9000/dawa24//uploads/documents/2026/02/test_cr.pdf', 'uploads/documents/2026/02/test_cr.pdf', 'test_cr.pdf', 'pending', 'application/pdf')
		RETURNING id
	`, orgID).Scan(&docID)
	if err != nil {
		t.Fatalf("failed to insert test document: %v", err)
	}

	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM platform_admin.documents WHERE id = $1`, docID)
	}()

	// 2. Perform GET /documents/{id}/view as admin
	docIDStr := fmt.Sprintf("%d", docID)
	req := httptest.NewRequest("GET", "/documents/"+docIDStr+"/view", nil)
	// Build chi URL params context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docIDStr)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Attach Admin actor
	actor := authctx.Actor{
		UserID:  1,
		Email:   "admin@dawa24.com",
		Role:    "admin",
		IsStaff: true,
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify that we NEVER redirect to localhost:9000
	location := rec.Header().Get("Location")
	if strings.Contains(location, "localhost:9000") || strings.Contains(location, "127.0.0.1:9000") {
		t.Fatalf("SECURITY/UX DEFECT: Response redirected to unreachable localhost URL: %s", location)
	}

	// If the file is not on disk/storage, it should return 404 with the clear DocumentUnavailablePage
	if rec.Code == http.StatusNotFound {
		body := rec.Body.String()
		if !strings.Contains(body, "تعذر فتح المستند") {
			t.Errorf("Expected DocumentUnavailablePage body to contain Arabic error title, got: %s", body)
		}
		if !strings.Contains(body, "السجل التجاري") {
			t.Errorf("Expected DocumentUnavailablePage body to contain document type label, got: %s", body)
		}
		if !strings.Contains(body, "test_cr.pdf") {
			t.Errorf("Expected DocumentUnavailablePage body to contain original filename, got: %s", body)
		}
	}
}
