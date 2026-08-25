package engine

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Header evidence.
//
// A synonym table is the cheap half of column detection and it is worth having,
// because most files do label their columns honestly. It is only ever half: the
// scores below feed a resolver that weighs them against what the values
// actually look like, and a strong header can still lose to strong contrary
// evidence.
//
// The vocabulary is drawn from real Egyptian distributor exports rather than
// invented. That is why "القائمة", "المرجح", "جملة" and "مندوب" appear as weak
// hints for the discount: none of those words means "discount" in Arabic, but
// all four label the discount column in files this engine has been run against,
// and a weak hint that the values can confirm is exactly the right strength for
// evidence that circumstantial.

// Header match strengths. The gaps are wide on purpose: an exact header match
// must beat any number of partial ones, and a full multi-word term a field owns
// must beat a single word several fields share.
const (
	scoreExact  = 100
	scoreStrong = 60
	scoreWeak   = 25
	scoreFloor  = scoreWeak
)

// headerSpec is how one field is recognised in a header cell.
//
// exact matches the whole normalised header. strong and weak are substring
// tests. blocked disqualifies the pair outright, which is how "سعر التكلفة" is
// kept away from the public price and "الباركود الدولي" away from the item
// code — the two collisions that quietly corrupt an imported price list.
type headerSpec struct {
	field   Field
	exact   []string
	strong  []string
	weak    []string
	blocked []string
}

// headerSpecs is ordered only for reading; resolution is order independent.
//
// Entries are written the natural way — "الشكل الدوائي", "Item No." — and
// folded through sheet.NormalizeKey at init, exactly as the header cells they
// are compared against are. Hand-folding them in source was tried in the admin
// importer and got the hamza forms wrong, which cost "الشكل الدوائي" its exact
// match and dropped it to a weak partial.
var headerSpecs = []headerSpec{
	{
		field: FieldName,
		exact: []string{
			"اسم الصنف", "إسم الصنف", "اسم المنتج", "اسم الدواء", "الصنف", "صنف",
			"المنتج", "الاسم", "اسم المستحضر", "المستحضر", "الاسم التجاري",
			"اسم الصنف التجاري", "بيان الصنف", "وصف الصنف", "الوصف",
			"item description", "item desc", "itemname", "item name", "description",
			"product name", "product", "name", "trade name", "medicine",
		},
		strong: []string{
			"اسم الصنف", "اسم المنتج", "اسم الدواء", "اسم المستحضر",
			"الاسم التجاري", "اسم الصنف التجاري", "وصف الصنف", "بيان الصنف",
			"item description", "product name", "item name", "product description",
		},
		weak: []string{"اسم", "الصنف", "المنتج", "description", "name", "item"},
		blocked: []string{
			"كود", "code", "باركود", "barcode", "سعر", "price", "خصم", "discount",
			"كميه", "qty", "quantity", "الشركه", "المصنع", "المورد", "manufacturer",
			"vendor", "supplier", "التصنيف", "category", "الوحده", "unit",
			"الحاله", "status", "علمي", "generic", "scientific", "english",
			"انجليز", "تاريخ", "date", "الفرع", "branch", "المخزن",
		},
	},
	{
		field: FieldNameEN,
		exact: []string{
			"name en", "english name", "english", "اسم بالانجليزي",
			"اسم الصنف بالانجليزي", "الاسم بالانجليزية", "الاسم الانجليزي",
			"الاسم اللاتيني",
		},
		strong:  []string{"english name", "english description", "اسم بالانجليز", "الاسم بالانجليز", "اسم لاتيني"},
		weak:    []string{"english", "انجليز", "latin", "لاتيني"},
		blocked: []string{"generic", "scientific", "علمي", "سعر", "price"},
	},
	{
		field: FieldSKU,
		exact: []string{
			"الكود", "كود", "كود الصنف", "كود المنتج", "رقم الصنف", "رقم المنتج",
			"كود المورد", "كود الشركه", "رقم الكود", "code", "sku", "item no",
			"item code", "item id", "product code", "material code", "article code",
			"ref", "reference", "id",
		},
		strong: []string{
			"كود الصنف", "كود المنتج", "رقم الصنف", "رقم المنتج", "كود المورد",
			"item code", "item no", "product code", "material code",
		},
		weak: []string{"كود", "code", "sku", "رقم", "ref"},
		blocked: []string{
			"باركود", "barcode", "ean", "upc", "gtin", "التشغيله", "batch", "lot",
			"الفرع", "المخزن", "تصنيف", "category",
			// A column the file calls a price, a discount, a quantity or a date
			// is not the item code, however code-shaped its values look. Without
			// these, a three-column file of name / price / discount offers the
			// price column as an item-code candidate and the review screen warns
			// about a competition that does not exist.
			"سعر", "price", "ثمن", "خصم", "discount", "الكميه", "quantity",
			"الرصيد", "stock", "تاريخ", "date", "صلاحيه", "expiry",
		},
	},
	{
		field:   FieldBarcode,
		exact:   []string{"الباركود", "باركود", "الباركود الدولي", "باركود دولي", "barcode", "bar code", "ean", "ean13", "upc", "gtin"},
		strong:  []string{"الباركود الدولي", "باركود دولي", "رقم الباركود", "barcode", "ean 13", "gtin"},
		weak:    []string{"باركود", "barcode", "ean", "upc", "gtin"},
		blocked: []string{},
	},
	{
		field:   FieldScientific,
		exact:   []string{"الاسم العلمي", "المادة الفعالة", "المادة النشطة", "الماده الفعاله", "علمي", "generic name", "generic", "scientific name", "scientific", "active ingredient", "active substance", "inn"},
		strong:  []string{"الاسم العلمي", "المادة الفعالة", "المادة النشطة", "المكون الفعال", "generic name", "scientific name", "active ingredient"},
		weak:    []string{"علمي", "generic", "scientific", "فعاله", "active", "ingredient"},
		blocked: []string{"سعر", "price"},
	},
	{
		field: FieldManufacturer,
		exact: []string{
			"الشركة", "الشركة المصنعة", "المصنع", "الشركه المصنعه", "جهة التصنيع",
			"الماركة", "البراند", "العلامة التجارية", "المورد", "اسم المورد",
			"manufacturer", "company", "brand", "vendor", "supplier", "maker",
			"preferred vendor", "producer", "agent",
		},
		strong:  []string{"الشركة المصنعة", "جهة التصنيع", "اسم الشركة", "اسم المورد", "شركة التصنيع", "preferred vendor", "manufacturer name"},
		weak:    []string{"الشركه", "المصنع", "المورد", "الماركه", "company", "brand", "vendor", "supplier"},
		blocked: []string{"كود", "code", "id", "رقم", "سعر", "price"},
	},
	{
		field:   FieldDosageForm,
		exact:   []string{"الشكل الصيدلي", "الشكل الدوائي", "شكل الدواء", "الشكل", "هيئة الدواء", "dosage form", "pharmaceutical form", "form", "dosage"},
		strong:  []string{"الشكل الصيدلي", "الشكل الدوائي", "هيئة الدواء", "dosage form", "pharmaceutical form"},
		weak:    []string{"الشكل", "form", "dosage"},
		blocked: []string{"format", "platform", "performance", "سعر", "price"},
	},
	{
		field:   FieldConcentration,
		exact:   []string{"التركيز", "تركيز", "القوة", "العيار", "الجرعة", "concentration", "strength", "dose", "potency"},
		strong:  []string{"تركيز الدواء", "قوة التركيز", "الجرعة الدوائية", "concentration"},
		weak:    []string{"تركيز", "strength", "dose", "الجرعه", "عيار"},
		blocked: []string{"سعر", "price"},
	},
	{
		field:   FieldUnit,
		exact:   []string{"الوحدة", "وحدة القياس", "العبوة", "نوع العبوة", "التعبئة", "unit", "uom", "pack", "packaging", "package"},
		strong:  []string{"وحدة القياس", "نوع العبوة", "unit of measure", "packaging type"},
		weak:    []string{"الوحده", "العبوه", "unit", "uom", "pack"},
		blocked: []string{"سعر الوحده", "unit price", "unit cost", "عدد", "count", "size", "حجم"},
	},
	{
		field:   FieldPackSize,
		exact:   []string{"عدد الوحدات", "حجم العبوة", "عدد الاقراص", "عدد بالعبوة", "pack size", "units per pack", "qty per pack", "size"},
		strong:  []string{"عدد الوحدات", "حجم العبوة", "عدد الاقراص", "pack size", "units per pack"},
		weak:    []string{"عدد", "size"},
		blocked: []string{"سعر", "price", "الكميه", "quantity", "الرصيد"},
	},
	{
		field:   FieldCategory,
		exact:   []string{"التصنيف", "الفئة", "تصنيف", "فئة", "القسم", "المجموعة", "فئة المنتج", "تصنيف المنتج", "نوع المنتج", "category", "group", "department", "class"},
		strong:  []string{"فئة المنتج", "تصنيف المنتج", "القسم الرئيسي", "المجموعة الرئيسية", "product category"},
		weak:    []string{"التصنيف", "الفئه", "القسم", "المجموعه", "category", "group"},
		blocked: []string{"كود", "code", "id", "sub"},
	},

	{
		field: FieldPublicPrice,
		exact: []string{
			"سعر الجمهور", "سعر ج", "سعر جمهور", "السعر", "سعر", "سعر البيع للجمهور",
			"السعر العام", "سعر قبل الخصم", "سعر العبوة", "سعر الوحدة", "سعر الصنف",
			"public price", "list price", "mrp", "retail price", "price", "unit price",
		},
		strong: []string{
			"سعر الجمهور", "سعر البيع للجمهور", "سعر قبل الخصم", "السعر العام",
			"public price", "retail price", "list price",
		},
		weak:    []string{"سعر", "price", "ثمن", "mrp"},
		blocked: []string{"تكلفه", "شراء", "cost", "purchase", "صافي", "net", "بعد الخصم", "خصم", "discount"},
	},
	{
		field:   FieldPrice,
		exact:   []string{"سعر البيع", "سعر الصيدلية", "سعر التوريد", "سعر المورد", "selling price", "sales price", "supply price", "pharmacy price", "wholesale price"},
		strong:  []string{"سعر البيع", "سعر الصيدلية", "سعر التوريد", "selling price", "pharmacy price", "supply price"},
		weak:    []string{"البيع", "selling", "توريد"},
		blocked: []string{"جمهور", "public", "تكلفه", "cost", "شراء", "purchase", "خصم", "discount"},
	},
	{
		field:   FieldNetPrice,
		exact:   []string{"الصافي", "صافي", "السعر الصافي", "سعر الصافي", "بعد الخصم", "السعر بعد الخصم", "net", "net price", "final price", "price after discount"},
		strong:  []string{"السعر الصافي", "السعر بعد الخصم", "net price", "price after discount", "final price"},
		weak:    []string{"صافي", "net", "بعدالخصم"},
		blocked: []string{"تكلفه", "cost", "جمهور", "public"},
	},
	{
		field: FieldCostPrice,
		exact: []string{
			"سعر التكلفة", "التكلفة", "سعر الشراء", "تكلفة الوحدة",
			"cost", "cost price", "purchase price", "buying price", "buy price",
		},
		strong:  []string{"سعر التكلفة", "سعر الشراء", "تكلفة الوحدة", "cost price", "purchase price", "buying price"},
		weak:    []string{"تكلفه", "شراء", "cost", "purchase"},
		blocked: []string{"جمهور", "public", "بيع", "selling"},
	},
	{
		field: FieldDiscountPct,
		exact: []string{
			"الخصم", "خصم", "نسبة الخصم", "نسبه الخصم", "الخصم التجاري",
			"خصم اساسي", "خصم أساسى", "خصم خاص", "الخصم الممنوح", "نسبة",
			"discount", "disc", "discount percent", "discount percentage",
			"discount rate", "disc %", "rebate",
		},
		strong: []string{
			"نسبة الخصم", "الخصم التجاري", "خصم اساسي", "خصم خاص",
			"الخصم الممنوح", "discount percent", "discount rate", "discount %",
		},
		// The circumstantial vocabulary. Every one of these labels a discount
		// column in a real distributor file and says nothing on its own; the
		// value evidence is what promotes or drops them.
		weak: []string{
			"خصم", "discount", "disc", "نسبه", "تخفيض",
			"القائمه", "المرجح", "جمله", "ج الجمله", "مندوب", "العموله",
		},
		blocked: []string{"بعد الخصم", "قبل الخصم", "after discount", "قيمة الخصم", "مبلغ الخصم"},
	},
	{
		field:   FieldDiscountAmt,
		exact:   []string{"قيمة الخصم", "مبلغ الخصم", "discount amount", "discount value"},
		strong:  []string{"قيمة الخصم", "مبلغ الخصم", "discount amount", "discount value"},
		weak:    []string{},
		blocked: []string{"نسبه", "percent", "rate"},
	},
	{
		field:   FieldBonus,
		exact:   []string{"البونص", "بونص", "العرض", "عرض", "الهدية", "هدية", "bonus", "offer", "free", "promo", "deal"},
		strong:  []string{"عرض الكمية", "بونص الكمية", "free goods", "bonus qty"},
		weak:    []string{"بونص", "عرض", "bonus", "offer", "هديه"},
		blocked: []string{"سعر", "price", "خصم", "discount"},
	},

	{
		field: FieldQuantity,
		exact: []string{
			"الكمية", "الكميه", "كمية", "الرصيد", "المخزون", "العدد", "المتاح",
			"الكمية المتوفرة", "الرصيد المتاح", "ارصدة", "أرصدة", "رصيد",
			"quantity", "qty", "stock", "balance", "available", "on hand", "count",
		},
		strong:  []string{"الكمية المتوفرة", "الرصيد المتاح", "كمية المخزون", "available quantity", "stock quantity", "on hand"},
		weak:    []string{"الكميه", "الرصيد", "المخزون", "quantity", "qty", "stock", "balance"},
		blocked: []string{"ادني", "اقل", "min", "حد", "طلب", "order", "moq", "اقصي", "max", "سعر", "price", "عبوه", "pack"},
	},
	{
		field:   FieldMinOrderQty,
		exact:   []string{"اقل كمية", "أقل كمية للطلب", "الحد الادني للطلب", "الحد الأدنى للطلب", "min order", "min order qty", "minimum order", "moq"},
		strong:  []string{"اقل كمية للطلب", "الحد الادني للطلب", "min order qty", "minimum order quantity"},
		weak:    []string{"moq", "min order", "اقل كميه"},
		blocked: []string{"مخزون", "stock", "reorder", "اعاده"},
	},
	{
		field:   FieldMinThreshold, // .
		exact:   []string{"حد الطلب", "حد اعادة الطلب", "الحد الادني للمخزون", "حد التنبيه", "reorder level", "reorder point", "min stock", "safety stock", "minimum stock"},
		strong:  []string{"حد اعادة الطلب", "الحد الادني للمخزون", "reorder level", "min stock level", "safety stock"},
		weak:    []string{"reorder", "حد التنبيه"},
		blocked: []string{"طلب الشراء", "purchase order"},
	},
	{
		field:   FieldBatchNumber,
		exact:   []string{"رقم التشغيلة", "التشغيلة", "تشغيلة", "رقم الباتش", "الباتش", "الطبخة", "batch", "batch no", "batch number", "lot", "lot no", "lot number"},
		strong:  []string{"رقم التشغيلة", "رقم الباتش", "batch number", "lot number"},
		weak:    []string{"تشغيله", "باتش", "batch", "lot", "الطبخه"},
		blocked: []string{"تاريخ", "date", "صلاحيه"},
	},
	{
		field:   FieldExpiryDate,
		exact:   []string{"تاريخ الصلاحية", "الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "الانتهاء", "expiry", "expiry date", "exp", "exp date", "expiration", "expiration date", "best before"},
		strong:  []string{"تاريخ الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "expiry date", "expiration date"},
		weak:    []string{"صلاحيه", "انتهاء", "expiry", "exp"},
		blocked: []string{"الانتاج", "production", "تصنيع", "manufactur"},
	},
	{
		field:   FieldWarehouse,
		exact:   []string{"المخزن", "مخزن", "المستودع", "مستودع", "warehouse", "store", "location", "depot"},
		strong:  []string{"اسم المخزن", "كود المخزن", "warehouse name", "warehouse code"},
		weak:    []string{"مخزن", "مستودع", "warehouse", "store"},
		blocked: []string{"الرصيد", "الكميه", "quantity", "ارصده"},
	},
	{
		field:   FieldBranch,
		exact:   []string{"الفرع", "فرع", "branch", "outlet"},
		strong:  []string{"اسم الفرع", "كود الفرع", "branch name", "branch code"},
		weak:    []string{"فرع", "branch"},
		blocked: []string{},
	},
	{
		field:   FieldStatus,
		exact:   []string{"الحالة", "حالة الصنف", "الحاله", "نشط", "status", "state", "active"},
		strong:  []string{"حالة الصنف", "حالة المنتج", "product status"},
		weak:    []string{"الحاله", "status", "state"},
		blocked: []string{"حالة الطلب", "order status", "payment"},
	},
	{
		field:   FieldNegotiable,
		exact:   []string{"قابل للتفاوض", "تفاوض", "التفاوض", "negotiable", "negotiation"},
		strong:  []string{"قابل للتفاوض", "negotiable"},
		weak:    []string{"تفاوض", "negotiab"},
		blocked: []string{},
	},
	{
		field:   FieldImage,
		exact:   []string{"الصورة", "الصوره", "صورة", "رابط الصورة", "image", "img", "picture", "photo", "image url", "image link"},
		strong:  []string{"رابط الصورة", "image url", "image link", "product image"},
		weak:    []string{"صوره", "image", "img", "photo"},
		blocked: []string{},
	},
	{
		field:   FieldNotes,
		exact:   []string{"ملاحظات", "ملاحظة", "بيان", "تعليق", "notes", "note", "remarks", "comment", "comments"},
		strong:  []string{"ملاحظات", "notes", "remarks"},
		weak:    []string{"ملاحظه", "notes", "remark", "comment"},
		blocked: []string{"بيان الصنف", "item description"},
	},
}

// foldedSpecs is headerSpecs with every synonym run through the same normaliser
// the header cells go through, so a match is a plain string comparison and
// neither side can drift.
var foldedSpecs = func() []headerSpec {
	fold := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]bool{}
		for _, s := range in {
			key := sheet.NormalizeKey(s)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
		return out
	}
	out := make([]headerSpec, len(headerSpecs))
	for i, spec := range headerSpecs {
		out[i] = headerSpec{
			field:   spec.field,
			exact:   fold(spec.exact),
			strong:  fold(spec.strong),
			weak:    fold(spec.weak),
			blocked: fold(spec.blocked),
		}
	}
	return out
}()

// scoreHeader rates how well one normalised header cell names one field, and
// reports whether the pair was disqualified outright.
func scoreHeader(spec headerSpec, key string) (score int, blocked bool) {
	if key == "" {
		return 0, false
	}
	for _, b := range spec.blocked {
		if strings.Contains(key, b) {
			return 0, true
		}
	}
	for _, e := range spec.exact {
		if key == e {
			return scoreExact, false
		}
	}
	best := 0
	for _, s := range spec.strong {
		if strings.Contains(key, s) {
			// A longer matched phrase is stronger evidence than a shorter one,
			// which is what separates "سعر البيع للجمهور" from a bare "سعر".
			if v := scoreStrong + len([]rune(s)); v > best {
				best = v
			}
		}
	}
	if best > 0 {
		return best, false
	}
	for _, w := range spec.weak {
		if strings.Contains(key, w) {
			if v := scoreWeak + len([]rune(w))/2; v > best {
				best = v
			}
		}
	}
	return best, false
}

// HeaderEvidence is what the synonym table made of one column for one field.
type HeaderEvidence struct {
	Score   int
	Blocked bool
}

// headerEvidence scores one header cell against every field.
func headerEvidence(header string) map[Field]HeaderEvidence {
	key := sheet.NormalizeKey(header)
	out := make(map[Field]HeaderEvidence, len(foldedSpecs))
	for _, spec := range foldedSpecs {
		score, blocked := scoreHeader(spec, key)
		if score == 0 && !blocked {
			continue
		}
		out[spec.field] = HeaderEvidence{Score: score, Blocked: blocked}
	}
	return out
}

// HeaderLooksLikeAField reports whether a cell names any known field at all. It
// is how a header row is told from a data row.
func HeaderLooksLikeAField(cell string) bool {
	key := sheet.NormalizeKey(cell)
	if key == "" {
		return false
	}
	for _, spec := range foldedSpecs {
		if score, blocked := scoreHeader(spec, key); score >= scoreFloor && !blocked {
			return true
		}
	}
	return false
}
