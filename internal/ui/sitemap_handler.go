package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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
		{"/llms.txt", "0.8", "weekly"},
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

// LLMsTxt serves /llms.txt per https://llmstxt.org providing an LLM-friendly index of the site.
func (h *UIHandler) LLMsTxt(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	content := fmt.Sprintf(`# Dawa24 (دواء 24)

> Unified B2B Marketplace connecting licensed pharmacies with verified pharmaceutical suppliers and warehouses in Egypt.

## Core Information
- Platform: Dawa24 B2B Pharmaceutical Marketplace
- Scope: Arab Republic of Egypt
- Website: %s

## Public Sections
- [About Dawa24](%s/about): Platform vision and mission.
- [How It Works](%s/how-it-works): Pharmacy ordering and supplier dispatch workflows.
- [Careers](%s/jobs): Open positions in technology, sales, and logistics.
- [FAQ](%s/faq): Common inquiries about pharmaceutical licensing and fulfillment.
- [Contact](%s/contact): Support hotline and contact information.
- [Terms](%s/terms): Platform terms and conditions.
- [Privacy Policy](%s/privacy): Data privacy policies.
`, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL, baseURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

// AgentsIndexJSON serves /.well-known/agents-index.json for DNS-AID HTTP discovery.
func (h *UIHandler) AgentsIndexJSON(w http.ResponseWriter, r *http.Request) {
	baseURL := resolveBaseURL(r)
	resp := map[string]any{
		"$schema":      "https://agents-index.org/schema/v1.json",
		"version":      "1.0",
		"organization": "Dawa24",
		"domain":       r.Host,
		"agents": []map[string]any{
			{
				"name":        "dawa24-web",
				"protocol":    "https",
				"endpoint":    baseURL,
				"description": "Dawa24 B2B Pharmaceutical Marketplace Web Interface",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}