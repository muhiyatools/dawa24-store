package catalog

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ImportStats captures metrics from the bulk import process.
type ImportStats struct {
	TotalRowsRead  int
	ValidProducts  int
	RepeatedHeader int
	EmptyRows      int
	Inserted       int
	Updated        int
}

var (
	dosageFormKeywords = []struct {
		keywords []string
		form     string
	}{
		{[]string{"غسول فم", "مضمضة", "مضمضه", "mouthwash"}, "غسول فم"},
		{[]string{"غسول", "wash", "lotion", "لوشن"}, "غسول"},
		{[]string{"كريم", "cream"}, "كريم"},
		{[]string{"مرهم", "ointment", "oint"}, "مرهم"},
		{[]string{"جل", "جيل", "gel"}, "جل"},
		{[]string{"زيت", "oil", "اويل"}, "زيت"},
		{[]string{"سيروم", "serum"}, "سيروم"},
		{[]string{"شامبو", "shampoo"}, "شامبو"},
		{[]string{"صابون", "صابونة", "صابونه", "soap"}, "صابون"},
		{[]string{"رول اون", "roll on", "رول-اون", "رول_اون"}, "رول اون"},
		{[]string{"اسبراي", "سبراي", "اسبراى", "سبراى", "بخاخ", "spray", "بدى ميست", "body mist"}, "بخاخ / اسبراي"},
		{[]string{"مناديل", "wipes"}, "مناديل مبللة"},
		{[]string{"صبغة", "صبغه", "hair color", "color", "colour"}, "صبغة شعر"},
		{[]string{"اقراص", "أقراص", "قرص", "tab", "tabs", "tablet", "tablets"}, "أقراص"},
		{[]string{"كبسول", "كبسولات", "cap", "caps", "capsule", "capsules"}, "كبسولات"},
		{[]string{"شراب", "syrup", "susp", "معلق"}, "شراب"},
		{[]string{"نقط", "drops", "قطرة", "قطره"}, "نقط"},
		{[]string{"حقن", "حقنة", "حقنه", "امبول", "أمبول", "امبولات", "أمبولات", "فيال", "vial", "ampoule", "inj"}, "حقن وأمبولات"},
		{[]string{"فوار", "ساشيت", "sachet", "eff"}, "أكياس فوار"},
		{[]string{"لبوس", "تحاميل", "تحميلة", "supp", "suppository"}, "لبوس"},
		{[]string{"معجون اسنان", "معجون أسنان", "toothpaste"}, "معجون أسنان"},
		{[]string{"حفاضات", "حفاضه", "diapers"}, "مستلزمات عناية"},
		{[]string{"استيك", "ستيك", "stick"}, "ستيك مضاد للتعرق"},
	}

	concentrationPattern = regexp.MustCompile("(?i)(\\d+(?:\\.\\d+)?\\s*(?:ملجرام|مليجرام|مجم|جم|جرام|مل|ملي|ملم|mg|g|gm|ml|mcg|iu|%|spf[+\\d]*|[+\\d]*spf))")
)

func CleanCellString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// ExtractDosageAndConcentration parses dosage form and concentration from product name.
func ExtractDosageAndConcentration(name string) (dosage string, conc string) {
	lowered := strings.ToLower(name)
	for _, dk := range dosageFormKeywords {
		for _, kw := range dk.keywords {
			if strings.Contains(lowered, kw) {
				dosage = dk.form
				break
			}
		}
		if dosage != "" {
			break
		}
	}
	if dosage == "" {
		dosage = "مستحضر صيدلاني"
	}

	match := concentrationPattern.FindString(name)
	if match != "" {
		conc = strings.TrimSpace(match)
	}
	return dosage, conc
}

// DetectHeaderRow scans the first 30 rows and returns the index of the best header candidate.
func DetectHeaderRow(records [][]string) int {
	if len(records) == 0 {
		return -1
	}

	bestIdx := -1
	bestScore := 0

	maxScan := min(30, len(records))
	for i := 0; i < maxScan; i++ {
		row := records[i]
		if len(row) == 0 {
			continue
		}

		score := 0
		for _, cell := range row {
			c := strings.ToLower(CleanCellString(cell))
			if c == "" {
				continue
			}

			if strings.Contains(c, "item") || strings.Contains(c, "desc") || strings.Contains(c, "vendor") ||
				strings.Contains(c, "name") || strings.Contains(c, "sku") || strings.Contains(c, "code") ||
				strings.Contains(c, "price") || strings.Contains(c, "generic") || strings.Contains(c, "dosage") ||
				strings.Contains(c, "barcode") || strings.Contains(c, "company") || strings.Contains(c, "manufacturer") ||
				strings.Contains(c, "اسم") || strings.Contains(c, "صنف") || strings.Contains(c, "كود") ||
				strings.Contains(c, "سعر") || strings.Contains(c, "شركة") || strings.Contains(c, "مورد") ||
				strings.Contains(c, "باركود") || strings.Contains(c, "شكل") || strings.Contains(c, "علمي") ||
				strings.Contains(c, "فعالة") || strings.Contains(c, "تسجيل") || strings.Contains(c, "eda") {
				score += 2
			}
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return bestIdx
}

// MapHeaderColumns creates a map of normalized field names to column indices.
func MapHeaderColumns(headerRow []string) map[string]int {
	colMap := make(map[string]int)
	if len(headerRow) == 0 {
		return colMap
	}

	for idx, cell := range headerRow {
		clean := strings.ToLower(CleanCellString(cell))
		clean = strings.ReplaceAll(clean, ".", "")
		clean = strings.ReplaceAll(clean, "_", "")
		clean = strings.ReplaceAll(clean, "-", "")
		clean = strings.ReplaceAll(clean, " ", "")

		// 1. SKU / Item Code / Registration
		if strings.Contains(clean, "itemno") || strings.Contains(clean, "itemcode") ||
			strings.Contains(clean, "sku") || strings.Contains(clean, "edaregnumber") ||
			strings.Contains(clean, "eda") || strings.Contains(clean, "كودالصنف") ||
			strings.Contains(clean, "كود") || strings.Contains(clean, "رقمصنف") ||
			strings.Contains(clean, "رقمالتسجيل") {
			if _, exists := colMap["sku"]; !exists {
				colMap["sku"] = idx
			}
		}

		// 2. Barcode
		if strings.Contains(clean, "barcode") || strings.Contains(clean, "باركود") || strings.Contains(clean, "ean") {
			if _, exists := colMap["barcode"]; !exists {
				colMap["barcode"] = idx
			}
		}

		// 3. Product Name (Arabic)
		if strings.Contains(clean, "itemdesc") || strings.Contains(clean, "itemdescription") ||
			strings.Contains(clean, "namear") || strings.Contains(clean, "arabicname") ||
			strings.Contains(clean, "اسمبالعربي") || strings.Contains(clean, "اسمالصنفبالعربي") ||
			strings.Contains(clean, "اسمالصنف") || strings.Contains(clean, "اسمالدواء") ||
			strings.Contains(clean, "المستحضر") || strings.Contains(clean, "وصفالصنف") ||
			strings.Contains(clean, "productname") || (strings.Contains(clean, "name") && !strings.Contains(clean, "en") && !strings.Contains(clean, "generic") && !strings.Contains(clean, "scientific")) {
			if _, exists := colMap["name_ar"]; !exists {
				colMap["name_ar"] = idx
			}
		}

		// 4. Product Name (English)
		if strings.Contains(clean, "nameen") || strings.Contains(clean, "englishname") ||
			strings.Contains(clean, "اسمبالانجليزي") || strings.Contains(clean, "اسمبالإنجليزية") ||
			strings.Contains(clean, "tradename") || strings.Contains(clean, "english") {
			if _, exists := colMap["name_en"]; !exists {
				colMap["name_en"] = idx
			}
		}

		// 5. Manufacturer / Vendor / Brand
		if strings.Contains(clean, "preferredvendor") || strings.Contains(clean, "vendor") ||
			strings.Contains(clean, "manufacturer") || strings.Contains(clean, "company") ||
			strings.Contains(clean, "brand") || strings.Contains(clean, "الشركةالمصنعة") ||
			strings.Contains(clean, "الشركة") || strings.Contains(clean, "المصنع") ||
			strings.Contains(clean, "الموردالمفضل") || strings.Contains(clean, "المورد") {
			if _, exists := colMap["manufacturer"]; !exists {
				colMap["manufacturer"] = idx
			}
		}

		// 6. Price
		if strings.Contains(clean, "price") || strings.Contains(clean, "publicprice") ||
			strings.Contains(clean, "سعرالجمهور") || strings.Contains(clean, "السعر") ||
			strings.Contains(clean, "سعرالبيع") || strings.Contains(clean, "سعر") {
			if _, exists := colMap["price"]; !exists {
				colMap["price"] = idx
			}
		}

		// 7. Generic / Scientific Name
		if strings.Contains(clean, "genericname") || strings.Contains(clean, "scientificname") ||
			strings.Contains(clean, "generic") || strings.Contains(clean, "scientific") ||
			strings.Contains(clean, "الاسمالعلمي") || strings.Contains(clean, "علمي") {
			if _, exists := colMap["generic_name"]; !exists {
				colMap["generic_name"] = idx
			}
		}

		// 8. Active Ingredient
		if strings.Contains(clean, "activeingredient") || strings.Contains(clean, "active") ||
			strings.Contains(clean, "المادةالفعالة") || strings.Contains(clean, "المادةالفعاله") ||
			strings.Contains(clean, "مادةفعالة") {
			if _, exists := colMap["active_ingredient"]; !exists {
				colMap["active_ingredient"] = idx
			}
		}

		// 9. Dosage Form
		if strings.Contains(clean, "dosageform") || strings.Contains(clean, "dosage") ||
			strings.Contains(clean, "form") || strings.Contains(clean, "الشكلالصيدلي") ||
			strings.Contains(clean, "الشكل") || strings.Contains(clean, "هيئةالدواء") {
			if _, exists := colMap["dosage_form"]; !exists {
				colMap["dosage_form"] = idx
			}
		}

		// 10. Description
		if strings.Contains(clean, "descriptionar") || strings.Contains(clean, "description") ||
			strings.Contains(clean, "الوصفبالعربي") || strings.Contains(clean, "الوصف") ||
			strings.Contains(clean, "دواعيالاستعمال") {
			if _, exists := colMap["description_ar"]; !exists {
				colMap["description_ar"] = idx
			}
		}
	}

	// Smart fallback: if name_ar is not mapped, check position 1 or 0
	if _, ok := colMap["name_ar"]; !ok {
		if len(headerRow) > 1 {
			colMap["name_ar"] = 1
		} else if len(headerRow) > 0 {
			colMap["name_ar"] = 0
		}
	}

	return colMap
}

// ParseProductRows converts raw spreadsheet rows into cleaned, valid domain Products.
func ParseProductRows(records [][]string) ([]*Product, ImportStats) {
	var stats ImportStats
	stats.TotalRowsRead = len(records)

	if len(records) == 0 {
		return nil, stats
	}

	headerIdx := DetectHeaderRow(records)
	var colMap map[string]int
	if headerIdx >= 0 {
		colMap = MapHeaderColumns(records[headerIdx])
	} else {
		// Fallback header map for headerless files
		colMap = map[string]int{
			"sku":          0,
			"name_ar":      1,
			"manufacturer": 2,
		}
	}

	startRow := 0
	if headerIdx >= 0 {
		startRow = headerIdx + 1
	}

	var products []*Product

	for rIdx := startRow; rIdx < len(records); rIdx++ {
		row := records[rIdx]
		if len(row) == 0 {
			stats.EmptyRows++
			continue
		}

		// 1. Check if row is a repeated header from pagination
		isRepeatedHeader := false
		for _, cell := range row {
			c := strings.ToLower(CleanCellString(cell))
			if c == "item no." || c == "item description" || c == "preferred vendor" ||
				c == "item no" || c == "item desc" || c == "اسم الصنف" || c == "كود الصنف" ||
				c == "اسم الصنف بالعربي" || c == "السعر" || c == "الشركة المصنعة" {
				isRepeatedHeader = true
				break
			}
		}
		if isRepeatedHeader {
			stats.RepeatedHeader++
			continue
		}

		// 2. Extract Fields safely
		getVal := func(key string) string {
			if idx, ok := colMap[key]; ok && idx < len(row) {
				return CleanCellString(row[idx])
			}
			return ""
		}

		sku := getVal("sku")
		barcode := getVal("barcode")
		if barcode == "" {
			barcode = sku
		}

		nameAr := getVal("name_ar")
		nameEn := getVal("name_en")
		mfr := getVal("manufacturer")
		generic := getVal("generic_name")
		active := getVal("active_ingredient")
		dosage := getVal("dosage_form")
		descAr := getVal("description_ar")
		priceStr := getVal("price")

		// If name is missing in mapped column, search other columns for descriptive text
		if nameAr == "" && nameEn == "" {
			for _, cell := range row {
				c := CleanCellString(cell)
				if len(c) > 3 && !regexp.MustCompile("^\\d+$").MatchString(c) {
					nameAr = c
					break
				}
			}
		}

		// Skip completely blank rows
		if nameAr == "" && sku == "" {
			stats.EmptyRows++
			continue
		}

		// Fallback name if only SKU exists
		if nameAr == "" && sku != "" {
			nameAr = "صنف دوائي #" + sku
		}
		if nameEn == "" {
			nameEn = nameAr
		}
		if nameAr == "" {
			nameAr = nameEn
		}

		// 3. Extract Dosage Form & Concentration if missing
		autoDosage, autoConc := ExtractDosageAndConcentration(nameAr)
		if dosage == "" {
			dosage = autoDosage
		}

		// 4. Parse Price
		var priceVal money.Amount
		if priceStr != "" {
			priceVal, _ = money.Parse(priceStr)
		}

		prod := &Product{
			Name:                   i18n.New(nameAr, nameEn),
			Description:            i18n.New(descAr, ""),
			SKU:                    sku,
			Barcode:                barcode,
			Price:                  priceVal,
			Status:                 StatusActive,
			DosageForm:             dosage,
			Concentration:          autoConc,
			ScientificName:         generic,
			Active:                 active,
			ManufacturingCompanies: mfr,
		}

		products = append(products, prod)
	}

	stats.ValidProducts = len(products)
	return products, stats
}

// ParseUploadedSpreadsheet reads Excel (.xlsx, .xls) and CSV files into a 2D string slice.
func ParseUploadedSpreadsheet(content []byte, filename string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	// Excel workbook (.xlsx / .xlsm / PK header)
	if ext == ".xlsx" || ext == ".xlsm" || bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("تعذر قراءة ملف Excel: %w", err)
		}
		defer f.Close()

		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, errors.New("ملف Excel لا يحتوي على أي صفحات بيانات")
		}

		// Choose sheet with the most rows
		var bestRows [][]string
		for _, sheet := range sheets {
			rows, err := f.GetRows(sheet)
			if err == nil && len(rows) > len(bestRows) {
				bestRows = rows
			}
		}

		if len(bestRows) == 0 {
			return nil, errors.New("صفحات ملف Excel المرفوع فارغة تماماً")
		}
		return bestRows, nil
	}

	// CSV Parsing with UTF-8 BOM removal and auto delimiter detection
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	firstLine := ""
	if idx := bytes.IndexByte(content, '\n'); idx >= 0 {
		firstLine = string(content[:idx])
	} else {
		firstLine = string(content)
	}

	var delimiter rune = ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		delimiter = ';'
	} else if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		delimiter = '\t'
	} else if strings.Count(firstLine, "|") > strings.Count(firstLine, ",") {
		delimiter = '|'
	}

	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("تعذر قراءة ملف CSV: %w", err)
	}
	return records, nil
}
