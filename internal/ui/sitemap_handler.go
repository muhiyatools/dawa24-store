package ui

import (
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