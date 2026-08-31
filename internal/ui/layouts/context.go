package layouts

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

type siteSettingsKey struct{}

// WithSiteSettings embeds SiteSettings into the context for all layout rendering.
func WithSiteSettings(ctx context.Context, s *platformadmin.SiteSettings) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, siteSettingsKey{}, s)
}

// GetSiteSettings retrieves SiteSettings from the context.
func GetSiteSettings(ctx context.Context) *platformadmin.SiteSettings {
	if ctx != nil {
		if s, ok := ctx.Value(siteSettingsKey{}).(*platformadmin.SiteSettings); ok && s != nil {
			return s
		}
	}
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
