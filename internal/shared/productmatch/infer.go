package productmatch

import (
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Inference from the product name.
//
// The great majority of Egyptian supplier files carry three columns — name,
// price, discount — and nothing else. The pharmaceutical form and the strength
// are in the name, written the way a pharmacist would say them, and pulling
// them out is the difference between a catalogue that can be filtered and a
// catalogue that can only be scrolled.
//
// Only the form is a genuine inference. The concentration is quoted out of the
// name verbatim, so it states what the supplier wrote rather than guessing on
// their behalf.

// dosageKeywords maps words that appear in an Egyptian product name onto the
// pharmaceutical form. Longer, more specific entries come first: "غسول فم" must
// beat "غسول", and "معجون أسنان" must beat any bare match.
var dosageKeywords = []struct {
	words []string
	form  string
}{
	{[]string{"غسول فم", "مضمضة", "مضمضه", "mouthwash"}, "غسول فم"},
	{[]string{"معجون اسنان", "معجون أسنان", "toothpaste"}, "معجون أسنان"},
	{[]string{"غسول", "wash", "lotion", "لوشن"}, "غسول"},
	{[]string{"كريم", "cream"}, "كريم"},
	{[]string{"مرهم", "ointment", "oint"}, "مرهم"},
	{[]string{"جل", "جيل", "gel"}, "جل"},
	{[]string{"زيت", "oil", "اويل"}, "زيت"},
	{[]string{"سيروم", "serum"}, "سيروم"},
	{[]string{"شامبو", "shampoo"}, "شامبو"},
	{[]string{"صابون", "صابونة", "صابونه", "soap"}, "صابون"},
	{[]string{"رول اون", "roll on", "رول-اون"}, "رول اون"},
	{[]string{"اسبراي", "سبراي", "اسبراى", "سبراى", "بخاخ", "spray", "بدى ميست", "body mist"}, "بخاخ / اسبراي"},
	{[]string{"مناديل", "wipes"}, "مناديل مبللة"},
	{[]string{"صبغة", "صبغه", "hair color", "colour"}, "صبغة شعر"},
	{[]string{"اقراص", "أقراص", "قرص", "ق ", "tab", "tabs", "tablet", "tablets"}, "أقراص"},
	{[]string{"كبسول", "كبسولات", "كبسولة", "كبسوله", "cap", "caps", "capsule", "capsules"}, "كبسولات"},
	{[]string{"شراب", "syrup", "susp", "معلق"}, "شراب"},
	{[]string{"نقط", "drops", "قطرة", "قطره"}, "نقط"},
	{[]string{"حقن", "حقنة", "حقنه", "امبول", "أمبول", "امبولات", "فيال", "vial", "ampoule", "inj"}, "حقن وأمبولات"},
	{[]string{"فوار", "ساشيت", "sachet", "eff"}, "أكياس فوار"},
	{[]string{"لبوس", "تحاميل", "تحميلة", "supp", "suppository"}, "لبوس"},
	{[]string{"حفاضات", "حفاضه", "diapers"}, "مستلزمات عناية"},
	{[]string{"استيك", "ستيك", "stick"}, "ستيك"},
	{[]string{"بودرة", "بودره", "powder", "بودر"}, "بودرة"},
	{[]string{"شريط", "strip"}, "أقراص"},
}

// DefaultDosageForm labels a product whose name gives no clue about its form.
const DefaultDosageForm = "مستحضر صيدلاني"

// strengthPattern matches a dose written the way it is printed on a box — and
// the several ways an Egyptian distributor abbreviates it in a spreadsheet.
//
// "مج" and "مغ" are milligrams and appear throughout real supplier files
// ("بروفين 400 مج اقراص"); "محم" is the ج/ح keyboard slip for مجم and appears
// often enough in live data to be worth reading rather than discarding. Missing
// any of them does not merely lose the strength — it leaves the figure loose in
// the name, where it competes with the brand as an ordinary token and the
// strength veto that keeps 400 mg away from 600 mg never fires.
//
// The alternation is longest-first: Go's regexp prefers the leftmost-first
// branch, so "مج" listed before "مجم" would match the first two letters of
// "مجم" and leave a stray "م" behind.
var strengthPattern = regexp.MustCompile(
	`(?i)(\d+(?:[./]\d+)?\s*(?:ملجرام|مليجرام|مجم|مكجم|محم|جرام|مج|مغ|جم|مللي|ملي|مل|وحدة|وحده|mg|mcg|gm|g|ml|l|iu|%|spf[+\d]*))`)

// InferDosageForm reads the pharmaceutical form out of a product name.
func InferDosageForm(name string) string {
	if name == "" {
		return DefaultDosageForm
	}
	lowered := strings.ToLower(sheet.CleanCell(name))
	for _, dk := range dosageKeywords {
		for _, w := range dk.words {
			if strings.Contains(lowered, w) {
				return dk.form
			}
		}
	}
	return DefaultDosageForm
}

// InferConcentration quotes the strength out of a product name, or returns
// empty when the name states none.
func InferConcentration(name string) string {
	if name == "" {
		return ""
	}
	if m := strengthPattern.FindString(sheet.NormalizeDigits(name)); m != "" {
		return sheet.CleanCell(m)
	}
	return ""
}

// packPattern matches the pack count Egyptian names carry: "30 قرص", "14كبسولة",
// "3 امبول", "20ق".
var packPattern = regexp.MustCompile(
	`(?i)(\d{1,4})\s*(قرص|اقراص|أقراص|ق|كبسول|كبسوله|كبسولة|كبسولات|امبول|أمبول|امبولة|امبولات|كيس|اكياس|شريط|لبوس|tab|tabs|cap|caps|amp)\b`)

// InferPackSize reads the number of units in the pack out of a product name.
//
// It is used only where the file has no pack-size column, and only for display
// and matching — never to derive a price. A pack count read wrongly out of a
// name would otherwise silently divide or multiply what a pharmacy pays.
func InferPackSize(name string) int {
	m := packPattern.FindStringSubmatch(sheet.NormalizeDigits(name))
	if len(m) < 2 {
		return 0
	}
	n, err := sheet.CoerceInt(m[1])
	if err != nil || n <= 0 || n > 5000 {
		return 0
	}
	return int(n)
}
