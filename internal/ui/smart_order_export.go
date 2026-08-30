package ui

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Exporting a run's results.
//
// The results screen has always offered "تصدير النتائج"; until now the link led
// nowhere, which is the worst version of an export — a button a buyer clicks
// when they need the data most.
//
// It writes every line of the run, not the page currently on screen, and it
// names the matched product rather than its id. An export whose match column
// reads "#255741" cannot be checked by the person who has to sign off on the
// order, which is the only reason to export it.

// utf8BOM makes Excel open a UTF-8 CSV as Arabic rather than as mojibake. Excel
// on Windows still guesses the local codepage without it, and every pharmacy in
// this market opens exports in Excel.
const utf8BOM = "\xEF\xBB\xBF"

// SmartOrderExportCSV streams the whole run as a spreadsheet.
func (h *UIHandler) SmartOrderExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	lines, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{All: true})
	if err != nil {
		h.log.ErrorContext(ctx, "export smart order results", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "smartorder.export_error"), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("smart-order-%s-%s.csv", run.PublicID, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(utf8BOM))

	out := csv.NewWriter(w)
	defer out.Flush()

	if lang == "en" {
		_ = out.Write([]string{
			"Row #", "Raw Name", "SKU", "Barcode",
			"Matched Product ID", "Matched Product Name",
			"Match Method", "Confidence", "Quantity", "Status", "Reason",
		})
	} else {
		_ = out.Write([]string{
			"رقم الصف", "الصنف كما ورد", "الكود", "الباركود",
			"رقم المنتج المطابق", "اسم المنتج المطابق",
			"طريقة المطابقة", "نسبة الثقة", "الكمية", "الحالة", "السبب",
		})
	}

	for _, l := range lines {
		matchedID, matchedName := "", ""
		if l.Matched() {
			matchedID = strconv.FormatInt(*l.MatchedProductID, 10)
			matchedName = l.MatchedProductName
		}
		_ = out.Write([]string{
			strconv.Itoa(l.RowNumber),
			l.RawName,
			l.RawSKU,
			l.RawBarcode,
			matchedID,
			matchedName,
			pages.MatchMethodLabel(l.MatchMethod),
			fmt.Sprintf("%.0f%%", l.MatchConfidence*100),
			strconv.FormatFloat(l.EffectiveQty, 'f', -1, 64),
			pages.SmartOrderOutcomeLabel(l.Outcome),
			l.OutcomeReason,
		})
	}
}
