package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorIngestPage renders the primary catalog upload, column mapping, and matching wizard.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	data := pages.IngestWizardData{
		Step:             1,
		NoticeType:       r.URL.Query().Get("notice"),
		NoticeMessage:    r.URL.Query().Get("msg"),
		ConfidenceFilter: r.URL.Query().Get("filter"),
	}

	if h.invSvc != nil && actor.OrganizationID > 0 {
		whs, err := h.invSvc.ListWarehouses(ctx)
		if err == nil {
			data.Warehouses = whs
		}
	}

	if h.ingSvc != nil && actor.OrganizationID > 0 {
		sList, err := h.ingSvc.ListSessions(ctx, actor.OrganizationID, 20, 0)
		if err == nil {
			data.Sessions = sList
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest page", "error", err)
	}
}

// VendorIngestSessionPage loads an ongoing or completed session to display at the exact wizard step.
func (h *UIHandler) VendorIngestSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "sessionID")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	data := pages.IngestWizardData{
		NoticeType:       r.URL.Query().Get("notice"),
		NoticeMessage:    r.URL.Query().Get("msg"),
		ConfidenceFilter: r.URL.Query().Get("filter"),
	}

	if h.invSvc != nil && actor.OrganizationID > 0 {
		whs, err := h.invSvc.ListWarehouses(ctx)
		if err == nil {
			data.Warehouses = whs
		}
	}

	if h.ingSvc != nil && actor.OrganizationID > 0 {
		sList, err := h.ingSvc.ListSessions(ctx, actor.OrganizationID, 20, 0)
		if err == nil {
			data.Sessions = sList
		}
		sItem, err := h.ingSvc.GetSessionProgress(ctx, sessionID)
		if err == nil && sItem != nil {
			data.Session = sItem
			rList, _ := h.ingSvc.ListImportRows(ctx, sessionID, data.ConfidenceFilter, 500, 0)
			data.Rows = rList

			if len(rList) > 0 && rList[0].RawData != nil {
				for k := range rList[0].RawData {
					data.AvailableHeaders = append(data.AvailableHeaders, k)
				}
			}
		}
	}

	if data.Session == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "جلسة الاستيراد غير موجودة.")
		return
	}

	// Find current warehouse
	if data.Session.WarehouseID != nil && len(data.Warehouses) > 0 {
		for _, wh := range data.Warehouses {
			if wh.ID == *data.Session.WarehouseID {
				data.CurrentWarehouse = wh
				break
			}
		}
	}

	// Fetch master products for manual override select dropdowns
	if h.catSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if prods, err := h.catSvc.Search(sysCtx, catalog.SearchParams{Limit: 2000}); err == nil {
			data.MasterProducts = prods
		}
	}

	// Determine wizard step
	reqStep := r.URL.Query().Get("step")
	if reqStep == "2" {
		data.Step = 2
	} else if reqStep == "3" {
		data.Step = 3
	} else if data.Session.Status == ingest.StatusCompleted {
		data.Step = 3
	} else if data.Session.MatchedRows > 0 || data.Session.ReviewRows > 0 || data.Session.UnmatchedRows > 0 {
		data.Step = 3
	} else if len(data.Rows) > 0 {
		data.Step = 2
	} else {
		data.Step = 1
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest session page", "error", err)
	}
}

// VendorIngestUploadSubmit handles the initial file upload, warehouse choice, mode, and switches.
func (h *UIHandler) VendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	// Limit upload size to 50MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "حجم الملف كبير جداً (الحد الأقصى 50 ميجابايت).")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "يرجى اختيار ملف صالح للاستيراد (.xlsx أو .csv).")
		return
	}
	defer file.Close()

	var warehouseID *int64
	whStr := r.PostFormValue("warehouse_id")
	if whID, err := strconv.ParseInt(whStr, 10, 64); err == nil && whID > 0 {
		warehouseID = &whID
	}

	importMode := ingest.ImportMode(r.PostFormValue("import_mode"))
	if importMode == "" {
		importMode = ingest.ModeUpdateAndAdd
	}

	enableAI := r.PostFormValue("enable_ai_matching") == "1" || r.PostFormValue("enable_ai_matching") == "on" || r.PostFormValue("enable_ai_matching") == "true"
	if r.PostFormValue("enable_ai_matching_submitted") == "" && r.PostFormValue("enable_ai_matching") == "" {
		enableAI = true
	}

	enableSavings := r.PostFormValue("enable_savings_matching") == "1" || r.PostFormValue("enable_savings_matching") == "on" || r.PostFormValue("enable_savings_matching") == "true"
	if r.PostFormValue("enable_savings_matching_submitted") == "" && r.PostFormValue("enable_savings_matching") == "" {
		enableSavings = true
	}

	if h.ingSvc != nil {
		fileUpload := &ingest.FileUpload{
			OrganizationID: actor.OrganizationID,
			UserID:         actor.UserID,
			Filename:       header.Filename,
			StorageKey:     fmt.Sprintf("orgs/%d/uploads/%s", actor.OrganizationID, header.Filename),
			FileSizeBytes:  header.Size,
			MimeType:       "application/octet-stream",
		}
		createdUpload, err := h.ingSvc.RegisterUpload(ctx, fileUpload)
		if err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
			return
		}

		session, err := h.ingSvc.StartSessionWithConfig(
			ctx,
			createdUpload.ID,
			nil,
			warehouseID,
			importMode,
			enableAI,
			enableSavings,
			0.85,
		)
		if err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
			return
		}

		// Stream spreadsheet rows into staging and auto-detect columns
		_, err = h.ingSvc.ProcessSpreadsheetStream(ctx, session.ID, file, header.Filename, "", nil)
		if err != nil {
			h.log.WarnContext(ctx, "stream rows warning", "session_id", session.ID, "error", err)
		}

		// Run immediate matching so Step 3 is instantly ready
		masterProducts, savingProducts := h.prepareMatchingData(ctx, actor.OrganizationID)
		if err := h.ingSvc.ExecuteMultiStageMatching(ctx, session.ID, masterProducts, savingProducts); err != nil {
			h.log.WarnContext(ctx, "auto matching warning", "session_id", session.ID, "error", err)
		}

		http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", session.ID), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
}

// VendorIngestMappingSubmit updates column mapping and triggers the multi-stage matching engine.
func (h *UIHandler) VendorIngestMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	mapping := make(map[string]string)

	// Collect form mappings: target field -> header name
	for k, v := range r.PostForm {
		if strings.HasPrefix(k, "target_field_") {
			targetField := strings.TrimPrefix(k, "target_field_")
			if len(v) > 0 && v[0] != "" && v[0] != "unmapped" {
				mapping[targetField] = v[0]
			}
		} else if len(v) > 0 && v[0] != "" && v[0] != "unmapped" {
			mapping[k] = v[0]
		}
	}

	// Validate product_name is mapped
	if mapping[ingest.FieldProductName] == "" {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), "error", "حقل (اسم الصنف / Product Name) إلزامي للمتابعة.")
		return
	}

	if h.ingSvc != nil {
		_ = h.ingSvc.UpdateColumnMapping(ctx, sessionID, mapping)

		// Prepare candidate data
		masterProducts, savingProducts := h.prepareMatchingData(ctx, actor.OrganizationID)

		// Run multi-stage matching
		if err := h.ingSvc.ExecuteMultiStageMatching(ctx, sessionID, masterProducts, savingProducts); err != nil {
			h.log.ErrorContext(ctx, "failed multi-stage matching", "session_id", sessionID, "error", err)
			h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), "error", "حدث خطأ أثناء مطابقة الأصناف: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), "success", "تمت مطابقة الأصناف بنجاح. يمكنك الآن مراجعة النتائج واعتماد الاستيراد.")
}

// VendorIngestRowUpdateSubmit overrides a staged row's matched master product.
func (h *UIHandler) VendorIngestRowUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, _ := strconv.ParseInt(idStr, 10, 64)
	rowIDStr := chi.URLParam(r, "rid")
	rowID, _ := strconv.ParseInt(rowIDStr, 10, 64)

	productIDStr := r.FormValue("product_id")
	productID, _ := strconv.ParseInt(productIDStr, 10, 64)

	if h.ingSvc != nil && rowID > 0 && productID > 0 {
		_ = h.ingSvc.OverrideRowMatchDetailed(ctx, rowID, productID)
	}

	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestRowToggleSubmit toggles whether a row is included/approved for import.
func (h *UIHandler) VendorIngestRowToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, _ := strconv.ParseInt(idStr, 10, 64)
	rowIDStr := chi.URLParam(r, "rid")
	rowID, _ := strconv.ParseInt(rowIDStr, 10, 64)

	approved := r.FormValue("approved") == "1" || r.FormValue("approved") == "true"
	if h.ingSvc != nil && rowID > 0 {
		_ = h.ingSvc.ToggleRowApproval(ctx, rowID, approved)
	}

	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestCommitSubmit commits the session and reconciles catalog variants & warehouse stocks.
func (h *UIHandler) VendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc != nil {
		outcome, err := h.ingSvc.CommitSessionWithReconciliation(ctx, sessionID, h.catSvc, h.invSvc)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to commit ingest session", "session_id", sessionID, "error", err)
			h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), "error", "تعذر إتمام الاستيراد: "+h.safeMessage(err, langOf(r)))
			return
		}

		msg := fmt.Sprintf("تم استيراد واعتماد البيانات بنجاح: تمت إضافة %d صنف جديد، وتحديث %d صنف موجود، وتخطي %d صنف.", outcome.Inserted, outcome.Updated, outcome.Skipped)
		h.redirectWithNotice(w, r, "/vendor/products", "success", msg)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestCancelSubmit cancels the session.
func (h *UIHandler) VendorIngestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc != nil {
		_ = h.ingSvc.CancelSession(ctx, sessionID)
	}

	h.redirectWithNotice(w, r, "/vendor/ingest", "info", "تم إلغاء جلسة الاستيراد.")
}

// VendorIngestRowsPartial returns rows JSON for review.
func (h *UIHandler) VendorIngestRowsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}

	var rows []*ingest.ImportRow
	if h.ingSvc != nil {
		rList, _ := h.ingSvc.ListImportRows(ctx, sessionID, "", 50, 0)
		rows = rList
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows, "count": len(rows)})
}

// prepareMatchingData aggregates master products and saving products for the in-memory matching index.
func (h *UIHandler) prepareMatchingData(ctx context.Context, orgID int64) ([]*ingest.MasterProductData, []*ingest.SavingProductData) {
	var masterList []*ingest.MasterProductData
	var savingList []*ingest.SavingProductData

	if h.catSvc != nil {
		sysCtx := database.AsSystem(ctx)
		prods, err := h.catSvc.Search(sysCtx, catalog.SearchParams{Limit: 10000})
		if err == nil {
			for _, p := range prods {
				if p != nil {
					masterList = append(masterList, &ingest.MasterProductData{
						ID:             p.ID,
						NameAR:         p.Name.Get("ar"),
						NameEN:         p.Name.Get("en"),
						SKU:            p.SKU,
						Barcode:        p.Barcode,
						DosageForm:     p.DosageForm,
						Concentration:  p.Concentration,
						Unit:           p.Unit,
						Manufacturer:   p.ManufacturingCompanies,
						ScientificName: p.ScientificName,
						PublicPrice:    p.Price.String(),
					})
				}
			}
		}

		if orgID > 0 {
			sProds, err := h.catSvc.ListSavingProducts(sysCtx, orgID, 5000, 0)
			if err == nil {
				for _, sp := range sProds {
					if sp != nil && sp.ProductID != nil && *sp.ProductID > 0 {
						savingList = append(savingList, &ingest.SavingProductData{
							ProductID:   *sp.ProductID,
							NameProduct: sp.NameProduct,
							SKU:         sp.SKU,
						})
					}
				}
			}
		}
	}

	return masterList, savingList
}
