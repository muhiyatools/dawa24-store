package ui

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

const visitorCookieName = "dawa24_visitor"

// visitorMiddleware records one visitor row per session per day. It never
// writes per request: the cookie carries the day already recorded, so a write
// happens at most once per visitor per day.
func (h *UIHandler) visitorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets are not page views; skip them.
		if h.adminSvc != nil && !strings.HasPrefix(r.URL.Path, "/static/") {
			h.recordVisitor(w, r)
		}
		next.ServeHTTP(w, r)
	})
}

// recordVisitor records the visitor if this is their first hit today, then
// refreshes the cookie so the next hit is a no-op.
func (h *UIHandler) recordVisitor(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")

	key := newVisitorKey()
	cookie, err := r.Cookie(visitorCookieName)
	if err == nil && cookie != nil {
		if dot := strings.LastIndex(cookie.Value, "."); dot >= 0 && cookie.Value[dot+1:] == today {
			return // already recorded today
		}
		key = cookie.Value
	}

	browser, device, osName := parseUserAgent(r.UserAgent())
	_ = h.adminSvc.RecordVisitor(r.Context(), &platformadmin.Visitor{
		VisitorKey: key,
		IP:         truncateIP(r.RemoteAddr),
		UserAgent:  r.UserAgent(),
		Browser:    browser,
		Device:     device,
		OS:         osName,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     visitorCookieName,
		Value:    key + "." + today,
		Path:     "/",
		MaxAge:   86400 * 365 * 2,
		SameSite: http.SameSiteLaxMode,
	})
}

func newVisitorKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "v" + time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(b)
}

// truncateIP reduces an address to a /24 (IPv4) or /64 (IPv6) prefix so raw
// client IPs are not retained, and strips any port.
func truncateIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			v4[2], v4[3] = 0, 0
			return v4.String()
		}
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}
	return host
}

// parseUserAgent does a lightweight, dependency-free split of the user agent.
func parseUserAgent(ua string) (browser, device, osName string) {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "chrome") || strings.Contains(u, "crios"):
		browser = "chrome"
	case strings.Contains(u, "firefox") || strings.Contains(u, "fxios"):
		browser = "firefox"
	case strings.Contains(u, "safari"):
		browser = "safari"
	case strings.Contains(u, "edg"):
		browser = "edge"
	default:
		browser = "other"
	}
	switch {
	case strings.Contains(u, "ipad"):
		device = "tablet"
	case strings.Contains(u, "mobile") || strings.Contains(u, "android") || strings.Contains(u, "iphone"):
		device = "mobile"
	default:
		device = "desktop"
	}
	switch {
	case strings.Contains(u, "windows"):
		osName = "windows"
	case strings.Contains(u, "mac os") || strings.Contains(u, "iphone") || strings.Contains(u, "ipad"):
		osName = "ios/macos"
	case strings.Contains(u, "android"):
		osName = "android"
	case strings.Contains(u, "linux"):
		osName = "linux"
	default:
		osName = "other"
	}
	return browser, device, osName
}
