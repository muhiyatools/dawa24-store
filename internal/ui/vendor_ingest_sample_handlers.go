package ui

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Vendor catalog import handlers are implemented in vendor_ingest_handlers.go (Plan V5 Phase 4).

// VendorIngestSampleXLSX streams a styled Excel template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	headers := []string{
		"الباركود",
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"سعر التوريد",
		"الرصيد",
		"رقم التشغيلة",
		"تاريخ الصلاحية",
	}

	for i, head := range headers {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s1", colName), head)
	}

	sampleRows := [][]string{
		{"6221142001234", "بانادول إكسترا 24 قرص", "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", "أقراص", "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", "أوجمنتين 1 جم 14 قرص", "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", "أقراص", "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", "كتفاست 50 مجم فوار", "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", "كونجستال 20 قرص", "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", "أنتينال 24 كبسولة", "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for rIdx, row := range sampleRows {
		for cIdx, val := range row {
			colName, _ := excelize.ColumnNumberToName(cIdx + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rIdx+2), val)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.xlsx\"")
	_ = f.Write(w)
}

// VendorIngestSampleCSV streams a UTF-8 BOM CSV template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{
		"الباركود",
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"سعر التوريد",
		"الرصيد",
		"رقم التشغيلة",
		"تاريخ الصلاحية",
	}
	_ = writer.Write(headers)

	sampleRows := [][]string{
		{"6221142001234", "بانادول إكسترا 24 قرص", "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", "أقراص", "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", "أوجمنتين 1 جم 14 قرص", "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", "أقراص", "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", "كتفاست 50 مجم فوار", "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", "كونجستال 20 قرص", "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", "أنتينال 24 كبسولة", "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for _, row := range sampleRows {
		_ = writer.Write(row)
	}
}

// VendorIngestExport exports the vendor's real active inventory and pricing as a CSV.
func (h *UIHandler) VendorIngestExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_inventory.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{"الباركود / SKU", "اسم الصنف الدوائي", "سعر التوريد (ج.م)", "الرصيد المتاح (عبوة)", "رقم التشغيلة", "تاريخ الصلاحية", "الحالة"}
	_ = writer.Write(headers)

	if h.catSvc != nil {
		products, _ := h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{Limit: 500})
		for _, p := range products {
			if p == nil {
				continue
			}
			variants, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
			for _, v := range variants {
				if v != nil && v.OrganizationID == actor.OrganizationID {
					expStr := ""
					if v.ExpiryDate != nil {
						expStr = v.ExpiryDate.Format("2006-01-02")
					}
					status := "متاح للطلب"
					if v.Status != catalog.StatusActive && v.Status != "" {
						status = "غير متاح"
					}
					_ = writer.Write([]string{
						v.SKU,
						p.Name.Get(i18n.AR),
						v.Price.String(),
						fmt.Sprintf("%d", v.StockQty),
						v.BatchNumber,
						expStr,
						status,
					})
				}
			}
		}
	}
}
