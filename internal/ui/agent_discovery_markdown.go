package ui

import (
	"fmt"
	"net/http"
	"strings"
)

func (h *UIHandler) serveMarkdownDoc(w http.ResponseWriter, r *http.Request, title, markdownContent string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	// x-markdown-tokens approximate calculation (avg 4 chars per token)
	tokenCount := len(markdownContent) / 4
	if tokenCount < 10 {
		tokenCount = 10
	}
	w.Header().Set("X-Markdown-Tokens", fmt.Sprintf("%d", tokenCount))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(markdownContent))
}

func generateAPIDocsMarkdown(host string) string {
	return fmt.Sprintf(`# Dawa24 API & Agent Documentation

> Unified B2B Pharmaceutical Marketplace & Drug Supply Platform in Egypt.

## Overview
Dawa24 provides programmatic interfaces for licensed pharmacies, pharmaceutical warehouses, distributors, and autonomous AI agents to search medicines, compare commercial discounts, check batch availability, and execute wholesale purchases.

## Key Discovery Endpoints
- **OpenAPI 3.1 Spec**: [https://%s/api/v1/openapi.json](https://%s/api/v1/openapi.json)
- **API Catalog (RFC 9727)**: [https://%s/.well-known/api-catalog](https://%s/.well-known/api-catalog)
- **AI Capability Manifest (ARD)**: [https://%s/.well-known/ai-catalog.json](https://%s/.well-known/ai-catalog.json)
- **MCP Server Card**: [https://%s/.well-known/mcp/server-card.json](https://%s/.well-known/mcp/server-card.json)
- **Agent Skills Discovery**: [https://%s/.well-known/agent-skills/index.json](https://%s/.well-known/agent-skills/index.json)
- **OAuth Discovery**: [https://%s/.well-known/openid-configuration](https://%s/.well-known/openid-configuration)
- **Agent Authentication Guide**: [https://%s/auth.md](https://%s/auth.md)
- **Sitemap**: [https://%s/sitemap.xml](https://%s/sitemap.xml)

## Authentication
Pass credentials via HTTP Authorization Header:
` + "```http" + `
Authorization: Bearer <your_session_token_or_api_key>
` + "```" + `
`, host, host, host, host, host, host, host, host, host, host, host, host, host, host, host, host)
}

func generateAuthMD(host string) string {
	return fmt.Sprintf(`# Dawa24 Agent Authentication & Registration (auth.md)

Welcome AI agents and autonomous systems. Dawa24 supports programmatic agent registration, credential issuance, and authenticated execution.

## 1. OAuth / OIDC Discovery
- **Issuer**: ` + "`https://%s`" + `
- **Authorization Server Metadata**: [https://%s/.well-known/oauth-authorization-server](https://%s/.well-known/oauth-authorization-server)
- **OpenID Configuration**: [https://%s/.well-known/openid-configuration](https://%s/.well-known/openid-configuration)
- **OAuth Protected Resource Metadata**: [https://%s/.well-known/oauth-protected-resource](https://%s/.well-known/oauth-protected-resource)

## 2. Agent Registration Flow
To register an autonomous agent with Dawa24:
1. Submit an agent registration request to ` + "`POST https://%s/api/v1/auth/register`" + ` with your agent identifier, operating organization, and intended scopes.
2. For interactive authorization, navigate the user to ` + "`https://%s/auth/login?redirect=...`" + `.
3. Obtain your bearer token or API key.

## 3. Supported Scopes
- ` + "`catalog:read`" + `: Search pharmaceutical products, formulations, and barcodes.
- ` + "`offers:read`" + `: Query real-time supplier discount tiers, payment conditions, and stock.
- ` + "`orders:write`" + `: Submit B2B wholesale purchase orders.
- ` + "`compare:read`" + `: Compare supplier prices and commercial discount matrices across Egypt.

## 4. Example Request
` + "```bash" + `
curl -X GET "https://%s/api/v1/auth/me" \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Accept: application/json"
` + "```" + `
`, host, host, host, host, host, host, host, host, host, host)
}

func generateSkillMarkdown(skillName, host string) string {
	cleanName := strings.TrimSpace(strings.ToLower(skillName))
	switch cleanName {
	case "catalog-search", "pharmaceutical-catalog-search":
		return fmt.Sprintf(`# Pharmaceutical Catalog Search Skill

Search and verify registered pharmaceutical medicines and medical supplies across Egyptian distributors.

## Schema
- **Endpoint**: ` + "`https://%s/api/v1/compare/search?q={query}`" + `
- **Method**: GET
- **Parameters**: ` + "`q`" + ` (search keyword for medicine trade name, active ingredient, or barcode)
`, host)
	case "smart-ordering", "b2b-pharma-ordering":
		return fmt.Sprintf(`# B2B Smart Ordering Skill

Execute automated multi-supplier item matching and discount-maximizing purchase orders.

## Schema
- **Endpoint**: ` + "`https://%s/api/v1/orders`" + `
- **Method**: POST
- **Requires Scope**: ` + "`orders:write`" + `
`, host)
	default:
		return fmt.Sprintf(`# Discount Comparison Skill

Compare commercial discount structures and delivery terms across verified Egyptian suppliers.

## Schema
- **Endpoint**: ` + "`https://%s/api/v1/compare/search`" + `
- **Method**: GET
`, host)
	}
}