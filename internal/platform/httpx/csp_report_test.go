package httpx_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/stretchr/testify/assert"
)

func TestCSPReportHandler_ValidReport(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	payload := `{
		"csp-report": {
			"document-uri": "https://example.com/checkout",
			"referrer": "",
			"violated-directive": "script-src-elem",
			"effective-directive": "script-src-elem",
			"original-policy": "default-src 'none'; script-src 'self'",
			"disposition": "enforce",
			"blocked-uri": "https://evil.com/xss.js",
			"line-number": 42,
			"source-file": "https://example.com/checkout",
			"status-code": 200,
			"script-sample": ""
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/csp-report", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReportHandler_MethodNotAllowed(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/csp-report", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestCSPReportHandler_PayloadTooLarge(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httpx.CSPReportHandler(log)

	hugePayload := `{"csp-report":{"document-uri":"` + strings.Repeat("A", 35*1024) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/csp-report", bytes.NewBufferString(hugePayload))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSecurityHeaders_CSPHardenedDirectives(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := httpx.SecurityHeaders(inner)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	assert.Equal(t, `default="/api/v1/csp-report"`, res.Header.Get("Reporting-Endpoints"))
	assert.Empty(t, res.Header.Get("Report-To"))
	assert.Equal(t, "none", res.Header.Get("X-Permitted-Cross-Domain-Policies"))
	assert.Equal(t, "?1", res.Header.Get("Origin-Agent-Cluster"))

	csp := res.Header.Get("Content-Security-Policy")
	assert.NotEmpty(t, csp)

	// Verify reporting directives
	assert.Contains(t, csp, "report-to default")
	assert.Contains(t, csp, "report-uri /api/v1/csp-report")

	// Verify script hash auditing & report samples
	assert.Contains(t, csp, "'report-sample'")
	assert.Contains(t, csp, "'report-sha256'")

	// Verify deprecated child-src is removed
	assert.NotContains(t, csp, "child-src")

	// Verify img-src does NOT allow arbitrary bare https: scheme or wildcard subdomains
	assert.NotContains(t, csp, " https:;")
	assert.NotContains(t, csp, " https: ")
	assert.NotContains(t, csp, "https://*.tile")
	assert.Contains(t, csp, "img-src 'self' data: blob: https://a.tile.openstreetmap.org")

	// Verify frame-src does NOT use subdomain wildcards
	assert.NotContains(t, csp, "https://*.google.com")
	assert.Contains(t, csp, "frame-src 'self' https://www.google.com https://maps.google.com https://www.openstreetmap.org")
}
