package ui

import (
	"fmt"
	"net/http"
	"strings"
)

// AgentDiscoveryLinkHeadersMiddleware injects RFC 8288 Link headers for agent discovery.
func (h *UIHandler) AgentDiscoveryLinkHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		linkHeader := `</.well-known/api-catalog>; rel="api-catalog", </.well-known/ai-catalog.json>; rel="service-desc", </.well-known/openid-configuration>; rel="oauth-authorization-server", </.well-known/oauth-protected-resource>; rel="oauth-protected-resource", </.well-known/mcp/server-card.json>; rel="mcp-server-card", </.well-known/agent-skills/index.json>; rel="agent-skills", </docs/api>; rel="service-doc", </auth.md>; rel="author-uri"`
		w.Header().Set("Link", linkHeader)
		next.ServeHTTP(w, r)
	})
}

// MarkdownNegotiationMiddleware handles Content Negotiation for AI agents (Accept: text/markdown).
func (h *UIHandler) MarkdownNegotiationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		formatQ := r.URL.Query().Get("format")
		isMarkdownAgent := strings.Contains(accept, "text/markdown") ||
			strings.Contains(accept, "text/x-markdown") ||
			formatQ == "markdown" ||
			r.URL.Query().Get("markdown") == "true"

		if !isMarkdownAgent || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		var mdContent string

		switch path {
		case "/":
			mdContent = fmt.Sprintf(`# Dawa24 (دواء 24) - B2B Pharmaceutical Marketplace

Unified platform connecting licensed pharmacies with verified pharmaceutical suppliers and warehouses in Egypt.

## Key Features
- **Smart Order Processing**: Upload pharmacy stock/order lists and get instant multi-supplier quotation with maximum commercial discounts.
- **Direct Supplier Directory**: Browse verified Egyptian drug warehouses and distributors.
- **Discount & Pricing Comparator**: Compare real-time medicine bonus structures and payment credit terms.

## Navigation & Links
- [About Dawa24](/about)
- [How It Works](/how-it-works)
- [Job Openings](/jobs)
- [Frequently Asked Questions](/faq)
- [Contact Support](/contact)
- [API Documentation](/docs/api)
- [Agent Auth Guide](/auth.md)
- [Sign In](/auth/login)
- [Register Pharmacy or Supplier](/auth/register)
`)
		case "/about":
			mdContent = `# About Dawa24

Dawa24 is Egypt's dedicated digital marketplace for pharmaceutical supply and distribution. We empower independent community pharmacies and verified suppliers to trade efficiently, transparently, and securely.
`
		case "/how-it-works":
			mdContent = `# How Dawa24 Works

1. **For Pharmacies**: Sign up, upload order requisition lists via Smart Order or search the catalog, select the highest commercial discounts, and receive deliveries.
2. **For Suppliers**: Publish inventories, discounts, and payment terms to thousands of licensed pharmacies across Egypt.
`
		case "/jobs":
			mdContent = `# Careers at Dawa24

Join our team building the future of digital healthcare and pharmaceutical supply chain in Egypt. Explore open engineering, sales, logistics, and customer success positions.
`
		case "/faq":
			mdContent = `# Frequently Asked Questions (FAQ)

- **Who can join Dawa24?** Licensed pharmacies, drug warehouses, distributors, and pharmaceutical companies in Egypt.
- **How are orders delivered?** Through verified distributor logistics and delivery representatives.
`
		case "/contact":
			mdContent = `# Contact Dawa24

- **Email**: support@dawa24.com
- **Phone / WhatsApp**: 01065397000 (+201065397000)
- **Location**: Cairo, Arab Republic of Egypt
`
		case "/terms":
			mdContent = `# Terms & Conditions of Dawa24

Official terms of service governing pharmaceutical trading, order validation, licensing compliance, and user obligations.
`
		case "/privacy":
			mdContent = `# Privacy Policy

Dawa24 privacy guidelines, data security, and pharmaceutical entity compliance.
`
		default:
			next.ServeHTTP(w, r)
			return
		}

		h.serveMarkdownDoc(w, r, "Dawa24 - "+path, mdContent)
	})
}