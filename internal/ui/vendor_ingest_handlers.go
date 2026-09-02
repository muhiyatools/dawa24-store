package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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
		Lang:          langOf(r),
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
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	session, err := h.ingSvc.LoadImport(ctx, publicID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
		return
	}

	view := pages.VendorImportView{
		Lang:          langOf(r),
		Session:       session,
		Warehouses:    h.vendorWarehouses(r),
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	view.AIAvailable, view.AIUnavailableReason = h.vendorImportAIState(ctx, langOf(r))

	switch {
	case session.Phase.Terminal():
		h.loadImportResults(r, &view)
	case session.Phase == ingest.PhaseReview:
		h.loadImportReview(r, &view)
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
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			i18n.T(langOf(r), "vendor.ingest.file_too_large"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			i18n.T(langOf(r), "vendor.ingest.invalid_file_format"))
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "vendor.ingest.read_file_error"))
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
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	overrides, err := vendorMappingOverrides(r)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", err.Error())
		return
	}

	if _, _, err := h.ingSvc.SaveMapping(ctx, publicID, overrides); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// vendorMappingOverrides reads the field-first mapping table.
//
// The table posts one entry per system field — `field_price=3` meaning "column
// four of my file is the price" — which is the inverse of what the engine
// stores, so it is turned back column-first here. Two fields pointing at the
// same column is the one error a vendor can make on this screen that the
// resolver cannot silently repair, so it is refused with both field names
// rather than resolved by whichever came last in a map iteration.
//
// Every column no field claimed is pinned to "" rather than left out. Left out
// means "decide for me", and the completion pass would then re-bind a column
// the vendor had just cleared — which is exactly how a cleared mapping came
// back on the next screen.
func vendorMappingOverrides(r *http.Request) (map[int]productmatch.Field, error) {
	overrides := map[int]productmatch.Field{}
	claimedBy := map[int]productmatch.Field{}
	sawFieldForm := false

	for key, values := range r.PostForm {
		if !strings.HasPrefix(key, "field_") || len(values) == 0 {
			continue
		}
		sawFieldForm = true
		field := productmatch.Field(strings.TrimPrefix(key, "field_"))
		if _, known := productmatch.SpecOf(field); !known {
			continue
		}
		if !productmatch.VendorFields.Allows(field) {
			continue
		}
		raw := strings.TrimSpace(values[0])
		if raw == "" {
			continue
		}
		index, convErr := strconv.Atoi(raw)
		if convErr != nil || index < 0 {
			continue
		}
		if other, taken := claimedBy[index]; taken {
			return nil, fmt.Errorf("العمود رقم %d مربوط بحقلين: «%s» و«%s». اختر حقلاً واحداً لكل عمود.",
				index+1, other.Label(), field.Label())
		}
		claimedBy[index] = field
		overrides[index] = field
	}

	if !sawFieldForm {
		// A client posting the legacy column-first form. Kept so a stale open
		// tab does not lose a vendor's work.
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
			case "__ignore":
				overrides[index] = productmatch.IgnoreField
			case "__none":
				overrides[index] = ""
			default:
				overrides[index] = productmatch.Field(values[0])
			}
		}
		return overrides, nil
	}

	// Pin every unclaimed column shut.
	for i := 0; i < vendorMaxMappedColumns; i++ {
		if _, claimed := claimedBy[i]; !claimed {
			overrides[i] = ""
		}
	}
	return overrides, nil
}

// vendorMaxMappedColumns is the widest sheet the mapping screen will pin shut.
// Wider files are still imported; their trailing columns simply keep the
// resolver's own reading, which for a column past this point is "unmapped"
// anyway.
const vendorMaxMappedColumns = 256

// VendorIngestSettingsSubmit records the import rules.
func (h *UIHandler) VendorIngestSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	actor, _ := authctx.From(ctx)

	settings := ingest.DefaultSettings()
	settings.WarehouseID, _ = strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64)

	// Auto-resolve branch and warehouse linkage
	if settings.WarehouseID > 0 && h.invSvc != nil {
		if wh, err := h.invSvc.GetWarehouse(ctx, settings.WarehouseID); err == nil && wh != nil && wh.BranchID != nil && *wh.BranchID > 0 {
			settings.BranchID = wh.BranchID
		}
	}
	if settings.BranchID == nil && actor.OrganizationID > 0 && h.orgSvc != nil {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil && len(branches) > 0 {
			for _, b := range branches {
				if b.IsMain {
					settings.BranchID = &b.ID
					break
				}
			}
			if settings.BranchID == nil {
				settings.BranchID = &branches[0].ID
			}
		}
	}
	if settings.WarehouseID <= 0 && actor.OrganizationID > 0 && h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, w := range whs {
			if w.OrganizationID == actor.OrganizationID && w.IsActive {
				settings.WarehouseID = w.ID
				if settings.BranchID == nil && w.BranchID != nil && *w.BranchID > 0 {
					settings.BranchID = w.BranchID
				}
				break
			}
		}
		if settings.WarehouseID <= 0 {
			whName := "مستودع التوريد الرئيسي"
			if settings.BranchID != nil && h.orgSvc != nil {
				if b, err := h.orgSvc.GetBranch(ctx, *settings.BranchID); err == nil && b != nil {
					whName = "مستودع " + b.Name.Get(i18n.AR)
				}
			}
			newWh := &inventory.Warehouse{
				OrganizationID: actor.OrganizationID,
				BranchID:       settings.BranchID,
				Name:           whName,
				Code:           fmt.Sprintf("WH-%d", actor.OrganizationID),
				IsActive:       true,
			}
			if created, err := h.invSvc.CreateWarehouse(ctx, newWh); err == nil && created != nil {
				settings.WarehouseID = created.ID
			}
		}
	}
	settings.Mode = ingest.ParseMode(r.PostFormValue("mode"))
	settings.StockMode = inventory.StockMode(r.PostFormValue("stock_mode"))
	settings.Duplicates = productmatch.DuplicatePolicy(r.PostFormValue("duplicates"))
	if score, err := strconv.ParseFloat(r.PostFormValue("min_match_score"), 64); err == nil {
		settings.MinMatchScore = score / 100
	}
	settings.TrustSupplierCode = checked(r, "trust_supplier_code")
	settings.CodeIsCatalogCode = checked(r, "code_is_catalog_code")
	settings.TrustBarcode = checked(r, "trust_barcode")
	// Physical stock rule: missing quantity is zero by default (no fake stock)
	settings.BlankQuantityIsZero = true
	if val := r.PostFormValue("blank_quantity_is_zero"); val == "false" || val == "0" {
		settings.BlankQuantityIsZero = false
	}
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
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
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
	h.redirectWithNotice(w, r, "/vendor/ingest", "info", i18n.T(langOf(r), "vendor.ingest.cancelled_notice"))
}
