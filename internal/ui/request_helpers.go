package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SiteSettingsMiddleware injects live SiteSettings from database into every request context.
func (h *UIHandler) SiteSettingsMiddleware(next http.Handler) http.Handler {
	return h.siteSettingsMiddleware(next)
}

func (h *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	h.log.ErrorContext(ctx, "ui error rendering page", "error", err, "path", r.URL.Path)

	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// err.Error() renders the internal form - "conflict [order.already_confirmed]:
	// ... (detail)" for an apperr, and for anything else the raw driver text,
	// which names tables, columns and constraints. apperr.Msg is documented as
	// user-safe and LocalizedMsg gives the Arabic wording by code; anything that
	// is not an apperr gets a generic message and lives only in the log above.
	msg := h.safeMessage(err, lang)

	if h.isHTMX(r) {
		// 200 on purpose: HTMX swaps the error state into the target region. A
		// non-2xx would leave the old content in place with nothing explaining why.
		w.WriteHeader(http.StatusOK)
		if rerr := components.ErrorState(components.ErrorStateProps{
			Title:      "حدث خطأ أثناء تحميل البيانات",
			Message:    msg,
			RetryURL:   r.URL.String(),
			RetryLabel: "إعادة المحاولة",
		}).Render(ctx, w); rerr != nil {
			h.log.ErrorContext(ctx, "render error state", "error", rerr)
		}
		return
	}

	w.WriteHeader(statusForError(err))
	if rerr := pages.ErrorPage(
		"عذراً، حدث خطأ",
		msg,
		"/",
		lang,
		dir,
	).Render(ctx, w); rerr != nil {
		h.log.ErrorContext(ctx, "render error page", "error", rerr)
	}
}

// safeMessage returns wording that may be shown to a user.
func (h *UIHandler) safeMessage(err error, lang string) string {
	if err == nil {
		return ""
	}
	if appErr, ok := apperr.As(err); ok {
		return appErr.LocalizedMsg(lang)
	}
	errStr := err.Error()
	if strings.Contains(errStr, "email") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505") || strings.Contains(errStr, "users_email_key")) {
		return "البريد الإلكتروني مسجل مسبقاً في النظام. يرجى تسجيل الدخول أو استخدام بريد آخر."
	}
	if strings.Contains(errStr, "commercial_register") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505")) {
		return "رقم السجل التجاري مسجل مسبقاً لمنشأة أخرى."
	}
	if strings.Contains(errStr, "city_id") || strings.Contains(errStr, "branches_city_id_fkey") {
		return "بيانات الموقع أو المدينة غير صالحة. يرجى إعادة اختيار المدينة من الخريطة."
	}
	if strings.Contains(errStr, "order_shipments_organization_id_fkey") || strings.Contains(errStr, "order_lines_organization_id_fkey") {
		return "تعذر تحديد بيانات شركة التوريد المسؤولة عن هذا الصنف (رمز المورد غير مسجل). يرجى مراجعة الأصناف بالسلة."
	}
	if strings.Contains(errStr, "orders_branch_id_fkey") || strings.Contains(errStr, "order_shipments_branch_id_fkey") {
		return "فرع الصيدلية المحدد غير صالح أو تم حذفه. يرجى اختيار فرع صيدلية نشط."
	}
	if strings.Contains(errStr, "orders_vendor_branch_id_fkey") {
		return "فرع التوريد المحدد للمورد غير صالح أو غير مسجل."
	}
	if strings.Contains(errStr, "foreign key") || strings.Contains(errStr, "23503") {
		return "تعذر إتمام العملية بسبب عدم تطابق البيانات المرجعية (" + errStr + ")."
	}
	if lang == "ar" {
		return "حدث خطأ أثناء المعالجة: " + errStr
	}
	return "Operation could not be completed: " + errStr
}

// statusForError maps an error onto a response code. A full page load that
// returns 200 for a failure is invisible to uptime checks and to the browser.
func statusForError(err error) int {
	switch apperr.KindOf(err) {
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// redirectWithNotice sends the user on with a message to show when they land.
//
// Form posts here redirect after handling, which is correct — it stops a
// refresh from resubmitting. But it also throws away everything the handler
// learned, which is why a failed save was indistinguishable from a successful
func (h *UIHandler) redirectWithNotice(w http.ResponseWriter, r *http.Request, path, kind, message string) {
	u, err := url.Parse(path)
	if err != nil {
		http.Redirect(w, r, path, http.StatusSeeOther)
		return
	}
	q := u.Query()
	q.Set("notice", kind)
	q.Set("msg", message)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

// SetLanguage persists the chosen UI language in the dawa24_lang cookie and
// returns the user to where they were. Signed-in users get the same choice
// written to their profile preference via UpdateProfile when they save settings.
func (h *UIHandler) SetLanguage(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code != "en" {
		code = "ar"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_lang",
		Value:    code,
		Path:     "/",
		MaxAge:   86400 * 365,
		SameSite: http.SameSiteLaxMode,
	})
	back := r.Header.Get("Referer")
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// langOf is the language alone, for callers that do not need the direction.
func langOf(r *http.Request) string {
	if r.URL.Query().Get("lang") == "en" {
		return "en"
	}
	return "ar"
}

func (h *UIHandler) pageLimit(r *http.Request) int {
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 100 {
		return 20
	}
	return lim
}

func (h *UIHandler) pageOffset(r *http.Request) int {
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if off < 0 {
		return 0
	}
	return off
}

func (h *UIHandler) isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// localeAndDir resolves the request language and text direction.
//
// Precedence: query ?lang= → dawa24_lang cookie → Accept-Language → Arabic.
// (User preference from profile.user_preferences is layered in later once the
// settings surface exists; the cookie already persists the choice for signed-out
// visitors.) Arabic is the default and the primary language of the marketplace.
func (h *UIHandler) localeAndDir(r *http.Request) (string, string) {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return dirForLang(lang)
	}
	if cookie, err := r.Cookie("dawa24_lang"); err == nil && cookie.Value != "" {
		return dirForLang(cookie.Value)
	}
	if header := r.Header.Get("Accept-Language"); header != "" {
		if lang := acceptLanguage(header); lang != "" {
			return dirForLang(lang)
		}
	}
	return "ar", "rtl"
}

// dirForLang returns the language and the matching text direction. Unknown
// values fall back to Arabic rather than erroring — language is a display
// preference, never a request failure.
func dirForLang(lang string) (string, string) {
	if lang == "en" {
		return "en", "ltr"
	}
	return "ar", "rtl"
}

// acceptLanguage maps an Accept-Language header onto a supported language by
// taking the first weighted entry, ignoring the rest. It is a best effort: a
// browser sending "fr-CH, fr;q=0.9" simply gets Arabic.
func acceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if lang == "en" || lang == "ar" {
			return lang
		}
	}
	return ""
}

// redirectToSettingsTab permanently redirects a retired settings sub-page to
// its tab or dedicated surface.
func redirectToSettingsTab(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		actor, hasActor := authctx.From(ctx)

		if tab == "organization" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/organization", http.StatusMovedPermanently)
				return
			}
		}
		if tab == "wallet" || tab == "payments" {
			if hasActor && actor.UserID > 0 {
				http.Redirect(w, r, walletDestFor(actor), http.StatusMovedPermanently)
				return
			}
			http.Redirect(w, r, "/customer/wallet", http.StatusMovedPermanently)
			return
		}
		if tab == "security" || tab == "sessions" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/sessions", http.StatusMovedPermanently)
				return
			}
			http.Redirect(w, r, "/customer/sessions", http.StatusMovedPermanently)
			return
		}
		if tab == "profile" || tab == "preferences" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/settings?tab="+tab, http.StatusMovedPermanently)
	}
}
