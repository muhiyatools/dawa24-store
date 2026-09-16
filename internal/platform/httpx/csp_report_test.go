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
