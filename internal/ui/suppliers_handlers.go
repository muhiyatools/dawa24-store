package ui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// computeVendorWorkingStatus evaluates working hours, open/closed status, and coverage schedule.
func computeVendorWorkingStatus(branches []*org.Branch, coverages []*workflow.CoverageView) (workingHours string, coverageDays string, coverageAreas []string, isOpenNow bool, statusNote string) {
	now := time.Now().UTC().Add(3 * time.Hour) // Egypt Time UTC+3 / EET
	currentWeekday := int(now.Weekday())        // 0=Sun, 1=Mon, ..., 6=Sat
	currentHourMin := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())

	workingHours = "09:00 ص - 06:00 م"
	coverageDays = "السبت - الخميس"
	isOpenNow = false
	statusNote = "مغلق حالياً"

	areaMap := make(map[string]bool)
	dayActiveMap := make(map[int]bool)
	var fromTime, toTime string

	for _, c := range coverages {
		if c == nil || !c.IsActive {
			continue
		}
		dayActiveMap[c.DayOfWeek] = true
		if c.GovernorateNameAr != "" {
			areaMap[c.GovernorateNameAr] = true
		} else if c.CityNameAr != "" {
			areaMap[c.CityNameAr] = true
		}
		if c.CoverageFrom != nil && *c.CoverageFrom != "" {
			fromTime = *c.CoverageFrom
		}
		if c.CoverageTo != nil && *c.CoverageTo != "" {
			toTime = *c.CoverageTo
		}
	}

	for a := range areaMap {
		coverageAreas = append(coverageAreas, a)
	}
	if len(coverageAreas) == 0 {
		for _, b := range branches {
			if b != nil && b.Address != "" {
				coverageAreas = append(coverageAreas, b.Address)
				break
			}
		}
	}
	if len(coverageAreas) == 0 {
		coverageAreas = []string{"القاهرة الكبرى", "كافة المحافظات"}
	}

	if fromTime != "" && toTime != "" {
		workingHours = fmt.Sprintf("%s - %s", fromTime, toTime)
	} else {
		for _, b := range branches {
			if b != nil && b.OperatingHours != "" {
				workingHours = b.OperatingHours
				break
			}
		}
	}

	if len(dayActiveMap) >= 6 {
		coverageDays = "طوال أيام الأسبوع 24/7"
	} else if len(dayActiveMap) > 0 {
		dayNames := []string{"الأحد", "الاثنين", "الثلاثاء", "الأربعاء", "الخميس", "الجمعة", "السبت"}
		var activeDays []string
		for d := 0; d <= 6; d++ {
			if dayActiveMap[d] {
				activeDays = append(activeDays, dayNames[d])
			}
		}
		if len(activeDays) > 0 {
			if len(activeDays) == 1 {
				coverageDays = "يوم " + activeDays[0]
			} else {
				coverageDays = activeDays[0] + " - " + activeDays[len(activeDays)-1]
			}
		}
	}

	if len(dayActiveMap) > 0 {
		if dayActiveMap[currentWeekday] {
			start := "08:00"
			end := "19:00"
			if fromTime != "" {
				start = fromTime
			}
			if toTime != "" {
				end = toTime
			}
			if currentHourMin >= start && currentHourMin <= end {
				isOpenNow = true
				statusNote = fmt.Sprintf("مفتوح الآن (يغلق %s)", end)
			} else if currentHourMin < start {
				isOpenNow = false
				statusNote = fmt.Sprintf("مغلق حالياً (يفتح %s)", start)
			} else {
				isOpenNow = false
				statusNote = "مغلق الآن (انتهت ساعات العمل)"
			}
		} else {
			isOpenNow = false
			statusNote = "مغلق اليوم (خارج أيام التغطية)"
		}
	} else {
		if currentWeekday == 5 { // Friday
			isOpenNow = false
			statusNote = "مغلق اليوم (عطلة الجمعة)"
		} else {
			if currentHourMin >= "09:00" && currentHourMin <= "18:00" {
				isOpenNow = true
				statusNote = "مفتوح الآن (يغلق 06:00 م)"
			} else if currentHourMin < "09:00" {
				isOpenNow = false
				statusNote = "مغلق حالياً (يفتح 09:00 ص)"
			} else {
				isOpenNow = false
				statusNote = "مغلق الآن (يفتح 09:00 ص غداً)"
			}
		}
	}

	return workingHours, coverageDays, coverageAreas, isOpenNow, statusNote
}

func resolveSupplierCoordinates(sID int64, branches []*org.Branch, coverages []*workflow.CoverageView) (lat, lng float64, hasCoords bool) {
	for _, b := range branches {
		if b != nil && b.Latitude != nil && b.Longitude != nil && *b.Latitude != 0 && *b.Longitude != 0 {
			return *b.Latitude, *b.Longitude, true
		}
	}
	for _, c := range coverages {
		if c != nil && c.Latitude != nil && c.Longitude != nil && *c.Latitude != 0 && *c.Longitude != 0 {
			return *c.Latitude, *c.Longitude, true
		}
	}
	baseLat := 30.0444 + float64((sID*7)%100)*0.003
	baseLng := 31.2357 + float64((sID*13)%100)*0.003
	return baseLat, baseLng, true
}

// SuppliersPage renders the public supplier directory.
func (h *UIHandler) SuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	viewTab := r.URL.Query().Get("view")
	if viewTab == "" {
		viewTab = "list"
	}

	data := pages.SupplierDirectoryData{
		Query:     q,
		ActiveTab: viewTab,
	}

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		typ := org.TypeVendor
		orgs, _ := h.orgSvc.ListOrganizations(sysCtx, &typ, nil, 100, 0)
		if len(orgs) == 0 {
			allOrgs, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 200, 0)
			for _, o := range allOrgs {
				if o != nil && (o.Type == org.TypeVendor || string(o.Type) == "supplier" || string(o.Type) == "distributor") {
					orgs = append(orgs, o)
				}
			}
		}

		var items []*pages.SupplierDirectoryItem
		for _, o := range orgs {
			if o == nil {
				continue
			}
			if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
				continue
			}
			if q != "" {
				nameAr := strings.ToLower(o.TradeName.Get(i18n.AR))
				nameEn := strings.ToLower(o.TradeName.Get(i18n.EN))
				legal := strings.ToLower(o.LegalName)
				cr := strings.ToLower(o.CommercialRegister)
				if !strings.Contains(nameAr, q) && !strings.Contains(nameEn, q) && !strings.Contains(legal, q) && !strings.Contains(cr, q) {
					continue
				}
			}

			branches, _ := h.orgSvc.ListBranches(sysCtx, o.ID)
			var mainBranch *org.Branch
			for _, b := range branches {
				if b != nil && b.IsMain {
					mainBranch = b
					break
				}
			}
			if mainBranch == nil && len(branches) > 0 {
				mainBranch = branches[0]
			}

			var coverages []*workflow.CoverageView
			if h.wfSvc != nil {
				coverages, _ = h.wfSvc.ListCoverageForOrganization(sysCtx, o.ID)
			}

			workingHours, coverageDays, coverageAreas, isOpenNow, statusNote := computeVendorWorkingStatus(branches, coverages)
			lat, lng, hasCoords := resolveSupplierCoordinates(o.ID, branches, coverages)

			rating := 5.0
			if o.Rating > 0 {
				rating = float64(o.Rating)
			}

			phone := ""
			addr := ""
			if mainBranch != nil {
				phone = mainBranch.Phone
				addr = mainBranch.Address
			}

			item := &pages.SupplierDirectoryItem{
				Org:            o,
				Branches:       branches,
				MainBranch:     mainBranch,
				Coverages:      coverages,
				WorkingHours:   workingHours,
				CoverageDays:   coverageDays,
				CoverageAreas:  coverageAreas,
				CoverageRadius: 50,
				IsOpenNow:      isOpenNow,
				StatusNote:     statusNote,
				Latitude:       lat,
				Longitude:      lng,
				HasCoordinates: hasCoords,
				MinOrderPrice:  o.MinOrderPrice,
				Rating:         rating,
				ReviewCount:    0,
				Phone:          phone,
				Address:        addr,
			}

			items = append(items, item)
		}
		data.Suppliers = items
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SuppliersDirectory(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render suppliers directory", "error", err)
	}
}

// FollowedSuppliersPage renders the list of suppliers followed by the current user.
func (h *UIHandler) FollowedSuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/suppliers/followed", http.StatusSeeOther)
		return
	}

	var suppliers []*org.Organization
	if h.orgSvc != nil {
		suppliers, _ = h.orgSvc.ListFollowedOrganizations(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerFollowedSuppliers(suppliers, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render followed suppliers page", "error", err)
	}
}

// SupplierProfilePage renders a supplier's public profile: catalogue, reviews
// and policies.
func (h *UIHandler) SupplierProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.orgSvc == nil {
		h.renderError(w, r, err)
		return
	}

	sysCtx := database.AsSystem(ctx)
	o, err := h.orgSvc.GetOrganization(sysCtx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// Allow approved or active suppliers
	if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
		h.renderError(w, r, fmt.Errorf("المورد غير متاح حالياً"))
		return
	}

	branches, _ := h.orgSvc.ListBranches(sysCtx, id)
	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		coverages, _ = h.wfSvc.ListCoverageForOrganization(sysCtx, id)
	}
	workingHours, coverageDays, coverageAreas, isOpenNow, statusNote := computeVendorWorkingStatus(branches, coverages)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}
	limit := 24
	offset := (page - 1) * limit

	data := pages.SupplierProfileData{
		Org:           o,
		Branches:      branches,
		Coverages:     coverages,
		WorkingHours:  workingHours,
		CoverageDays:  coverageDays,
		CoverageAreas: coverageAreas,
		IsOpenNow:     isOpenNow,
		StatusNote:    statusNote,
		CurrentPage:   page,
		SearchQuery:   q,
	}

	if h.catSvc != nil {
		variants, total, err := h.catSvc.ListVariantsByOrganization(ctx, id, catalog.VariantSearchParams{
			Query:  q,
			Limit:  limit,
			Offset: offset,
		})
		if err == nil {
			data.Variants = variants
			data.TotalVariants = total
			if total > 0 {
				data.TotalPages = int(math.Ceil(float64(total) / float64(limit)))
			} else {
				data.TotalPages = 1
			}

			if len(variants) > 0 {
				data.ProductsMap = make(map[int64]*catalog.Product)
				for _, v := range variants {
					if v != nil && v.ProductID > 0 {
						if _, ok := data.ProductsMap[v.ProductID]; !ok {
							if p, _, err := h.catSvc.GetProduct(ctx, v.ProductID); err == nil && p != nil {
								data.ProductsMap[v.ProductID] = p
							}
						}
					}
				}
			}
		}
	}

	if h.promoSvc != nil {
		data.Sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, id)
	}
	if h.orgSvc != nil {
		data.Reviews, _ = h.orgSvc.ListReviews(ctx, id, 20, 0)
		data.Policies, _ = h.orgSvc.ListPolicies(ctx, id)
		if actor, ok := authctx.From(ctx); ok {
			data.IsFollowing, _ = h.orgSvc.IsFollowing(ctx, id, actor.UserID)
		}
	}
	data.ReviewCount = len(data.Reviews)
	if data.ReviewCount > 0 {
		var sum int
		for _, rv := range data.Reviews {
			sum += rv.Rating
		}
		data.Rating = float64(sum) / float64(data.ReviewCount)
	} else {
		data.Rating = 0
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SupplierProfile(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render supplier profile", "error", err)
	}
}

// SupplierFollowSubmit toggles following for the signed-in user.
func (h *UIHandler) SupplierFollowSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_, _ = h.orgSvc.ToggleFollow(ctx, id, userID)
	}

	back := r.Referer()
	if back == "" {
		back = "/suppliers"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// SupplierQuoteSubmit creates a bulk quote request addressed to a supplier.
func (h *UIHandler) SupplierQuoteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}
	if actor.OrganizationID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", "تحتاج إلى حساب مؤسسة معتمد لطلب عرض سعر.")
		return
	}

	supplierID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", "أدخل كمية صحيحة.")
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", "الخدمة غير متاحة حالياً.")
		return
	}

	var productID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64); err == nil && pid > 0 {
		productID = &pid
	}

	_, err := h.commSvc.CreateQuoteRequest(ctx, &commerce.QuoteRequest{
		OrganizationID:    supplierID,
		CustomerOrgID:     actor.OrganizationID,
		ProductID:         productID,
		RequestedQuantity: qty,
		BuyerNotes:        r.PostFormValue("notes"),
	})
	if err != nil {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "success", "تم إرسال طلب عرض السعر.")
}
