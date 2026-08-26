package ui

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The vendor catalogue import screen.
//
// Seven stages behind six handlers. Everything a stage needs is re-derived from
// the stored file on each request, which costs a few hundred milliseconds and
// buys the guarantee that matters: the mapping the vendor is looking at is the
// mapping that will run.

// maxImportUpload bounds the multipart body. It matches the service's own limit
// so an oversized file is refused by the reader rather than after being read.
const maxImportUpload = ingest.MaxImportBytes

// VendorIngestPage renders the upload screen and the import history.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	view := pages.VendorImportView{
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	view.Warehouses = h.vendorWarehouses(r)
	if h.ingSvc != nil && actor.OrganizationID > 0 {
		recent, err := h.ingSvc.RecentImports(ctx, actor.OrganizationID, 12)
		if err != nil {
			h.log.WarnContext(ctx, "import history unavailable", "error", err)
		}
		view.Recent = recent
	}
	h.renderImport(w, r, view)
}

// VendorIngestSessionPage renders whichever stage the import has reached.
func (h *UIHandler) VendorIngestSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}

	session, err := h.ingSvc.LoadImport(ctx, publicID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
		return
	}

	view := pages.VendorImportView{
		Session:       session,
		Warehouses:    h.vendorWarehouses(r),
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	view.AIAvailable, view.AIUnavailableReason = h.vendorImportAIState(ctx)

	switch {
	case session.Phase.Terminal():
		h.loadImportResults(r, &view)
	case session.Phase == ingest.PhaseProcessing:
		// Nothing to analyse: the run owns the file and the screen polls.
	default:
		_, analysis, aErr := h.ingSvc.Analysis(ctx, publicID)
		if aErr != nil {
			view.Fatal = h.safeMessage(aErr, langOf(r))
		} else {
			view.Analysis = analysis
		}
	}
	h.renderImport(w, r, view)
}

// VendorIngestUploadSubmit analyses an uploaded file and opens an import.
func (h *UIHandler) VendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			"تعذر قراءة الملف المرفوع — قد يتجاوز حجمه الحد المسموح (25 ميجابايت).")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			"يرجى اختيار ملف صالح للاستيراد (.xlsx أو .xls أو CSV).")
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "تعذر قراءة محتوى الملف.")
		return
	}

	session, _, err := h.ingSvc.StartImport(ctx, actor.UserID, header.Filename, content)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+session.PublicID, http.StatusSeeOther)
}

// VendorIngestMappingSubmit records the vendor's column corrections.
func (h *UIHandler) VendorIngestMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "تعذر قراءة النموذج المرسل.")
		return
	}

	overrides := map[int]productmatch.Field{}
	for key, values := range r.PostForm {
		if !strings.HasPrefix(key, "column_") || len(values) == 0 {
			continue
		}
		index, convErr := strconv.Atoi(strings.TrimPrefix(key, "column_"))
		if convErr != nil {
			continue
		}
		switch values[0] {
		case "":
			// "Decide for me" — left out so the completion pass may bind it.
		case "__ignore":
			overrides[index] = productmatch.IgnoreField
		case "__none":
			overrides[index] = ""
		default:
			overrides[index] = productmatch.Field(values[0])
		}
	}

	if _, _, err := h.ingSvc.SaveMapping(ctx, publicID, overrides); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// VendorIngestSettingsSubmit records the import rules.
func (h *UIHandler) VendorIngestSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "تعذر قراءة النموذج المرسل.")
		return
	}

	settings := ingest.DefaultSettings()
	settings.WarehouseID, _ = strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64)
	settings.Mode = ingest.ParseMode(r.PostFormValue("mode"))
	settings.StockMode = inventory.StockMode(r.PostFormValue("stock_mode"))
	settings.Duplicates = productmatch.DuplicatePolicy(r.PostFormValue("duplicates"))
	if score, err := strconv.ParseFloat(r.PostFormValue("min_match_score"), 64); err == nil {
		settings.MinMatchScore = score / 100
	}
	settings.TrustSupplierCode = checked(r, "trust_supplier_code")
	settings.BlankQuantityIsZero = checked(r, "blank_quantity_is_zero")
	settings.InferDosageForm = checked(r, "infer_dosage_form")
	settings.InferConcentration = checked(r, "infer_concentration")
	settings.RejectExpired = checked(r, "reject_expired")
	settings.MarkNegotiable = checked(r, "mark_negotiable")
	settings.PublishImmediately = checked(r, "publish_immediately")
	// A vendor cannot switch on a tier the platform cannot run: the checkbox is
	// disabled in that case and submits nothing, and honouring an absent value
	// as "on" would make the results screen claim AI work that never happened.
	settings.UseAI = checked(r, "use_ai") && h.ingSvc.AIAvailable()
	settings.RecordRows = checked(r, "record_rows")
	if v, err := strconv.Atoi(r.PostFormValue("default_min_order_qty")); err == nil {
		settings.DefaultMinOrderQty = v
	}
	if v, err := strconv.Atoi(r.PostFormValue("default_min_threshold")); err == nil {
		settings.DefaultMinThreshold = v
	}

	if _, err := h.ingSvc.SaveSettings(ctx, publicID, settings); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// checked reads an HTML checkbox, which is absent rather than false when off.
func checked(r *http.Request, name string) bool {
	v := r.PostFormValue(name)
	return v == "1" || v == "on" || v == "true"
}

// VendorIngestBackSubmit reopens the column review.
func (h *UIHandler) VendorIngestBackSubmit(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if h.ingSvc != nil {
		if _, err := h.ingSvc.BackToMapping(r.Context(), publicID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// VendorIngestConfirmSubmit starts the processing run.
func (h *UIHandler) VendorIngestConfirmSubmit(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}
	if _, err := h.ingSvc.ConfirmImport(r.Context(), publicID); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// VendorIngestCancelSubmit discards an import.
func (h *UIHandler) VendorIngestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if h.ingSvc != nil {
		if err := h.ingSvc.CancelImport(r.Context(), publicID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/vendor/ingest", "info", "تم إلغاء عملية الاستيراد.")
}

// VendorIngestProgress reports a running import, for the progress screen's poll.
func (h *UIHandler) VendorIngestProgress(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if h.ingSvc == nil {
		http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	session, err := h.ingSvc.LoadImport(r.Context(), publicID)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	payload := map[string]any{
		"phase":    session.Phase,
		"percent":  session.ProgressPercent,
		"note":     session.ProgressNote,
		"done":     session.Phase.Terminal(),
		"inserted": session.InsertedRows,
		"updated":  session.UpdatedRows,
		"skipped":  session.SkippedRows,
		"errors":   session.ErrorRows,
		"message":  session.ErrorMessage,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(r.Context(), "import progress encode failed", "error", err)
	}
}

// VendorIngestRowsExport downloads the rows a vendor still has to deal with.
//
// Distinct from VendorIngestExport, which dumps their live inventory: this is
// the ledger of one run, filtered to whatever the results screen was showing.
func (h *UIHandler) VendorIngestRowsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: r.URL.Query().Get("match"),
		Limit:      500,
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="import-rows.csv"`)
	// The BOM is what makes Excel open an Arabic CSV as UTF-8 instead of as
	// mojibake, which is the difference between a usable export and a support
	// ticket.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	out := csv.NewWriter(w)
	defer out.Flush()
	_ = out.Write([]string{"رقم الصف", "اسم الصنف في الملف", "الصنف المطابق المعتمد بالكتالوج", "كود الصنف بالملف", "النتيجة", "درجة المطابقة", "الملاحظة"})

	for offset := 0; offset < 20000; offset += filter.Limit {
		filter.Offset = offset
		rows, total, err := h.ingSvc.ImportRows(ctx, publicID, filter)
		if err != nil || len(rows) == 0 {
			return
		}
		for _, row := range rows {
			_ = out.Write([]string{
				strconv.Itoa(row.SourceRow), row.DisplayName, row.MatchedCatalogName(), row.SourceCode,
				pages.OutcomeLabel(row.Outcome), pages.PercentText(row.MatchScore), row.Message,
			})
		}
		out.Flush()
		if offset+len(rows) >= total {
			return
		}
	}
}

// loadImportResults fills the results table for a finished import.
func (h *UIHandler) loadImportResults(r *http.Request, view *pages.VendorImportView) {
	ctx := r.Context()
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: r.URL.Query().Get("match"),
		Search:     r.URL.Query().Get("q"),
		Limit:      50,
	}
	if page, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && page > 1 {
		filter.Offset = (page - 1) * filter.Limit
	}
	view.Filter = filter

	rows, total, err := h.ingSvc.ImportRows(ctx, view.Session.PublicID, filter)
	if err != nil {
		h.log.WarnContext(ctx, "import rows unavailable", "error", err)
		return
	}
	view.Rows, view.RowTotal = rows, total

	counts, err := h.ingSvc.ImportRowCounts(ctx, view.Session.PublicID)
	if err != nil {
		h.log.WarnContext(ctx, "import row counts unavailable", "error", err)
		return
	}
	view.RowCounts = counts
}

// vendorWarehouses lists the vendor's warehouses for the settings stage.
func (h *UIHandler) vendorWarehouses(r *http.Request) []*inventory.Warehouse {
	if h.invSvc == nil {
		return nil
	}
	list, err := h.invSvc.ListWarehouses(r.Context())
	if err != nil {
		h.log.WarnContext(r.Context(), "warehouses unavailable for import", "error", err)
		return nil
	}
	return list
}

// renderImport writes the page.
func (h *UIHandler) renderImport(w http.ResponseWriter, r *http.Request, view pages.VendorImportView) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render vendor import page", "error", err)
	}
}
