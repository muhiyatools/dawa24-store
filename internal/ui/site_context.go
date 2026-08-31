package ui

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"net/http"
	"sync"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/ui/layouts"
)

type siteSettingsKey struct{}

var (
	cachedSettings *platformadmin.SiteSettings
	cachedAt       time.Time
	cacheMu        sync.RWMutex
)

// InvalidateSiteSettingsCache clears the cached settings so next request fetches fresh from DB.
func InvalidateSiteSettingsCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cachedSettings = nil
}

// siteSettingsMiddleware injects live SiteSettings from database into every request context.
func (h *UIHandler) siteSettingsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		cacheMu.RLock()
		if cachedSettings != nil && time.Since(cachedAt) < 10*time.Second {
			s := cachedSettings
			cacheMu.RUnlock()
			next.ServeHTTP(w, r.WithContext(WithSiteSettings(ctx, s)))
			return
		}
		cacheMu.RUnlock()

		var s *platformadmin.SiteSettings
		if h.adminSvc != nil {
			s, _ = h.adminSvc.GetSiteSettings(ctx)
		}
		if s == nil {
			s = DefaultSiteSettings()
		}

		cacheMu.Lock()
		cachedSettings = s
		cachedAt = time.Now()
		cacheMu.Unlock()

		next.ServeHTTP(w, r.WithContext(layouts.WithSiteSettings(ctx, s)))
	})
}

// WithSiteSettings embeds SiteSettings into context.
func WithSiteSettings(ctx context.Context, s *platformadmin.SiteSettings) context.Context {
	return layouts.WithSiteSettings(ctx, s)
}

// GetSiteSettings retrieves SiteSettings from context or returns default.
func GetSiteSettings(ctx context.Context) *platformadmin.SiteSettings {
	return layouts.GetSiteSettings(ctx)
}

// DefaultSiteSettings returns fallback settings if DB is uninitialized.
func DefaultSiteSettings() *platformadmin.SiteSettings {
	return &platformadmin.SiteSettings{
		SiteName:        "دواء 24",
		SiteDescription: i18n.TDefault("w4_ui.s_99_99"),
		LogoURL:         "/static/img/logo.png",
		FaviconURL:      "/static/img/logo.png",
		ContactEmail:    "info@dawa24.com",
		SupportEmail:    "support@dawa24.com",
		Phone:           "01065397000",
		WhatsApp:        "201065397000",
		Address:         i18n.TDefault("w4_ui.s_100_100"),
		SocialLinks: map[string]string{
			"facebook":  "https://facebook.com/dawa24",
			"twitter":   "https://twitter.com/dawa24",
			"instagram": "https://instagram.com/dawa24",
			"linkedin":  "https://linkedin.com/company/dawa24",
			"youtube":   "https://youtube.com/@dawa24",
			"tiktok":    "https://tiktok.com/@dawa24",
			"snapchat":  "https://snapchat.com/add/dawa24",
			"whatsapp":  "https://wa.me/201065397000",
			"telegram":  "https://t.me/dawa24",
		},
	}
}
