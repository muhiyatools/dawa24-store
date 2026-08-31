package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestAdminProductImagesUploadAndMappingFlow(t *testing.T) {
	repo := newMockCatalogImageRepo()
	repo.products["PAN-500"] = &catalog.Product{
		ID:    101,
		SKU:   "PAN-500",
		Name:  i18n.Text{"ar": "بنادول إكسترا 500 مجم", "en": "Panadol Extra 500mg"},
		Price: money.MustParse("50.00"),
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	catSvc := catalog.NewService(repo, logger)
	handler := &UIHandler{log: logger, catSvc: catSvc}

	// Create a sample test image PNG in memory
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var imgBuf bytes.Buffer
	_ = png.Encode(&imgBuf, img)

	// Local HTTP server serving test image
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imgBuf.Bytes())
	}))
	defer testServer.Close()

	// 1. Upload CSV with valid product SKU and server image URL
	csvContent := fmt.Sprintf("كود الصنف,رابط الصورة\nPAN-500,%s/test.png\nUNKNOWN-999,%s/unknown.png\n", testServer.URL, testServer.URL)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "test_images.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/products/images/import/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		Role:           "superadmin",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.AdminProductImagesUploadSubmit(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("AdminProductImagesUploadSubmit status = %d; want 303", w.Code)
	}

	redirectURL := w.Header().Get("Location")
	if redirectURL == "" {
		t.Fatalf("AdminProductImagesUploadSubmit missing Location redirect")
	}

	u, _ := url.Parse(redirectURL)
	sessionID := u.Path[len("/admin/products/images/import/"):]

	sess, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		t.Fatalf("session %s not found in store", sessionID)
	}

	if sess.TotalRows != 2 {
		t.Errorf("TotalRows = %d; want 2", sess.TotalRows)
	}

	// 2. Submit column mapping & run import pipeline synchronously for test
	form := url.Values{}
	form.Set("sku_col", "0")
	form.Set("url_col", "1")
	reqMap := httptest.NewRequest("POST", "/admin/products/images/import/"+sessionID+"/mapping", strings.NewReader(form.Encode()))
	reqMap.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sessionID)
	reqMap = reqMap.WithContext(context.WithValue(reqMap.Context(), chi.RouteCtxKey, rctx))
	wMap := httptest.NewRecorder()

	handler.AdminProductImagesMappingSubmit(wMap, reqMap)
	if wMap.Code != http.StatusSeeOther {
		t.Errorf("AdminProductImagesMappingSubmit status = %d; want 303", wMap.Code)
	}

	// Wait briefly for background process
	for i := 0; i < 20; i++ {
		cur, _ := globalAdminImageImportSessionStore.GetSession(sessionID)
		if cur != nil && (cur.Phase == AdminImagePhaseCompleted || cur.Phase == AdminImagePhaseFailed) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cur, _ := globalAdminImageImportSessionStore.GetSession(sessionID)
	if cur == nil {
		t.Fatalf("session %s gone", sessionID)
	}

	if cur.Phase != AdminImagePhaseCompleted {
		t.Errorf("Phase = %v; want completed", cur.Phase)
	}
	if cur.NotFoundRows != 1 {
		t.Errorf("NotFoundRows = %d; want 1 (for UNKNOWN-999)", cur.NotFoundRows)
	}

	// No cleanup here. UploadBaseDir is the *default* relative directory, so
	// removing it deleted internal/ui/data/uploads -- tracked files included --
	// on every run. TestMain points uploads at a temporary directory and
	// removes that instead.
}
