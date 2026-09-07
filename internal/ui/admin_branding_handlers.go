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
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminSiteSettingsSubmit persists public contact info and social media links.
func (h *UIHandler) AdminSiteSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", i18n.T(lang, "admin.settings.service_unavailable"))
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
	if section == "contact" || section == "" {
		if name := strings.TrimSpace(r.FormValue("site_name")); name != "" {
			curr.SiteName = name
		}
		if desc := strings.TrimSpace(r.FormValue("site_description")); desc != "" || r.Form.Has("site_description") {
			curr.SiteDescription = desc
		}
		if email := strings.TrimSpace(r.FormValue("contact_email")); email != "" || r.Form.Has("contact_email") {
			curr.ContactEmail = email
		}
		if supEmail := strings.TrimSpace(r.FormValue("support_email")); supEmail != "" || r.Form.Has("support_email") {
			curr.SupportEmail = supEmail
		}
		if phone := strings.TrimSpace(r.FormValue("phone")); phone != "" || r.Form.Has("phone") {
			curr.Phone = phone
		}
		if wa := strings.TrimSpace(r.FormValue("whatsapp")); wa != "" || r.Form.Has("whatsapp") {
			curr.WhatsApp = wa
		}
		if addr := strings.TrimSpace(r.FormValue("address")); addr != "" || r.Form.Has("address") {
			curr.Address = addr
		}
		if logo := strings.TrimSpace(r.FormValue("logo_url")); logo != "" {
			curr.LogoURL = logo
		}
		if fav := strings.TrimSpace(r.FormValue("favicon_url")); fav != "" {
			curr.FaviconURL = fav
		}
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
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, lang))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", i18n.T(lang, "admin.branding.site_saved_success"))
}

// AdminBrandingSubmit updates platform logo and favicon.
func (h *UIHandler) AdminBrandingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{}
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)

	logoURL := strings.TrimSpace(r.FormValue("logo_url"))
	faviconURL := strings.TrimSpace(r.FormValue("favicon_url"))
	if name := strings.TrimSpace(r.FormValue("site_name")); name != "" {
		curr.SiteName = name
	}

	// Check if a new logo file was uploaded
	if file, header, err := r.FormFile("logo_file"); err == nil && file != nil {
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/logo_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			switch ext {
			case ".svg":
				contentType = "image/svg+xml"
			case ".webp":
				contentType = "image/webp"
			case ".jpg", ".jpeg":
				contentType = "image/jpeg"
			case ".png":
				contentType = "image/png"
			default:
				contentType = "image/png"
			}
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

		if !uploadedToStorage {
			savePath := filepath.Join(GetUploadBaseDir(), "branding", fmt.Sprintf("logo_%d%s", time.Now().Unix(), ext))
			if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo mkdir", "error", err)
			} else if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo create", "error", err)
			} else {
				_, _ = file.Seek(0, 0)
				if _, err := io.Copy(out, file); err != nil {
					h.log.WarnContext(ctx, "branding: fallback logo write", "error", err)
				} else {
					logoURL = "/uploads/branding/" + filepath.Base(savePath)
				}
				out.Close()
			}
		}
	}

	// Check if a new favicon file was uploaded
	if file, header, err := r.FormFile("favicon_file"); err == nil && file != nil {
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/favicon_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			switch ext {
			case ".svg":
				contentType = "image/svg+xml"
			case ".ico":
				contentType = "image/x-icon"
			case ".webp":
				contentType = "image/webp"
			case ".jpg", ".jpeg":
				contentType = "image/jpeg"
			case ".png":
				contentType = "image/png"
			default:
				contentType = "image/png"
			}
		}

		uploadedToStorage := false
		if h.storage != nil {
			if err := h.storage.Put(ctx, key, file, header.Size, contentType); err != nil {
				h.log.WarnContext(ctx, "branding: upload favicon to storage", "error", err)
			} else {
				pubURL := h.storage.PublicURL(key)
				if pubURL == "" {
					pubURL = "/uploads/" + key
				}
				faviconURL = pubURL
				uploadedToStorage = true
			}
		}

		if !uploadedToStorage {
			savePath := filepath.Join(GetUploadBaseDir(), "branding", fmt.Sprintf("favicon_%d%s", time.Now().Unix(), ext))
			if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
				h.log.WarnContext(ctx, "branding: fallback favicon mkdir", "error", err)
			} else if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback favicon create", "error", err)
			} else {
				_, _ = file.Seek(0, 0)
				if _, err := io.Copy(out, file); err != nil {
					h.log.WarnContext(ctx, "branding: fallback favicon write", "error", err)
				} else {
					faviconURL = "/uploads/branding/" + filepath.Base(savePath)
				}
				out.Close()
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
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, lang))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", i18n.T(lang, "admin.branding.branding_saved_success"))
}
