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
	country, city := detectCountryAndCity(r)

	_ = h.adminSvc.RecordVisitor(r.Context(), &platformadmin.Visitor{
		VisitorKey: key,
		IP:         truncateIP(r.RemoteAddr),
		UserAgent:  r.UserAgent(),
		Browser:    browser,
		Device:     device,
		OS:         osName,
		Country:    country,
		City:       city,
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

// detectCountryAndCity determines geographic info from headers and network address.
func detectCountryAndCity(r *http.Request) (country, city string) {
	// 1. Check CDN / Cloudflare / Proxy headers
	if cfCountry := strings.TrimSpace(r.Header.Get("CF-IPCountry")); cfCountry != "" && len(cfCountry) == 2 {
		country = mapCountryCode(cfCountry)
	}
	if country == "" {
		if xCountry := strings.TrimSpace(r.Header.Get("X-Country-Code")); xCountry != "" {
			country = mapCountryCode(xCountry)
		}
	}
	if city == "" {
		city = cleanCityName(strings.TrimSpace(r.Header.Get("CF-IPCity")))
		if city == "" {
			city = cleanCityName(strings.TrimSpace(r.Header.Get("X-City-Name")))
		}
	}

	// 2. Check if private or local address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if country == "" {
			country = "مصر 🇪🇬"
		}
		if city == "" {
			city = "القاهرة"
		}
		return country, city
	}

	// 3. Check Accept-Language header if still undetermined
	if country == "" {
		al := strings.ToLower(r.Header.Get("Accept-Language"))
		switch {
		case strings.Contains(al, "ar-eg"), strings.Contains(al, "eg"):
			country = "مصر 🇪🇬"
			if city == "" {
				city = "القاهرة"
			}
		case strings.Contains(al, "ar-sa"), strings.Contains(al, "sa"):
			country = "السعودية 🇸🇦"
			if city == "" {
				city = "الرياض"
			}
		case strings.Contains(al, "ar-ae"), strings.Contains(al, "ae"):
			country = "الإمارات 🇦🇪"
			if city == "" {
				city = "دبي"
			}
		case strings.Contains(al, "ar-kw"), strings.Contains(al, "kw"):
			country = "الكويت 🇰🇼"
			if city == "" {
				city = "الكويت العاصمة"
			}
		case strings.Contains(al, "ar-jo"), strings.Contains(al, "jo"):
			country = "الأردن 🇯🇴"
			if city == "" {
				city = "عمان"
			}
		case strings.Contains(al, "en-us"):
			country = "الولايات المتحدة 🇺🇸"
			if city == "" {
				city = "نيويورك"
			}
		case strings.Contains(al, "en-gb"):
			country = "المملكة المتحدة 🇬🇧"
			if city == "" {
				city = "لندن"
			}
		default:
			country = "مصر 🇪🇬"
			if city == "" {
				city = "القاهرة"
			}
		}
	}

	if city == "" {
		if strings.Contains(country, "مصر") {
			city = "القاهرة"
		} else if strings.Contains(country, "السعودية") {
			city = "الرياض"
		} else {
			city = "المركز الرئيسي"
		}
	}
	return country, city
}

func cleanCityName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "غير محدد" || strings.Contains(raw, "Local") {
		return ""
	}
	switch strings.ToLower(raw) {
	case "cairo", "al qahirah", "el qahira":
		return "القاهرة"
	case "giza", "al jizah":
		return "الجيزة"
	case "alexandria", "al iskandariyah":
		return "الإسكندرية"
	case "mansoura", "al mansurah":
		return "المنصورة"
	case "tanta":
		return "طنطا"
	case "asyut", "assiut":
		return "أسيوط"
	case "zagazig", "az zaqaziq":
		return "الزقازيق"
	case "ismailia", "al ismailiyah":
		return "الإسماعيلية"
	case "suez", "as suways":
		return "السويس"
	case "port said", "bur said":
		return "بورسعيد"
	case "damietta", "dumyat":
		return "دمياط"
	case "aswan":
		return "أسوان"
	case "luxor", "al uqsur":
		return "الأقصر"
	case "hurghada", "al ghardaqah":
		return "الغردقة"
	case "sharm el-sheikh", "sharm ash shaykh":
		return "شرم الشيخ"
	case "riyadh", "ar riyad":
		return "الرياض"
	case "jeddah":
		return "جدة"
	case "mecca", "makkah":
		return "مكة المكرمة"
	case "medina", "madinah":
		return "المدينة المنورة"
	case "dammam", "ad dammam":
		return "الدمام"
	case "dubai":
		return "دبي"
	case "abu dhabi":
		return "أبوظبي"
	case "doha":
		return "الدوحة"
	case "kuwait", "kuwait city":
		return "الكويت العاصمة"
	case "amman":
		return "عمان"
	default:
		return raw
	}
}

func mapCountryCode(code string) string {
	switch strings.ToUpper(code) {
	case "EG":
		return "مصر 🇪🇬"
	case "SA":
		return "السعودية 🇸🇦"
	case "AE":
		return "الإمارات 🇦🇪"
	case "KW":
		return "الكويت 🇰🇼"
	case "JO":
		return "الأردن 🇯🇴"
	case "OM":
		return "عمان 🇴🇲"
	case "QA":
		return "قطر 🇶🇦"
	case "BH":
		return "البحرين 🇧🇭"
	case "IQ":
		return "العراق 🇮🇶"
	case "LY":
		return "ليبيا 🇱🇾"
	case "SD":
		return "السودان 🇸🇩"
	case "US":
		return "الولايات المتحدة 🇺🇸"
	case "GB", "UK":
		return "المملكة المتحدة 🇬🇧"
	case "DE":
		return "ألمانيا 🇩🇪"
	default:
		return strings.ToUpper(code)
	}
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
