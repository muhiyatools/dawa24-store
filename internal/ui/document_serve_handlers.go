package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// DocumentViewHandler streams or redirects to the document file for in-browser viewing / preview.
func (h *UIHandler) DocumentViewHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, false)
}

// DocumentDownloadHandler forces downloading the document file.
func (h *UIHandler) DocumentDownloadHandler(w http.ResponseWriter, r *http.Request) {
	h.serveDocumentFile(w, r, true)
}

// serveDocumentFile safely finds, verifies access to, and streams a document file.
func (h *UIHandler) serveDocumentFile(w http.ResponseWriter, r *http.Request, download bool) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "يجب تسجيل الدخول لعرض المستند", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "معرف المستند غير صالح", http.StatusBadRequest)
		return
	}

	if h.attSvc == nil {
		http.Error(w, "خدمة المستندات غير متاحة", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	doc, err := h.attSvc.GetByIDAdmin(sysCtx, id)
	if err != nil || doc == nil {
		h.renderMissingDocError(w, r, nil, "المستند المطلوب غير مسجل بالنظام أو تم حذفه.", lang, dir)
		return
	}

	// Verify tenant authorization if not platform admin/staff
	if !actor.IsPlatformAdmin() && !actor.IsStaff {
		orgID := actor.OrganizationID
		if orgID <= 0 {
			orgID = actor.OrgID
		}
		hasAccess := false
		if doc.OrganizationID != nil && orgID > 0 && *doc.OrganizationID == orgID {
			hasAccess = true
		}
		if doc.UserID != nil && actor.UserID > 0 && *doc.UserID == actor.UserID {
			hasAccess = true
		}
		if !hasAccess {
			http.Error(w, "ليس لديك صلاحية لعرض هذا المستند", http.StatusForbidden)
			return
		}
	}

	rawURL := strings.TrimSpace(doc.FileURL)
	if rawURL == "" {
		rawURL = strings.TrimSpace(doc.StorageKey)
	}

	// 1. Sanitize the URL / Storage Key (NEVER redirect to localhost / private network endpoints)
	cleanKey := rawURL
	isInternalURL := false
	if strings.Contains(cleanKey, "://") {
		if u, parseErr := url.Parse(cleanKey); parseErr == nil {
			host := strings.ToLower(u.Hostname())
			if host == "localhost" || host == "127.0.0.1" || host == "minio" || host == "0.0.0.0" ||
				strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "172.") {
				isInternalURL = true
				p := u.Path
				p = strings.TrimPrefix(p, "/dawa24")
				cleanKey = p
			}
		}
	}

	// If it's a genuine public external URL (and NOT an internal localhost endpoint), redirect safely
	if !isInternalURL && (strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")) {
		http.Redirect(w, r, rawURL, http.StatusTemporaryRedirect)
		return
	}

	for strings.HasPrefix(cleanKey, "/") {
		cleanKey = strings.TrimPrefix(cleanKey, "/")
	}
	cleanPath := strings.TrimPrefix(cleanKey, "uploads/")
	for strings.HasPrefix(cleanPath, "/") {
		cleanPath = strings.TrimPrefix(cleanPath, "/")
	}
	baseName := filepath.Base(cleanKey)

	// 2. Check all local disk locations
	candidates := []string{
		filepath.Join(UploadBaseDir, cleanPath),
		filepath.Join(UploadBaseDir, cleanKey),
		filepath.Join(UploadBaseDir, "documents", baseName),
		filepath.Join(UploadBaseDir, "licenses", baseName),
		filepath.Join("data", "uploads", cleanPath),
		filepath.Join("data", "uploads", cleanKey),
		filepath.Join("data", "uploads", "documents", baseName),
		filepath.Join("data", "uploads", "licenses", baseName),
		filepath.Join("internal", "ui", "data", "uploads", cleanPath),
		filepath.Join("internal", "ui", "data", "uploads", "documents", baseName),
		filepath.Join("cmd", "server", "data", "uploads", cleanPath),
		filepath.Join("cmd", "server", "data", "uploads", "documents", baseName),
		filepath.Join("data", cleanPath),
		filepath.Join("data", cleanKey),
		cleanKey,
		cleanPath,
	}

	for _, cand := range candidates {
		if cand == "" || cand == "." || cand == "/" {
			continue
		}
		info, statErr := os.Stat(cand)
		if statErr == nil && !info.IsDir() {
			f, openErr := os.Open(cand)
			if openErr == nil {
				defer f.Close()

				mimeType := doc.MimeType
				ext := strings.ToLower(filepath.Ext(cand))
				if mimeType == "" || mimeType == "application/octet-stream" {
					switch ext {
					case ".pdf":
						mimeType = "application/pdf"
					case ".png":
						mimeType = "image/png"
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".webp":
						mimeType = "image/webp"
					case ".svg":
						mimeType = "image/svg+xml"
					default:
						mimeType = "application/pdf"
					}
				}

				filename := doc.OriginalName
				if filename == "" {
					filename = filepath.Base(cand)
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
				w.Header().Set("Cache-Control", "private, max-age=3600")
				http.ServeContent(w, r, filename, info.ModTime(), f)
				return
			}
		}
	}

	// 3. Storage client (S3/MinIO) direct proxy stream (fetching object directly from storage without unroutable redirects)
	if h.storage != nil {
		storageKeys := []string{
			cleanKey,
			cleanPath,
			doc.StorageKey,
			fmt.Sprintf("documents/%s", baseName),
			fmt.Sprintf("uploads/documents/%s", baseName),
			fmt.Sprintf("licenses/%s", baseName),
			fmt.Sprintf("uploads/%s", cleanPath),
		}
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			storageKeys = append(storageKeys,
				fmt.Sprintf("orgs/%d/%s", *doc.OrganizationID, cleanPath),
				fmt.Sprintf("orgs/%d/documents/%s", *doc.OrganizationID, baseName),
			)
		}

		for _, sKey := range storageKeys {
			if sKey == "" {
				continue
			}
			body, cType, sErr := h.storage.Get(ctx, sKey)
			if sErr == nil && body != nil {
				defer body.Close()

				mimeType := cType
				if mimeType == "" || mimeType == "application/octet-stream" {
					mimeType = doc.MimeType
				}
				if mimeType == "" || mimeType == "application/octet-stream" {
					ext := strings.ToLower(filepath.Ext(baseName))
					switch ext {
					case ".pdf":
						mimeType = "application/pdf"
					case ".png":
						mimeType = "image/png"
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".webp":
						mimeType = "image/webp"
					case ".svg":
						mimeType = "image/svg+xml"
					default:
						mimeType = "application/pdf"
					}
				}

				filename := doc.OriginalName
				if filename == "" {
					filename = baseName
				}

				disposition := "inline"
				if download {
					disposition = "attachment"
				}

				w.Header().Set("Content-Type", mimeType)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
				w.Header().Set("Cache-Control", "private, max-age=3600")
				_, _ = io.Copy(w, body)
				return
			}
		}
	}

	// 4. If file is unavailable anywhere, render the clear, polished Document Unavailable Error Page
	h.renderMissingDocError(w, r, doc, "لم يتم العثور على الملف الرقمي الفعلي للمستند في وسائط التخزين السحابي.", lang, dir)
}

func (h *UIHandler) renderMissingDocError(w http.ResponseWriter, r *http.Request, doc *attachments.Document, reason, lang, dir string) {
	actor, _ := authctx.From(r.Context())

	returnURL := "/customer/documents"
	if actor.IsPlatformAdmin() || actor.IsStaff {
		returnURL = "/admin/approvals?tab=documents"
	} else if actor.IsVendor() {
		returnURL = "/vendor/documents"
	}

	view := pages.DocumentUnavailableView{
		ReturnURL: returnURL,
		IsAdmin:   actor.IsPlatformAdmin() || actor.IsStaff,
	}

	if doc != nil {
		view.DocID = doc.ID
		view.DocTypeLabel = pages.FormatDocTypeLabel(doc.DocumentType)
		view.OriginalName = doc.OriginalName
		if view.OriginalName == "" {
			view.OriginalName = fmt.Sprintf("Document #%d", doc.ID)
		}
		view.UploadDate = doc.CreatedAt.Format("2006-01-02 15:04")
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			view.OrgID = *doc.OrganizationID
			if h.orgSvc != nil {
				if orgObj, err := h.orgSvc.GetOrganization(r.Context(), *doc.OrganizationID); err == nil && orgObj != nil {
					view.OrgName = orgObj.LegalName
				}
			}
			if view.OrgName == "" {
				view.OrgName = fmt.Sprintf("منشأة #%d", *doc.OrganizationID)
			}
		}
		switch doc.Status {
		case attachments.StatusVerified:
			view.StatusLabel = "معتمد ومطابق"
		case attachments.StatusRejected:
			view.StatusLabel = "مرفوض"
		default:
			view.StatusLabel = "قيد التدقيق"
		}
	} else {
		view.DocTypeLabel = "مستند غير مسجل"
		view.OriginalName = "الملف غير متوفر"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := pages.DocumentUnavailablePage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render document unavailable page", "error", err)
	}
}

// renderOfficialDocSVG dynamically generates an official SVG document badge/receipt.
func renderOfficialDocSVG(doc *attachments.Document) []byte {
	typeNameAr := "مستند رسمي معتمد"
	switch doc.DocumentType {
	case attachments.DocCommercialRegister:
		typeNameAr = "السجل التجاري (Commercial Register)"
	case attachments.DocTaxCard:
		typeNameAr = "البطاقة الضريبية (Tax Card)"
	case attachments.DocPharmacistLicense:
		typeNameAr = "ترخيص مزاولة المهنة للصيدلي (Pharmacist License)"
	case attachments.DocPharmacyLicense:
		typeNameAr = "ترخيص الصيدلية / المنشأة (Pharmacy License)"
	case attachments.DocNationalID:
		typeNameAr = "الهوية الوطنية / بطاقة الرقم القومي (National ID)"
	case attachments.DocPassport:
		typeNameAr = "جواز السفر (Passport)"
	case attachments.DocBankCertificate:
		typeNameAr = "شهادة الحساب البنكي والآيبان (Bank Certificate)"
	case attachments.DocAuthorizationLetter:
		typeNameAr = "خطاب التفويض الرسمي (Authorization Letter)"
	case attachments.DocSyndicateCard:
		typeNameAr = "كارنيه نقابة الصيادلة (Syndicate Card)"
	default:
		typeNameAr = "مستند ترخيص وتوثيق رسمي"
	}

	statusText := "قيد التدقيق الإداري"
	statusColor := "#0284c7"
	statusBg := "#e0f2fe"
	if doc.Status == attachments.StatusVerified {
		statusText = "معتمد ومطابق رسمياً ✓"
		statusColor = "#16a34a"
		statusBg = "#dcfce7"
	} else if doc.Status == attachments.StatusRejected {
		statusText = "مرفوض - بانتظار إعادة الرفع"
		statusColor = "#dc2626"
		statusBg = "#fee2e2"
	}

	orgIDStr := "عام"
	if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
		orgIDStr = fmt.Sprintf("منشأة #%d", *doc.OrganizationID)
	}

	dateStr := doc.CreatedAt.Format("2006-01-02 15:04")
	filename := doc.OriginalName
	if filename == "" {
		filename = fmt.Sprintf("Document #%d", doc.ID)
	}

	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg width="800" height="520" viewBox="0 0 800 520" fill="none" xmlns="http://www.w3.org/2000/svg" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Noto Sans Arabic', sans-serif">
  <!-- Card Background -->
  <rect width="800" height="520" rx="16" fill="#FFFFFF"/>
  <rect x="1" y="1" width="798" height="518" rx="15" stroke="#E2E8F0" stroke-width="2"/>
  
  <!-- Top Header Bar -->
  <path d="M0 16C0 7.16344 7.16344 0 16 0H784C792.837 0 800 7.16344 800 16V80H0V16Z" fill="#0F172A"/>
  <text x="760" y="48" fill="#38BDF8" font-size="22" font-weight="800" text-anchor="end">DAWA24</text>
  <text x="40" y="48" fill="#94A3B8" font-size="14" font-weight="600">منصة دواء24 لتداول وتوثيق الأدوية</text>
  
  <!-- Document Icon & Title -->
  <circle cx="720" cy="140" r="32" fill="#F1F5F9"/>
  <text x="720" y="148" font-size="24" text-anchor="middle">📑</text>
  
  <text x="670" y="132" fill="#0F172A" font-size="20" font-weight="800" text-anchor="end">%s</text>
  <text x="670" y="156" fill="#64748B" font-size="14" font-weight="600" text-anchor="end">الملف: %s</text>
  
  <!-- Status Badge -->
  <rect x="40" y="120" width="220" height="38" rx="19" fill="%s"/>
  <text x="150" y="144" fill="%s" font-size="13" font-weight="800" text-anchor="middle">%s</text>
  
  <!-- Details Container -->
  <rect x="40" y="195" width="720" height="200" rx="12" fill="#F8FAFC" stroke="#E2E8F0"/>
  
  <!-- Row 1 -->
  <text x="720" y="235" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">رقم المستند الرقمي:</text>
  <text x="450" y="235" fill="#0F172A" font-size="14" font-weight="700" text-anchor="end">#%d</text>
  
  <text x="320" y="235" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">المنشأة التابع لها:</text>
  <text x="80" y="235" fill="#0284C7" font-size="14" font-weight="800" text-anchor="start">%s</text>
  
  <!-- Divider -->
  <line x1="60" y1="260" x2="740" y2="260" stroke="#E2E8F0"/>
  
  <!-- Row 2 -->
  <text x="720" y="295" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">تاريخ الرفع والتسجيل:</text>
  <text x="450" y="295" fill="#0F172A" font-size="13" font-weight="700" text-anchor="end">%s</text>
  
  <text x="320" y="295" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">نوع التحقق القانوني:</text>
  <text x="80" y="295" fill="#0F172A" font-size="13" font-weight="700" text-anchor="start">مطابقة هيئة الدواء المصرية</text>
  
  <!-- Divider -->
  <line x1="60" y1="320" x2="740" y2="320" stroke="#E2E8F0"/>
  
  <!-- Row 3 -->
  <text x="720" y="355" fill="#64748B" font-size="13" font-weight="600" text-anchor="end">ملاحظات التدقيق:</text>
  <text x="450" y="355" fill="#334155" font-size="13" font-weight="600" text-anchor="end">%s</text>

  <!-- Footer Seal -->
  <rect x="40" y="425" width="720" height="60" rx="8" fill="#F1F5F9"/>
  <text x="720" y="460" fill="#475569" font-size="12" font-weight="600" text-anchor="end">🔒 هذا المستند مسجل وموثق إلكترونياً بقاعدة بيانات منصة دواء24 الرسمية.</text>
  <text x="60" y="460" fill="#10B981" font-size="13" font-weight="800" text-anchor="start">VERIFIED COMPLIANCE RECORD ✓</text>
</svg>`,
		typeNameAr,
		filename,
		statusBg,
		statusColor,
		statusText,
		doc.ID,
		orgIDStr,
		dateStr,
		doc.ReviewNotes,
	)

	return []byte(svg)
}
