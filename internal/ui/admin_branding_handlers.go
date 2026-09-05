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

		// Fallback when object storage is not configured: write into the
		// persistent uploads volume, not internal/ui/static. Static assets are
		// served from the //go:embed snapshot taken at startup (see
		// internal/ui/static.go), so a file written under internal/ui/static at
		// runtime is never served and is wiped on the next image rebuild. The
		// uploads directory is a mounted volume and is served by
		// RegisterUploadRoutes.
		if !uploadedToStorage {
			savePath := filepath.Join(GetUploadBaseDir(), "branding", fmt.Sprintf("logo_%d%s", time.Now().Unix(), ext))
			if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo mkdir", "error", err)
			} else if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo create", "error", err)
			} else {
				defer out.Close()
				_, _ = file.Seek(0, 0)
				if _, err := io.Copy(out, file); err != nil {
					h.log.WarnContext(ctx, "branding: fallback logo write", "error", err)
				} else {
					logoURL = "/uploads/branding/" + filepath.Base(savePath)
				}
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

		// Same reasoning as the logo fallback above: persist to the uploads
		// volume when object storage is not configured.
		if !uploadedToStorage {
			savePath := filepath.Join(GetUploadBaseDir(), "branding", fmt.Sprintf("favicon_%d%s", time.Now().Unix(), ext))
			if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
				h.log.WarnContext(ctx, "branding: fallback favicon mkdir", "error", err)
			} else if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback favicon create", "error", err)
			} else {
				defer out.Close()
				_, _ = file.Seek(0, 0)
				if _, err := io.Copy(out, file); err != nil {
					h.log.WarnContext(ctx, "branding: fallback favicon write", "error", err)
				} else {
					faviconURL = "/uploads/branding/" + filepath.Base(savePath)
				}
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
