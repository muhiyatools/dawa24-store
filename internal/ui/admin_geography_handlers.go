package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminCitiesPage renders the Egyptian cities and spatial coordinates management screen.
func (h *UIHandler) AdminCitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var governorates []*platformadmin.Governorate
	var allCities []*platformadmin.City
	if h.adminSvc != nil {
		governorates, _ = h.adminSvc.ListAllGovernorates(ctx, 1)
		allCities, _ = h.adminSvc.ListAllCities(ctx, 1)
	}
	if len(allCities) == 0 {
		allCities = h.listCities(ctx)
	}

	selectedGovID, _ := strconv.ParseInt(r.URL.Query().Get("gov_id"), 10, 64)
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Filter cities by governorate and search query
	filteredCities := make([]*platformadmin.City, 0, len(allCities))
	for _, c := range allCities {
		if selectedGovID > 0 && (c.GovernorateID == nil || *c.GovernorateID != selectedGovID) {
			continue
		}
		if query != "" {
			qLower := strings.ToLower(query)
			nameAr := c.Name["ar"]
			nameEn := strings.ToLower(c.Name["en"])
			govAr := ""
			govEn := ""
			if c.GovernorateName != nil {
				govAr = (*c.GovernorateName)["ar"]
				govEn = strings.ToLower((*c.GovernorateName)["en"])
			}
			if !strings.Contains(nameAr, query) && !strings.Contains(nameEn, qLower) &&
				!strings.Contains(govAr, query) && !strings.Contains(govEn, qLower) {
				continue
			}
		}
		filteredCities = append(filteredCities, c)
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	totalFiltered := len(filteredCities)
	var paginatedCities []*platformadmin.City
	var totalPages int = 1

	if limit >= 1000 {
		paginatedCities = filteredCities
		totalPages = 1
	} else {
		totalPages = (totalFiltered + limit - 1) / limit
		if totalPages < 1 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
		start := (page - 1) * limit
		if start < 0 {
			start = 0
		}
		end := start + limit
		if end > totalFiltered {
			end = totalFiltered
		}
		if start < totalFiltered {
			paginatedCities = filteredCities[start:end]
		}
	}

	data := pages.AdminCitiesData{
		Governorates:          governorates,
		Cities:                paginatedCities,
		SelectedGovernorateID: selectedGovID,
		TotalCities:           len(allCities),
		TotalGovernorates:     len(governorates),
		TotalFiltered:         totalFiltered,
		Page:                  page,
		Limit:                 limit,
		TotalPages:            totalPages,
		Query:                 query,
	}

	h.renderPage(ctx, w, "render admin cities page", pages.AdminCities(data, lang, dir, h.isHTMX(r)))
}

// AdminCityCreateSubmit adds a new city / district with coordinates under a governorate.
func (h *UIHandler) AdminCityCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	var govIDPtr *int64
	govID, _ := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64)
	if govID > 0 {
		govIDPtr = &govID
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("city_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("city_lon"), 64)

	city := &platformadmin.City{
		CountryID:     1,
		GovernorateID: govIDPtr,
		Name:          i18n.New(nameAr, nameEn),
		Latitude:      lat,
		Longitude:     lon,
		IsActive:      true,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.CreateCity(ctx, city); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	redirectURL := "/admin/cities"
	if govID > 0 {
		redirectURL = fmt.Sprintf("/admin/cities?gov_id=%d", govID)
	}
	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "admin.geo.city_created_success"))
}

// AdminCityToggleSubmit toggles the active status of a city.
func (h *UIHandler) AdminCityToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_invalid_id"))
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.ToggleCityStatus(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.city_status_updated_success"))
}

// AdminCityEditSubmit updates an existing city / district.
func (h *UIHandler) AdminCityEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_invalid_id"))
		return
	}

	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.city_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	var govIDPtr *int64
	govID, _ := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64)
	if govID > 0 {
		govIDPtr = &govID
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("city_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("city_lon"), 64)
	isCapital := r.PostFormValue("is_capital") == "true" || r.PostFormValue("is_capital") == "1" || r.PostFormValue("is_capital") == "on"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "on"

	city := &platformadmin.City{
		ID:            id,
		CountryID:     1,
		GovernorateID: govIDPtr,
		Name:          i18n.New(nameAr, nameEn),
		Latitude:      lat,
		Longitude:     lon,
		IsActive:      isActive,
		IsCapital:     isCapital,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpdateCity(ctx, city); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.city_updated_success"))
}

// AdminGovernorateCreateSubmit adds a new main governorate.
func (h *UIHandler) AdminGovernorateCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("gov_name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("gov_name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("gov_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("gov_lon"), 64)

	gov := &platformadmin.Governorate{
		CountryID: 1,
		Name:      i18n.New(nameAr, nameEn),
		Latitude:  lat,
		Longitude: lon,
		IsActive:  true,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.CreateGovernorate(ctx, gov); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", i18n.T(lang, "admin.geo.gov_created_success"))
}

// AdminGovernorateEditSubmit updates an existing governorate.
func (h *UIHandler) AdminGovernorateEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_invalid_id"))
		return
	}

	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("gov_name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("gov_name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_name_ar_required"))
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("gov_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("gov_lon"), 64)
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "on"

	gov := &platformadmin.Governorate{
		ID:        id,
		CountryID: 1,
		Name:      i18n.New(nameAr, nameEn),
		Latitude:  lat,
		Longitude: lon,
		IsActive:  isActive,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.UpdateGovernorate(ctx, gov); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/cities"
	}
	h.redirectWithNotice(w, r, referer, "success", i18n.T(lang, "admin.geo.gov_updated_success"))
}

// AdminGovernorateToggleSubmit toggles the active status of a governorate.
func (h *UIHandler) AdminGovernorateToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", i18n.T(lang, "admin.geo.gov_invalid_id"))
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.ToggleGovernorateStatus(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", i18n.T(lang, "admin.geo.gov_status_updated_success"))
}
