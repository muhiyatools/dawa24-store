package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
)

// AdminSiteSettingsSubmit persists public contact info and social media links.
func (h *UIHandler) AdminSiteSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", "خدمة الإعدادات غير متاحة.")
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{SocialLinks: map[string]string{}}
	}
	if curr.SocialLinks == nil {
		curr.SocialLinks = map[string]string{}
	}

	section := r.FormValue("section")
	if section == "contact" {
		curr.SiteName = strings.TrimSpace(r.FormValue("site_name"))
		curr.SiteDescription = strings.TrimSpace(r.FormValue("site_description"))
		curr.ContactEmail = strings.TrimSpace(r.FormValue("contact_email"))
		curr.SupportEmail = strings.TrimSpace(r.FormValue("support_email"))
		curr.Phone = strings.TrimSpace(r.FormValue("phone"))
		curr.WhatsApp = strings.TrimSpace(r.FormValue("whatsapp"))
		curr.Address = strings.TrimSpace(r.FormValue("address"))
	} else if section == "socials" {
		curr.SocialLinks["facebook"] = strings.TrimSpace(r.FormValue("social_facebook"))
		curr.SocialLinks["twitter"] = strings.TrimSpace(r.FormValue("social_twitter"))
		curr.SocialLinks["instagram"] = strings.TrimSpace(r.FormValue("social_instagram"))
		curr.SocialLinks["linkedin"] = strings.TrimSpace(r.FormValue("social_linkedin"))
		curr.SocialLinks["youtube"] = strings.TrimSpace(r.FormValue("social_youtube"))
		curr.SocialLinks["tiktok"] = strings.TrimSpace(r.FormValue("social_tiktok"))
		curr.SocialLinks["snapchat"] = strings.TrimSpace(r.FormValue("social_snapchat"))
		curr.SocialLinks["telegram"] = strings.TrimSpace(r.FormValue("social_telegram"))
		if curr.WhatsApp != "" {
			curr.SocialLinks["whatsapp"] = "https://wa.me/" + curr.WhatsApp
		}
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, langOf(r)))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", "تم حفظ وتحديث إعدادات الموقع بنجاح.")
}

// AdminBrandingSubmit updates platform logo and favicon.
func (h *UIHandler) AdminBrandingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", "خدمة الإعدادات غير متاحة.")
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{}
	}

	_ = r.ParseMultipartForm(10 << 20)

	logoURL := strings.TrimSpace(r.FormValue("logo_url"))
	faviconURL := strings.TrimSpace(r.FormValue("favicon_url"))

	// Check if a new logo file was uploaded
	if file, header, err := r.FormFile("logo_file"); err == nil && file != nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/logo_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}

		uploadedToStorage := false
		if h.storage != nil {
			if err := h.storage.Put(ctx, key, file, header.Size, contentType); err != nil {
				h.log.WarnContext(ctx, "branding: upload logo to storage", "error", err)
			} else {
				pubURL := h.storage.PublicURL(key)
				if pubURL == "" {
					pubURL = "/uploads/" + key
				}
				logoURL = pubURL
				uploadedToStorage = true
			}
		}

		// Also save locally as static fallback
		if !uploadedToStorage {
			savePath := "internal/ui/static/img/logo.png"
			if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo create", "error", err)
			} else {
				defer out.Close()
				_, _ = file.Seek(0, 0)
				_, _ = io.Copy(out, file)
				logoURL = "/static/img/logo.png"
			}
		}
	}

	// Check if a new favicon file was uploaded
	if file, header, err := r.FormFile("favicon_file"); err == nil && file != nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/favicon_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}

		if h.storage != nil {
			if err := h.storage.Put(ctx, key, file, header.Size, contentType); err != nil {
				h.log.WarnContext(ctx, "branding: upload favicon to storage", "error", err)
			} else {
				pubURL := h.storage.PublicURL(key)
				if pubURL == "" {
					pubURL = "/uploads/" + key
				}
				faviconURL = pubURL
			}
		}
	}

	if logoURL != "" {
		curr.LogoURL = logoURL
	}
	if faviconURL != "" {
		curr.FaviconURL = faviconURL
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, langOf(r)))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", "تم حفظ وتطبيق الهوية البصرية بنجاح.")
}
