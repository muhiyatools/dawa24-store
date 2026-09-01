package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func resolveBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// SitemapXML generates and returns the canonical XML sitemap per sitemaps.org protocol.
func (h *UIHandler) SitemapXML(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	now := time.Now().UTC().Format("2006-01-02")

	routes := []struct {
		path       string
		priority   string
		changefreq string
	}{
		{"/", "1.0", "daily"},
		{"/about", "0.8", "monthly"},
		{"/how-it-works", "0.8", "monthly"},
		{"/jobs", "0.7", "weekly"},
		{"/faq", "0.7", "monthly"},
		{"/contact", "0.7", "monthly"},
		{"/terms", "0.5", "monthly"},
		{"/privacy", "0.5", "monthly"},
		{"/auth/login", "0.6", "monthly"},
		{"/auth/register", "0.6", "monthly"},
		{"/docs/api", "0.8", "weekly"},
		{"/auth.md", "0.8", "weekly"},
	}

	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

	for _, item := range routes {
		sb.WriteString("  <url>\n")
		sb.WriteString(fmt.Sprintf("    <loc>%s%s</loc>\n", baseURL, item.path))
		sb.WriteString(fmt.Sprintf("    <lastmod>%s</lastmod>\n", now))
		sb.WriteString(fmt.Sprintf("    <changefreq>%s</changefreq>\n", item.changefreq))
		sb.WriteString(fmt.Sprintf("    <priority>%s</priority>\n", item.priority))
		sb.WriteString("  </url>\n")
	}

	sb.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}

// APICatalogJSON serves the RFC 9727 API Catalog in application/linkset+json format.
func (h *UIHandler) APICatalogJSON(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	catalog := map[string]any{
		"linkset": []map[string]any{
			{
				"anchor": baseURL + "/api/v1",
				"service-desc": []map[string]any{
					{
						"href": baseURL + "/api/v1/openapi.json",
						"type": "application/vnd.oai.openapi+json;version=3.1",
					},
				},
				"service-doc": []map[string]any{
					{
						"href": baseURL + "/docs/api",
						"type": "text/html",
					},
					{
						"href": baseURL + "/auth.md",
						"type": "text/markdown",
					},
				},
				"status": []map[string]any{
					{
						"href": baseURL + "/healthz",
						"type": "application/json",
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/linkset+json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(catalog)
}

// OpenIDConfiguration serves OAuth 2.0 / OpenID Connect discovery metadata (RFC 8414).
func (h *UIHandler) OpenIDConfiguration(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	config := map[string]any{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/auth/login",
		"token_endpoint":                        baseURL + "/api/v1/auth/login",
		"userinfo_endpoint":                     baseURL + "/api/v1/auth/me",
		"jwks_uri":                              baseURL + "/.well-known/jwks.json",
		"registration_endpoint":                 baseURL + "/api/v1/auth/register",
		"scopes_supported":                      []string{"openid", "profile", "email", "catalog:read", "offers:read", "orders:write", "compare:read"},
		"response_types_supported":              []string{"token", "code"},
		"grant_types_supported":                 []string{"password", "client_credentials", "refresh_token", "authorization_code"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "bearer"},
		"agent_auth": map[string]any{
			"register_uri":             baseURL + "/auth.md",
			"supported_identity_types": []string{"service_account", "api_key", "bearer_token"},
			"credential_types":         []string{"api_key", "bearer_jwt"},
			"documentation_url":        baseURL + "/docs/api",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(config)
}

// OAuthProtectedResource serves OAuth Protected Resource Metadata (RFC 9728).
func (h *UIHandler) OAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	metadata := map[string]any{
		"resource":              baseURL + "/api/v1",
		"authorization_servers": []string{baseURL},
		"scopes_supported": []string{
			"catalog:read",
			"offers:read",
			"orders:write",
			"compare:read",
			"profile",
		},
		"bearer_methods_supported": []string{
			"header",
			"cookie",
		},
		"resource_documentation": baseURL + "/docs/api",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(metadata)
}

// AuthMD serves agent registration and authentication documentation.
func (h *UIHandler) AuthMD(w http.ResponseWriter, r *http.Request) {
	h.serveMarkdownDoc(w, r, "Dawa24 Agent Authentication", generateAuthMD(r.Host))
}

// MCPServerCard serves the MCP Server Card specification (SEP-1649).
func (h *UIHandler) MCPServerCard(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	card := map[string]any{
		"$schema": "https://json.schemastore.org/mcp-server-card.json",
		"serverInfo": map[string]any{
			"name":        "dawa24-store",
			"title":       "Dawa24 B2B Pharmaceutical Exchange MCP Server",
			"version":     "1.0.0",
			"description": "Exposes tools for searching pharmaceutical products, comparing discounts, browsing suppliers, checking stock, and placing wholesale orders on Dawa24.",
		},
		"transport": map[string]any{
			"type": "sse",
			"url":  baseURL + "/api/v1/mcp/sse",
		},
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": true,
			},
			"resources": map[string]any{
				"subscribe": false,
			},
			"prompts": map[string]any{
				"listChanged": true,
			},
		},
		"tools": []map[string]any{
			{
				"name":        "search_products",
				"description": "Search pharmaceutical catalog products by trade name, barcode, active ingredient, or pharmaceutical category.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Search keyword"},
						"page":  map[string]any{"type": "integer", "default": 1},
						"limit": map[string]any{"type": "integer", "default": 20},
					},
					"required": []string{"query"},
				},
			},
			{
				"name":        "compare_discounts",
				"description": "Compare commercial discount matrices, bonus schemes, and credit terms across licensed Egyptian drug suppliers.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Medicine name or supplier name"},
					},
				},
			},
			{
				"name":        "list_suppliers",
				"description": "List verified pharmaceutical warehouses, distributors, and manufacturers with active licenses.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"governorate_id": map[string]any{"type": "integer", "description": "Optional governorate ID filter"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(card)
}

// AgentSkillsIndex serves the Agent Skills discovery index (RFC v0.2.0).
func (h *UIHandler) AgentSkillsIndex(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	skill1Doc := generateSkillMarkdown("catalog-search", r.Host)
	skill2Doc := generateSkillMarkdown("smart-ordering", r.Host)
	skill3Doc := generateSkillMarkdown("discount-comparison", r.Host)

	d1 := sha256.Sum256([]byte(skill1Doc))
	d2 := sha256.Sum256([]byte(skill2Doc))
	d3 := sha256.Sum256([]byte(skill3Doc))

	index := map[string]any{
		"$schema": "https://agentskills.io/schema/v0.2.0/index.json",
		"skills": []map[string]any{
			{
				"name":        "pharmaceutical-catalog-search",
				"type":        "api",
				"description": "Search Egyptian registered medicine catalog with barcode, trade names, and active ingredients.",
				"url":         baseURL + "/.well-known/agent-skills/catalog-search/SKILL.md",
				"digest":      "sha256:" + hex.EncodeToString(d1[:]),
			},
			{
				"name":        "b2b-pharma-ordering",
				"type":        "action",
				"description": "Execute B2B smart pharmaceutical orders with automated item matching and discount maximization.",
				"url":         baseURL + "/.well-known/agent-skills/smart-ordering/SKILL.md",
				"digest":      "sha256:" + hex.EncodeToString(d2[:]),
			},
			{
				"name":        "discount-comparison",
				"type":        "analysis",
				"description": "Compare commercial discount matrices across Egyptian pharmaceutical suppliers.",
				"url":         baseURL + "/.well-known/agent-skills/discount-comparison/SKILL.md",
				"digest":      "sha256:" + hex.EncodeToString(d3[:]),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(index)
}

// AgentSkillDoc serves individual SKILL.md markdown definitions.
func (h *UIHandler) AgentSkillDoc(w http.ResponseWriter, r *http.Request) {
	skill := chi.URLParam(r, "skill")
	if skill == "" {
		skill = "catalog-search"
	}
	content := generateSkillMarkdown(skill, r.Host)
	h.serveMarkdownDoc(w, r, "Skill: "+skill, content)
}

// AICatalogJSON serves the ARD (Agentic Resource Discovery) capability manifest.
func (h *UIHandler) AICatalogJSON(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	catalog := map[string]any{
		"specVersion": "1.0.0",
		"host": map[string]any{
			"domain":      r.Host,
			"name":        "Dawa24 Pharmaceutical Marketplace",
			"description": "Unified B2B pharmaceutical marketplace connecting licensed pharmacies with verified drug suppliers in Egypt.",
			"contact":     "support@dawa24.com",
		},
		"entries": []map[string]any{
			{
				"id":          "urn:air:dawa24:mcp:server",
				"displayName": "Dawa24 Model Context Protocol (MCP) Server",
				"type":        "application/json+mcp-card",
				"url":         baseURL + "/.well-known/mcp/server-card.json",
				"representativeQueries": []string{
					"Search medicines in Egyptian pharmacies",
					"Find best drug discounts for pharmacy",
					"Check supplier inventory for antibiotics",
					"Order pharmaceutical supplies wholesale",
				},
			},
			{
				"id":          "urn:air:dawa24:api:openapi",
				"displayName": "Dawa24 REST API OpenAPI Specification",
				"type":        "application/vnd.oai.openapi+json;version=3.1",
				"url":         baseURL + "/api/v1/openapi.json",
				"representativeQueries": []string{
					"Dawa24 API documentation",
					"Pharma marketplace REST API",
					"Integrate pharmacy ERP with Dawa24",
				},
			},
			{
				"id":          "urn:air:dawa24:skills:index",
				"displayName": "Dawa24 Agent Skills Index",
				"type":        "application/json+agent-skills",
				"url":         baseURL + "/.well-known/agent-skills/index.json",
				"representativeQueries": []string{
					"AI agent skills for pharmacy ordering",
					"Automated medical stock replenishment",
					"Drug price and discount comparator skill",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(catalog)
}

// JWKSJSON serves empty or platform JSON Web Key Set for token validation.
func (h *UIHandler) JWKSJSON(w http.ResponseWriter, r *http.Request) {
	jwks := map[string]any{
		"keys": []any{},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(jwks)
}