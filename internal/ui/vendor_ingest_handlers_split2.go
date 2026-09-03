package ui

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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
	// The bar must not read 100 while the run is still going. A staging pass
	// that reports 99 and then writes for another twenty seconds is a bar the
	// vendor watches sitting at "finished" while nothing has finished.
	percent := session.ProgressPercent
	running := session.Phase == ingest.PhaseProcessing
	if running && percent >= importprogress.Complete {
		percent = importprogress.Complete - 1
	}
	if !running {
		percent = importprogress.Complete
	}

	payload := map[string]any{
		"phase":   session.Phase,
		"percent": percent,
		"note":    session.ProgressNote,
		// "message" as well as "note": the shared progress bar reads one field
		// name across all four import tools, and this endpoint had its own.
		"message": session.ProgressNote,
		// "done" means the run has stopped, not that the import is finished.
		//
		// It used to be Phase.Terminal(), which is completed/failed/cancelled —
		// correct for the commit path, and wrong for staging, which ends in
		// 'review'. The browser polled a bar sitting at 100% for ever and the
		// vendor never reached the review screen the run had already built.
		"done":     session.Phase != ingest.PhaseProcessing,
		"inserted": session.InsertedRows,
		"updated":  session.UpdatedRows,
		"skipped":  session.SkippedRows,
		"errors":   session.ErrorRows,
		"error":    session.ErrorMessage,
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
	lang := langOf(r)
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
	_ = out.Write([]string{
		i18n.T(lang, "ingest.csv.row_number"),
		i18n.T(lang, "ingest.csv.item_name"),
		i18n.T(lang, "ingest.csv.matched_catalog_item"),
		i18n.T(lang, "ingest.csv.item_code"),
		i18n.T(lang, "ingest.csv.outcome"),
		i18n.T(lang, "ingest.csv.match_score"),
		i18n.T(lang, "ingest.csv.notes"),
	})

	for offset := 0; offset < 20000; offset += filter.Limit {
		filter.Offset = offset
		rows, total, err := h.ingSvc.ImportRows(ctx, publicID, filter)
		if err != nil || len(rows) == 0 {
			return
		}
		for _, row := range rows {
			_ = out.Write([]string{
				strconv.Itoa(row.SourceRow), row.DisplayName, row.MatchedCatalogName(), row.SourceCode,
				pages.OutcomeLabel(row.Outcome, lang), pages.PercentText(row.MatchScore), row.Message,
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
	actor, ok := authctx.From(r.Context())
	if !ok || actor.OrganizationID <= 0 {
		return nil
	}
	list, err := h.invSvc.ListWarehouses(r.Context())
	if err != nil {
		h.log.WarnContext(r.Context(), "warehouses unavailable for import", "error", err)
		return nil
	}
	var filtered []*inventory.Warehouse
	for _, wh := range list {
		if wh.OrganizationID == actor.OrganizationID {
			filtered = append(filtered, wh)
		}
	}
	return filtered
}

// renderImport writes the page.
func (h *UIHandler) renderImport(w http.ResponseWriter, r *http.Request, view pages.VendorImportView) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render vendor import page", "error", err)
	}
}
