package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OpenAPISpecJSON serves the OpenAPI 3.1.0 specification for Dawa24 B2B APIs.
func (h *UIHandler) OpenAPISpecJSON(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":          "Dawa24 B2B Pharmaceutical Marketplace API",
			"version":        "1.0.0",
			"description":    "Official REST API and Agent Interfaces for Dawa24 Egypt - connecting licensed pharmacies with verified drug distributors, warehouses, and pharmaceutical manufacturers.",
			"termsOfService": baseURL + "/terms",
			"contact": map[string]any{
				"name":  "Dawa24 Developer Support",
				"url":   baseURL + "/contact",
				"email": "support@dawa24.com",
			},
			"license": map[string]any{
				"name": "Proprietary / Commercial License",
				"url":  baseURL + "/terms",
			},
		},
		"servers": []map[string]any{
			{
				"url":         baseURL + "/api/v1",
				"description": "Primary Production API Endpoint",
			},
		},
		"paths": map[string]any{
			"/auth/login": map[string]any{
				"post": map[string]any{
					"summary":     "Authenticate User or Agent",
					"description": "Authenticates credentials and issues a session token / bearer cookie.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"email":    map[string]any{"type": "string", "format": "email"},
										"password": map[string]any{"type": "string"},
									},
									"required": []string{"email", "password"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Authentication successful",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"token": map[string]any{"type": "string"},
											"user":  map[string]any{"type": "object"},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "Invalid credentials"},
					},
				},
			},
			"/auth/me": map[string]any{
				"get": map[string]any{
					"summary":     "Get Current User & Actor Profile",
					"description": "Returns profile, active organization, and granted permissions for the authenticated caller.",
					"security":    []map[string]any{{"BearerAuth": []any{}}, {"CookieAuth": []any{}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Profile details retrieved successfully"},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
			},
			"/compare/search": map[string]any{
				"get": map[string]any{
					"summary":     "Compare Pharmaceutical Discounts",
					"description": "Searches and compares commercial discounts, payment terms, and supplier offers for pharmaceutical products in Egypt.",
					"parameters": []map[string]any{
						{
							"name":        "q",
							"in":          "query",
							"description": "Search keyword (brand name, generic name, or barcode)",
							"schema":      map[string]any{"type": "string"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Comparative discount results"},
					},
				},
			},
			"/assistant/chat": map[string]any{
				"post": map[string]any{
					"summary":     "Dawa24 AI Assistant Conversation",
					"description": "Interacts with the intelligent platform assistant for pharmacy stock lookup, drug alternatives, and supplier terms.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"message": map[string]any{"type": "string"},
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Assistant reply message"},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"BearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT / Token",
					"description":  "Pass token in Authorization header as: Bearer <token>",
				},
				"CookieAuth": map[string]any{
					"type":        "apiKey",
					"in":          "cookie",
					"name":        "dawa24_session",
					"description": "HttpOnly browser session cookie",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(spec)
}

// APIDocsPage renders a clean OpenAPI documentation page.
func (h *UIHandler) APIDocsPage(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/markdown") {
		h.serveMarkdownDoc(w, r, "Dawa24 API Documentation", generateAPIDocsMarkdown(r.Host))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Dawa24 API & Agent Documentation</title>
	<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Readex+Pro:wght@400;600;700&family=Spline+Sans+Mono:wght@400;600&display=swap">
	<link rel="stylesheet" href="/static/css/tokens.css">
	<link rel="stylesheet" href="/static/css/base.css">
	<link rel="stylesheet" href="/static/css/components.css">
</head>
<body class="bg-surface-sunken p-6 font-sans">
	<div class="max-w-4xl mx-auto glass-panel p-8">
		<div class="d-flex items-center justify-between pb-4 mb-6 border-b">
			<div>
				<h1 class="text-2xl font-black text-primary m-0">Dawa24 API & AI Agent Documentation</h1>
				<p class="text-sm text-secondary m-0 mt-1">Unified APIs for Egyptian Pharmaceutical Marketplace & B2B Smart Ordering</p>
			</div>
			<a href="/api/v1/openapi.json" class="btn btn-secondary btn-sm font-bold text-xs" target="_blank">OpenAPI Spec (JSON)</a>
		</div>

		<div class="d-flex flex-col gap-6">
			<div class="p-4 bg-surface rounded-xl border">
				<h2 class="text-base font-bold text-primary m-0 mb-2">Agent & Discovery Links</h2>
				<ul class="text-xs text-secondary d-flex flex-col gap-1.5 m-0 p-0 pl-4">
					<li><strong>API Catalog (RFC 9727):</strong> <a href="/.well-known/api-catalog" class="text-brand">/.well-known/api-catalog</a></li>
					<li><strong>AI Capability Manifest (ARD):</strong> <a href="/.well-known/ai-catalog.json" class="text-brand">/.well-known/ai-catalog.json</a></li>
					<li><strong>MCP Server Card (SEP-1649):</strong> <a href="/.well-known/mcp/server-card.json" class="text-brand">/.well-known/mcp/server-card.json</a></li>
					<li><strong>Agent Skills Index:</strong> <a href="/.well-known/agent-skills/index.json" class="text-brand">/.well-known/agent-skills/index.json</a></li>
					<li><strong>OAuth Discovery:</strong> <a href="/.well-known/openid-configuration" class="text-brand">/.well-known/openid-configuration</a></li>
					<li><strong>Agent Auth Guide:</strong> <a href="/auth.md" class="text-brand">/auth.md</a></li>
					<li><strong>Sitemap:</strong> <a href="/sitemap.xml" class="text-brand">/sitemap.xml</a></li>
				</ul>
			</div>

			<div class="p-4 bg-surface rounded-xl border">
				<h2 class="text-base font-bold text-primary m-0 mb-2">Authentication for AI Agents</h2>
				<p class="text-xs text-secondary leading-relaxed m-0 mb-3">
					Autonomous agents can authenticate using HTTP Bearer tokens or API keys passed in the <code>Authorization: Bearer &lt;token&gt;</code> header.
					See the full agent instructions at <a href="/auth.md" class="text-brand font-bold">/auth.md</a>.
				</p>
			</div>
		</div>
	</div>
</body>
</html>`
	_, _ = w.Write([]byte(html))
}