package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestCompare_PharmacyAccessible(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	pharmacyActor := authctx.Actor{UserID: 100, OrganizationID: 200, OrgType: "customer", Permissions: []string{"pharmacy.*"}}
	vendorActor := authctx.Actor{UserID: 101, OrganizationID: 201, OrgType: "vendor", Permissions: []string{"vendor.*"}}

	// 1. Compare Tool Page should NOT be accessible for pharmacy (redirects to dashboard)
	reqTool := httptest.NewRequest("GET", "/compare/tool", nil)
	reqTool = reqTool.WithContext(authctx.WithActor(reqTool.Context(), pharmacyActor))
	recTool := httptest.NewRecorder()
	h.CompareToolPage(recTool, reqTool)
	if recTool.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther for pharmacy on compare tool, got %d", recTool.Code)
	}

	// 2. Market Discounts Page should NOT be accessible for pharmacy (redirects to dashboard)
	reqMarket := httptest.NewRequest("GET", "/market-discounts", nil)
	reqMarket = reqMarket.WithContext(authctx.WithActor(reqMarket.Context(), pharmacyActor))
	recMarket := httptest.NewRecorder()
	h.MarketDiscountsPage(recMarket, reqMarket)
	if recMarket.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther for pharmacy on market discounts, got %d", recMarket.Code)
	}

	// 3. Compare Tool and Market Discounts should be accessible for vendor
	reqVendorTool := httptest.NewRequest("GET", "/compare/tool", nil)
	reqVendorTool = reqVendorTool.WithContext(authctx.WithActor(reqVendorTool.Context(), vendorActor))
	recVendorTool := httptest.NewRecorder()
	h.CompareToolPage(recVendorTool, reqVendorTool)
	if recVendorTool.Code != http.StatusOK {
		t.Errorf("expected 200 OK for vendor on compare tool, got %d", recVendorTool.Code)
	}
}
