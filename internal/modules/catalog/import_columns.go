package catalog

import (
	"sort"
	"strings"
)

// Column identification for spreadsheet import.
//
// The previous mapper walked the header left to right and gave a column to the
// first field whose keyword it contained, first match winning. That is why a
// column headed "الباركود الدولي" was bound to sku: the sku rule tested for
// "كود", which is a substring of "باركود", and it ran before the barcode rule.
// The real barcode column was then unavailable and the real item-code column,
// further right, was never looked at.
//
// The fix is to stop deciding per column and start deciding globally. Every
// (field, column) pair is scored, and the assignment is made best-score-first
// with each column claimed once, so a strong signal anywhere in the header
// outranks a weak coincidence earlier in it.

// Import field names. These are the keys of the binding map that row parsing
// reads, kept as constants so a typo cannot silently produce an unmapped field.
const (
	FieldNameAR        = "name_ar"
	FieldNameEN        = "name_en"
	FieldSKU           = "sku"
	FieldBarcode       = "barcode"
	FieldPrice         = "price"
	FieldPublicPrice   = "public_price"
	FieldCostPrice     = "cost_price"
	FieldDiscount      = "discount"
	FieldGenericName   = "generic_name"
	FieldActive        = "active_ingredient"
	FieldDosageForm    = "dosage_form"
	FieldConcentration = "concentration"
	FieldUnit          = "unit"
	FieldManufacturer  = "manufacturer"
	FieldDescriptionAR = "description_ar"
	FieldDescriptionEN = "description_en"
	FieldQuantity      = "quantity"
	FieldStatus        = "status"
	FieldCategory      = "category"
)

// FieldLabels are the Arabic names shown in the import report, so an admin
// reading "لم يتم العثور على عمود: السعر" knows which column to add.
var FieldLabels = map[string]string{
	FieldNameAR:        "اسم الصنف بالعربي",
	FieldNameEN:        "اسم الصنف بالإنجليزي",
	FieldSKU:           "كود الصنف",
	FieldBarcode:       "الباركود",
	FieldPrice:         "السعر",
	FieldPublicPrice:   "سعر الجمهور",
	FieldCostPrice:     "سعر التكلفة",
	FieldDiscount:      "الخصم",
	FieldGenericName:   "الاسم العلمي",
	FieldActive:        "المادة الفعالة",
	FieldDosageForm:    "الشكل الصيدلي",
	FieldConcentration: "التركيز",
	FieldUnit:          "الوحدة",
	FieldManufacturer:  "الشركة المصنعة",
	FieldDescriptionAR: "الوصف بالعربي",
	FieldDescriptionEN: "الوصف بالإنجليزي",
	FieldQuantity:      "الكمية",
	FieldStatus:        "الحالة",
	FieldCategory:      "فئة المنتج",
}

// Match strengths. The gaps are wide on purpose: an exact header match must beat
// any number of partial ones, and a "strong" phrase (a full multi-word term the
// field owns) must beat a "weak" one (a single word several fields share).
const (
	scoreExact  = 100
	scoreStrong = 60
	scoreWeak   = 25
	scoreMin    = scoreWeak
)

// fieldSpec describes how to recognise one product field in a header cell.
//
// exact matches the whole normalised header. strong and weak are substring
// tests. blocked disqualifies the pair outright, which is how "سعر التكلفة" is
// kept away from the selling-price field and "الاسم العلمي" away from the trade
// name — those are the collisions that silently corrupted imported catalogues.
type fieldSpec struct {
	field   string
	exact   []string
	strong  []string
	weak    []string
	blocked []string
}

// fieldSpecs is ordered only for readability; the assignment below is order
// independent.
//
// The entries are written the natural way — "الشكل الدوائي", "Item No." — and
// folded through NormalizeKey at init, exactly as the header cells they are
// compared against are. Hand-folding them in the source was tried and got the
// hamza forms wrong, which cost "الشكل الدوائي" its exact match and dropped it
// to a weak partial score.
var fieldSpecs = []fieldSpec{
	{
		field: FieldSKU,
		exact: []string{
			"itemno", "itemcode", "sku", "code", "productcode", "itemid", "id",
			"كودالصنف", "كودالمنتج", "الكود", "كود", "رقمالصنف", "رقمالمنتج",
			"رقمالتسجيل", "edaregnumber", "edano", "eda",
		},
		strong: []string{
			"itemcode", "itemno", "productcode", "materialcode", "articlecode",
			"كودالصنف", "كودالمنتج", "رقمالصنف", "رقمالتسجيل", "رقمالتسجيلeda",
		},
		weak:    []string{"sku", "code", "كود", "eda"},
		blocked: []string{"barcode", "باركود", "ean", "upc", "كودالشركه", "كودالمورد", "كودالتصنيف"},
	},
	{
		field:  FieldBarcode,
		exact:  []string{"barcode", "ean", "ean13", "upc", "gtin", "باركود", "الباركود"},
		strong: []string{"barcode", "باركود", "الباركودالدولي", "ean13", "gtin"},
		weak:   []string{"ean", "upc"},
	},
	{
		field: FieldNameAR,
		exact: []string{
			"itemdescription", "itemdesc", "description", "productname", "name",
			"itemname", "product", "اسمالصنف", "اسمالمنتج", "اسمالدواء", "الصنف",
			"المنتج", "اسمالصنفبالعربي", "الاسم", "اسمالصنفالتجاري", "المستحضر",
			"اسمالمستحضر", "الاسمالتجاري",
		},
		strong: []string{
			"اسمالصنفبالعربي", "اسمالمنتجبالعربي", "الاسمبالعربي", "اسمعربي",
			"namear", "arabicname", "itemdescription", "اسمالصنف", "اسمالمنتج",
			"اسمالدواء", "اسمالمستحضر", "الاسمالتجاري", "اسمالصنفالتجاري",
			"وصفالصنف", "بيانالصنف",
		},
		weak: []string{"اسم", "الصنف", "المنتج", "itemdesc", "productname", "name", "description"},
		blocked: []string{
			"english", "انجليز", "nameen", "generic", "scientific", "علمي",
			"الشركه", "المصنع", "المورد", "vendor", "brand", "manufacturer",
			"category", "التصنيف", "المجموعه", "unit", "الوحده", "الحاله",
		},
	},
	{
		field: FieldNameEN,
		exact: []string{
			"nameen", "englishname", "tradename", "english", "اسمبالانجليزي",
			"اسمالصنفبالانجليزي", "الاسمبالانجليزيه", "الاسمالانجليزي",
		},
		strong: []string{
			"nameen", "englishname", "englishdescription", "tradename",
			"اسمبالانجليز", "الاسمبالانجليز", "اسمانجليزي", "اسملاتيني",
		},
		weak:    []string{"english", "انجليز", "latin", "لاتيني"},
		blocked: []string{"generic", "scientific", "علمي", "description", "الوصف"},
	},
	{
		field: FieldManufacturer,
		exact: []string{
			"manufacturer", "company", "brand", "vendor", "supplier", "maker",
			"preferredvendor", "الشركه", "الشركهالمصنعه", "المصنع", "المورد",
			"الماركه", "العلامهالتجاريه", "جههالتصنيع", "المنتج", "شركه",
		},
		strong: []string{
			"manufacturer", "preferredvendor", "الشركهالمصنعه", "جههالتصنيع",
			"الموردالمفضل", "اسمالشركه", "اسمالمورد", "شركهالتصنيع", "المصنعه",
		},
		weak:    []string{"company", "brand", "vendor", "supplier", "الشركه", "المصنع", "المورد", "الماركه"},
		blocked: []string{"كودالشركه", "companycode", "vendorcode", "supplierid", "الشركهرقم"},
	},
	{
		field: FieldPrice,
		exact: []string{
			"price", "unitprice", "sellingprice", "salesprice", "السعر", "سعر",
			"سعرالبيع", "سعرالوحده", "سعرالصنف",
		},
		strong: []string{
			"sellingprice", "salesprice", "unitprice", "retailprice", "listprice",
			"سعرالبيع", "سعرالوحده", "سعرالصنف", "سعرالبيعللجمهور",
		},
		weak: []string{"price", "سعر", "ثمن"},
		blocked: []string{
			"cost", "purchase", "buying", "التكلفه", "الشراء", "شراء",
			"old", "قديم", "before", "discount", "خصم", "net", "صافي",
		},
	},
	{
		field: FieldPublicPrice,
		exact: []string{
			"publicprice", "listprice", "mrp", "سعرالجمهور", "سعرالبيعللجمهور",
			"السعرالعام", "سعرقبلالخصم",
		},
		strong: []string{"publicprice", "سعرالجمهور", "سعرالبيعللجمهور", "سعرقبلالخصم", "oldprice", "السعرالقديم"},
		weak:   []string{"mrp", "listprice", "السعرالعام"},
	},
	{
		field:  FieldCostPrice,
		exact:  []string{"costprice", "cost", "purchaseprice", "سعرالتكلفه", "التكلفه", "سعرالشراء"},
		strong: []string{"costprice", "purchaseprice", "buyingprice", "سعرالتكلفه", "سعرالشراء", "تكلفهالوحده"},
		weak:   []string{"cost", "التكلفه", "الشراء"},
	},
	{
		field:  FieldDiscount,
		exact:  []string{"discount", "disc", "الخصم", "نسبهالخصم", "خصم"},
		strong: []string{"discountpercent", "discountrate", "نسبهالخصم", "قيمهالخصم", "الخصمالممنوح"},
		weak:   []string{"discount", "خصم", "disc"},
	},
	{
		field: FieldGenericName,
		exact: []string{
			"genericname", "generic", "scientificname", "scientific", "inn",
			"الاسمالعلمي", "الاسمالكيميائي", "علمي",
		},
		strong: []string{"genericname", "scientificname", "الاسمالعلمي", "الاسمالكيميائي", "الاسمالعام"},
		weak:   []string{"generic", "scientific", "علمي", "inn"},
	},
	{
		field: FieldActive,
		exact: []string{
			"activeingredient", "activesubstance", "active", "ingredient",
			"المادهالفعاله", "المادهالنشطه", "مادهفعاله", "الفعاله",
		},
		strong: []string{"activeingredient", "activesubstance", "المادهالفعاله", "المادهالنشطه", "المكونالفعال"},
		weak:   []string{"active", "ingredient", "فعاله", "الماده"},
	},
	{
		field: FieldDosageForm,
		exact: []string{
			"dosageform", "form", "pharmaceuticalform", "الشكلالصيدلي",
			"الشكلالدوائي", "الشكل", "هيئهالدواء", "شكلالدواء",
		},
		strong: []string{"dosageform", "pharmaceuticalform", "الشكلالصيدلي", "الشكلالدوائي", "هيئهالدواء"},
		weak:   []string{"dosage", "form", "الشكل"},
		// "format" and "platform" would otherwise match "form".
		blocked: []string{"format", "platform", "performance"},
	},
	{
		field:  FieldConcentration,
		exact:  []string{"concentration", "strength", "dose", "التركيز", "الجرعه", "تركيز"},
		strong: []string{"concentration", "التركيز", "قوهالتركيز", "الجرعهالدوائيه"},
		weak:   []string{"strength", "dose", "الجرعه"},
	},
	{
		field:   FieldUnit,
		exact:   []string{"unit", "uom", "packaging", "pack", "الوحده", "وحدهالقياس", "العبوه", "التعبئه"},
		strong:  []string{"unitofmeasure", "packsize", "وحدهالقياس", "حجمالعبوه", "نوعالعبوه"},
		weak:    []string{"unit", "uom", "pack", "الوحده", "العبوه"},
		blocked: []string{"unitprice", "سعرالوحده", "unitcost"},
	},
	{
		field:   FieldDescriptionAR,
		exact:   []string{"descriptionar", "notes", "remarks", "الوصف", "الوصفبالعربي", "ملاحظات", "دواعيالاستعمال", "التفاصيل"},
		strong:  []string{"descriptionar", "الوصفبالعربي", "دواعيالاستعمال", "وصفالمنتج", "تفاصيلالصنف"},
		weak:    []string{"الوصف", "notes", "remarks", "ملاحظات"},
		blocked: []string{"itemdescription", "itemdesc", "shortdescription", "وصفالصنف"},
	},
	{
		field:  FieldDescriptionEN,
		exact:  []string{"descriptionen", "الوصفبالانجليزي", "englishdescription"},
		strong: []string{"descriptionen", "englishdescription", "الوصفبالانجليز"},
	},
	{
		field:   FieldQuantity,
		exact:   []string{"quantity", "qty", "stock", "الكميه", "الرصيد", "المخزون", "الكميهالمتوفره"},
		strong:  []string{"quantityavailable", "stockquantity", "الكميهالمتوفره", "الرصيدالمتاح", "كميهالمخزون"},
		weak:    []string{"quantity", "qty", "stock", "الكميه", "الرصيد"},
		blocked: []string{"minqty", "maxqty", "الحدالادني", "الحدالاقصي"},
	},
	{
		field: FieldCategory,
		exact: []string{
			"category", "categoryname", "التصنيف", "الفئة", "فئه", "تصنيف",
			"القسم", "المجموعه", "فئهالمنتج", "تصنيفالمنتج", "نوعالمنتج",
		},
		strong: []string{
			"productcategory", "categoryname", "فئهالمنتج", "تصنيفالمنتج",
			"القسمالرئيسي", "المجموعهالرئيسيه",
		},
		weak: []string{"category", "التصنيف", "الفئه", "القسم", "المجموعه"},
		// A "تصنيف" that is really a sub-classification of the dosage form, or
		// a category code rather than a name, is not this field.
		blocked: []string{"كودالتصنيف", "categorycode", "categoryid", "subcategory"},
	},
	{
		field:   FieldStatus,
		exact:   []string{"status", "state", "الحاله", "حالهالصنف", "نشط"},
		strong:  []string{"productstatus", "حالهالصنف", "حالهالمنتج"},
		weak:    []string{"status", "state", "الحاله"},
		blocked: []string{"orderstatus", "paymentstatus", "حالهالطلب"},
	},
}

// init folds every synonym through the same normaliser the header cells go
// through, so a match is a plain string comparison and neither side can drift.
func init() {
	fold := func(in []string) []string {
		out := in[:0]
		seen := map[string]bool{}
		for _, s := range in {
			key := NormalizeKey(s)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
		return out
	}
	for i := range fieldSpecs {
		fieldSpecs[i].exact = fold(fieldSpecs[i].exact)
		fieldSpecs[i].strong = fold(fieldSpecs[i].strong)
		fieldSpecs[i].weak = fold(fieldSpecs[i].weak)
		fieldSpecs[i].blocked = fold(fieldSpecs[i].blocked)
	}
}

// ColumnBinding records one resolved field-to-column decision, so the import
// report can show the admin exactly how their file was read.
type ColumnBinding struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Header string `json:"header"`
	Index  int    `json:"index"`
	Score  int    `json:"score"`
}

// ColumnPlan is the outcome of reading a header row.
type ColumnPlan struct {
	// Columns maps a field constant to its zero-based column index.
	Columns map[string]int
	// Bindings lists the same decisions with their evidence, ordered by column.
	Bindings []ColumnBinding
	// Unmapped holds header labels that matched no field. They are shown to the
	// admin rather than dropped silently: an unmapped "سعر الجمهور" is the
	// difference between a correct catalogue and a wrong one.
	Unmapped []string
	// Positional is true when no usable header was found and the plan falls back
	// to column order. Callers warn loudly in that case.
	Positional bool
}

// Has reports whether a field was bound to a column.
func (p ColumnPlan) Has(field string) bool {
	_, ok := p.Columns[field]
	return ok
}

// scoreHeader rates how well one header cell identifies one field.
func scoreHeader(spec fieldSpec, normalized string) int {
	if normalized == "" {
		return 0
	}
	for _, b := range spec.blocked {
		if strings.Contains(normalized, b) {
			return 0
		}
	}
	for _, e := range spec.exact {
		if normalized == e {
			return scoreExact
		}
	}
	best := 0
	for _, s := range spec.strong {
		if strings.Contains(normalized, s) {
			// A longer matched phrase is stronger evidence than a shorter one,
			// which separates "سعر البيع للجمهور" from a bare "سعر".
			if v := scoreStrong + len(s); v > best {
				best = v
			}
		}
	}
	if best > 0 {
		return best
	}
	for _, w := range spec.weak {
		if strings.Contains(normalized, w) {
			if v := scoreWeak + len(w)/2; v > best {
				best = v
			}
		}
	}
	return best
}

// candidate is one scored (field, column) pair awaiting assignment.
type candidate struct {
	field  string
	column int
	score  int
}

// PlanColumns resolves a header row into a field-to-column plan.
//
// Assignment is global and greedy on score: the strongest pair wins, and both
// its field and its column are then taken. A column therefore serves exactly one
// field, which is why a header like "اسم الصنف التجاري / الوصف" is bound to the
// product name and not also duplicated into the description — the previous
// mapper wrote the name into both columns of every such file.
func PlanColumns(headerRow []string) ColumnPlan {
	plan := ColumnPlan{Columns: map[string]int{}}
	if len(headerRow) == 0 {
		return plan
	}

	normalized := make([]string, len(headerRow))
	for i, cell := range headerRow {
		normalized[i] = NormalizeKey(cell)
	}

	var cands []candidate
	for _, spec := range fieldSpecs {
		for col, norm := range normalized {
			if score := scoreHeader(spec, norm); score >= scoreMin {
				cands = append(cands, candidate{field: spec.field, column: col, score: score})
			}
		}
	}

	// Highest score first; ties resolve by leftmost column so the result is
	// deterministic for a file with two identically named columns.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].column < cands[j].column
	})

	takenCol := make(map[int]bool, len(headerRow))
	for _, c := range cands {
		if _, done := plan.Columns[c.field]; done {
			continue
		}
		if takenCol[c.column] {
			continue
		}
		plan.Columns[c.field] = c.column
		takenCol[c.column] = true
		plan.Bindings = append(plan.Bindings, ColumnBinding{
			Field:  c.field,
			Label:  FieldLabels[c.field],
			Header: CleanCellString(headerRow[c.column]),
			Index:  c.column,
			Score:  c.score,
		})
	}

	for col, norm := range normalized {
		if norm == "" || takenCol[col] {
			continue
		}
		plan.Unmapped = append(plan.Unmapped, CleanCellString(headerRow[col]))
	}

	sort.Slice(plan.Bindings, func(i, j int) bool { return plan.Bindings[i].Index < plan.Bindings[j].Index })
	return plan
}

// MapHeaderColumns resolves a header row to a field-to-index map.
//
// Retained as the narrow entry point used by callers that only need the
// mapping; PlanColumns carries the evidence the import report renders.
func MapHeaderColumns(headerRow []string) map[string]int {
	return PlanColumns(headerRow).Columns
}
