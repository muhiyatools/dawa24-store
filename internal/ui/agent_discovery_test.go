package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestAgentRouter() http.Handler {
	r := chi.NewRouter()
	h := &UIHandler{}
	h.RegisterPublicRoutes(r)
	return r
}

func TestAgentDiscoverySitemap(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	cType := resp.Header.Get("Content-Type")
	if !strings.Contains(cType, "xml") {
		t.Errorf("expected Content-Type xml, got %s", cType)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<urlset") || !strings.Contains(bodyStr, "<loc>") {
		t.Errorf("sitemap.xml missing urlset or loc tags: %s", bodyStr)
	}
}

func TestAgentDiscoveryLinkHeaders(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		t.Fatalf("expected Link header on response, got none")
	}

	expectedRels := []string{
		`rel="api-catalog"`,
		`rel="service-desc"`,
		`rel="oauth-authorization-server"`,
		`rel="oauth-protected-resource"`,
		`rel="mcp-server-card"`,
		`rel="agent-skills"`,
	}

	for _, rel := range expectedRels {
		if !strings.Contains(linkHeader, rel) {
			t.Errorf("Link header missing relation %s: %s", rel, linkHeader)
		}
	}
}

func TestAgentDiscoveryMarkdownNegotiation(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/markdown")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	cType := resp.Header.Get("Content-Type")
	if !strings.Contains(cType, "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %s", cType)
	}

	if resp.Header.Get("X-Markdown-Tokens") == "" {
		t.Errorf("expected X-Markdown-Tokens header on markdown response")
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "# Dawa24") {
		t.Errorf("markdown response missing title: %s", string(body))
	}
}

func TestAgentDiscoveryAPICatalogRFC9727(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/api-catalog", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	cType := resp.Header.Get("Content-Type")
	if !strings.Contains(cType, "application/linkset+json") {
		t.Errorf("expected Content-Type application/linkset+json, got %s", cType)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if _, ok := data["linkset"]; !ok {
		t.Errorf("expected 'linkset' key in RFC 9727 catalog")
	}
}

func TestAgentDiscoveryOAuthAndOpenID(t *testing.T) {
	router := newTestAgentRouter()

	// 1. OpenID configuration
	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for openid-configuration, got %d", w.Result().StatusCode)
	}

	var oidc map[string]any
	_ = json.NewDecoder(w.Result().Body).Decode(&oidc)
	if oidc["token_endpoint"] == "" || oidc["authorization_endpoint"] == "" {
		t.Errorf("missing token_endpoint or authorization_endpoint in oidc discovery")
	}
	if _, ok := oidc["agent_auth"]; !ok {
		t.Errorf("missing agent_auth block in OAuth metadata")
	}

	// 2. OAuth Protected Resource
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for oauth-protected-resource, got %d", w2.Result().StatusCode)
	}

	var resMeta map[string]any
	_ = json.NewDecoder(w2.Result().Body).Decode(&resMeta)
	if resMeta["resource"] == "" || len(resMeta["authorization_servers"].([]any)) == 0 {
		t.Errorf("invalid oauth-protected-resource payload")
	}
}

func TestAgentDiscoveryAuthMD(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/auth.md", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /auth.md, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/markdown") {
		t.Errorf("expected text/markdown for /auth.md, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestAgentDiscoveryMCPServerCard(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/mcp/server-card.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for mcp server card, got %d", resp.StatusCode)
	}

	var card map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if _, ok := card["serverInfo"]; !ok {
		t.Errorf("missing serverInfo in MCP Server Card")
	}
	if _, ok := card["transport"]; !ok {
		t.Errorf("missing transport in MCP Server Card")
	}
}

func TestAgentDiscoverySkillsIndex(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-skills/index.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for agent skills index, got %d", resp.StatusCode)
	}

	var index map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	skills, ok := index["skills"].([]any)
	if !ok || len(skills) == 0 {
		t.Fatalf("expected non-empty skills array in index.json")
	}
}

func TestAgentDiscoveryAICatalogARD(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/ai-catalog.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for ai-catalog.json, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", resp.Header.Get("Access-Control-Allow-Origin"))
	}

	var catalog map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if catalog["specVersion"] != "1.0.0" {
		t.Errorf("expected specVersion 1.0.0, got %v", catalog["specVersion"])
	}
}