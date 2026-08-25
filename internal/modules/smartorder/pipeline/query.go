package pipeline

import (
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Preparing a pharmacy's line for the matcher.
//
// The shared matcher was tuned for **vendor price lists**, where a supplier's
// line is terse and already close to the catalogue name. A pharmacy's purchase
// list is not that. It is descriptive:
//
//	بروفين مسكن ومضاد للالتهاب        ("Brufen painkiller and anti-inflammatory")
//	كونجستال أقراص للبرد والاحتقان    ("Congestal tablets for cold and congestion")
//
// The catalogue records those products as "بروفين 400مج 20ق" — the therapeutic
// description appears nowhere in its name. Handed the raw line, the scorer
// divides matched weight by the whole query's weight, so five words of prose
// against two matching ones drags a correct match below the floor. Measured on
// the live catalogue it did worse than that: "بانادول إكسترا 500 مجم أقراص"
// matched a **Sensodyne toothbrush**, because the only surviving shared token
// was "اكسترا".
//
// So the line is decomposed before it reaches the scorer: the strength, the
// form and the pack size move into the structured fields the matcher already
// compares separately, and the therapeutic prose is dropped. What is left is
// the brand and its distinguishing words — which is what the catalogue's name
// actually contains.

// strengthPattern captures a dose and its unit, in either script.
var strengthPattern = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(مجم|ملجم|مغ|مج|جم|جرام|جم|مل|ملل|ميكروجرام|mg|mcg|g|gm|ml|iu|وحدة)\b`)

// packPattern captures a pack count: "20 قرص", "24قرص", "20 tabs".
var packPattern = regexp.MustCompile(`(?i)(\d+)\s*(قرص|أقراص|اقراص|كبسولة|كبسول|كبسولات|امبول|أمبول|tab|tabs|caps|capsule)\b`)

// formWords map a dosage form written in a line onto a canonical term.
//
// Kept small and specific. A form word left in the name is not merely useless —
// it is actively harmful, because it is common enough to pull in every product
// that happens to share it.
var formWords = map[string]string{
	"أقراص": "أقراص", "اقراص": "أقراص", "قرص": "أقراص", "قروص": "أقراص",
	"كبسول": "كبسولات", "كبسولة": "كبسولات", "كبسولات": "كبسولات",
	"شراب": "شراب", "شرب": "شراب",
	"حقن": "حقن", "امبول": "حقن", "أمبول": "حقن", "حقنة": "حقن",
	"بخاخ": "بخاخ", "سبراى": "بخاخ", "سبراي": "بخاخ",
	"كريم": "كريم", "مرهم": "مرهم", "جل": "جل",
	"نقط": "نقط", "قطرة": "نقط", "قطره": "نقط",
	"لبوس": "لبوس", "تحاميل": "لبوس",
	"tab": "أقراص", "tabs": "أقراص", "capsule": "كبسولات", "caps": "كبسولات",
	"syrup": "شراب", "cream": "كريم", "gel": "جل", "drops": "نقط",
}

// therapeuticFiller is prose that describes what a medicine *does*.
//
// A pharmacist writes it for their own benefit; the catalogue never carries it.
// Every one of these words was observed poisoning a real match on the live
// catalogue — "مسكن" and "سريع" together are what matched a painkiller to a
// toothbrush advertising fast relief.
var therapeutic = map[string]bool{
	"مسكن": true, "مسكنة": true, "مضاد": true, "مضادة": true, "للالتهاب": true,
	"التهاب": true, "الالتهاب": true, "للبرد": true, "برد": true, "والاحتقان": true,
	"احتقان": true, "الاحتقان": true, "مطهر": true, "معوي": true, "معوى": true,
	"للأنف": true, "للانف": true, "للكبار": true, "للأطفال": true, "للاطفال": true,
	"سريع": true, "سريعة": true, "قوي": true, "قوى": true, "قوية": true,
	"خافض": true, "للحرارة": true, "حرارة": true, "فيتامين": true, "مكمل": true,
	"غذائي": true, "غذائى": true, "تجريبي": true, "تجريبى": true, "جديد": true,
	"توفير": true, "عرض": true, "خصم": true,
	"and": true, "for": true, "with": true, "the": true,
}

// BuildRow turns an imported line into the row the matcher scores.
func BuildRow(l *smartorder.Line) *productmatch.Row {
	raw := strings.TrimSpace(l.RawName)

	row := &productmatch.Row{
		Number:  l.RowNumber,
		SKU:     l.RawSKU,
		Barcode: l.RawBarcode,
	}

	rest := raw

	// Strength: "500 مجم" becomes a structured comparison rather than two more
	// words competing with the brand.
	if m := strengthPattern.FindStringSubmatch(rest); m != nil {
		row.Concentration = strings.TrimSpace(m[1] + " " + m[2])
		rest = strings.Replace(rest, m[0], " ", 1)
	}

	// Pack size, same reasoning.
	if m := packPattern.FindStringSubmatch(rest); m != nil {
		row.PackSize = atoiSafe(m[1])
		if form, ok := formWords[strings.ToLower(m[2])]; ok {
			row.DosageForm = form
		}
		rest = strings.Replace(rest, m[0], " ", 1)
	}

	// Whatever survives: keep the identifying words, drop the description.
	var kept []string
	for _, tok := range strings.Fields(rest) {
		clean := strings.Trim(tok, "()[]{}،,.؛;:-_/\\")
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)

		if form, ok := formWords[lower]; ok {
			if row.DosageForm == "" {
				row.DosageForm = form
			}
			continue
		}
		if therapeutic[lower] {
			continue
		}
		kept = append(kept, clean)
	}

	row.Name = strings.Join(kept, " ")
	if row.Name == "" {
		// Everything was filler. Better to match on the original than on
		// nothing — a poor candidate the buyer can correct beats an unmatched
		// line with no explanation.
		row.Name = raw
	}
	return row
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
