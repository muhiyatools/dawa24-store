package ui

import (
	"net/url"
	"strings"
)

// safeLocalRedirect validates that a target redirect destination is a safe relative
// path on the local origin, preventing open redirect vulnerabilities.
func safeLocalRedirect(target, fallback string) string {
	target = strings.TrimSpace(target)
	if target == "" || !strings.HasPrefix(target, "/") {
		return fallback
	}
	if strings.HasPrefix(target, "//") || strings.HasPrefix(target, "/\\") || strings.HasPrefix(target, "/\t") {
		return fallback
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return fallback
	}
	return target
}
