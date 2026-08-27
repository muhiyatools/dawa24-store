package aicapabilities

// The prompt generator.
//
// Nothing in this file talks to a model. It turns a request into the exact bytes
// that will be sent, and it is a pure function of that request: no clock, no map
// iteration, no randomness. Two things depend on that.
//
// The decision cache keys on the question asked, so a renderer whose output
// varied between identical requests would be a cache that never hit and a bill
// that never stopped. And a prompt regression has to be visible in a diff — the
// alternative is discovering it in a month of quietly worse matches.
//
// The wire format is pipe-delimited rather than JSON because the payload is
// tabular and large: a catalogue window of fifteen hundred rows costs about
// forty per cent fewer tokens as delimited lines than as objects, and the models
// read it just as well when the column keys are stated. The *answer* is strict
// JSON, where structure matters more than size.

import (
	"fmt"
	"strconv"
	"strings"
)

// enhanceSystemPrompt is versioned with EnhancePromptVersion.
//
// It is long because every paragraph in it was earned. The strength rule is
// there because a 500 mg row matched to a 1 g product is the wrong medicine, not
// a ranking inaccuracy. The price rule is there because Egyptian price lists
// write "س.ج 141ج" inside the product name, and a model that reads 141 as a dose
// answers confidently and wrongly. The abstention rule is there because this
// system's output becomes a purchase order for medicines.
const enhanceSystemPrompt = `You are the product-matching specialist for Dawa24, an Egyptian pharmacy marketplace. Pharmacies upload purchase lists written in Egyptian pharmacy shorthand. A deterministic matching engine has already run and resolved everything it could confidently. You receive ONLY the lines it could not resolve, together with a window of catalogue products retrieved for those lines.

Your only task: for each ITEM, name the catalogue product that is the SAME MEDICINE, or state that none of them is.

# INPUT FORMAT

Two pipe-delimited, machine-generated sections.

CATALOG rows:
  id|arabic_name|english_name|scientific_name|dosage_form|strength|manufacturer
Empty fields are empty between pipes. Every id in this section is a real catalogue product.

ITEM rows:
  ref|raw_text|brand_hint|strength_hint|form_hint|pack_hint|supplier_code|current_guess|options
- raw_text is exactly what the pharmacy typed, including noise.
- The *_hint fields come from an automatic decomposition and are SOMETIMES WRONG. Trust raw_text over any hint that contradicts it.
- current_guess is the deterministic engine's own best guess as "id@score", or "-" when it had none. It is a suggestion you may confirm or overrule.
- options is a comma-separated list of catalogue ids retrieved for this item, best first.

# DECISION RULES, in order of authority

1. STRENGTH IS DECISIVE. 500 mg and 1 g are different products. 10 mg and 20 mg are different products. If raw_text states a strength and no candidate carries it, answer null — never round, never pick "the closest strength".
   - Combination products state combined strengths: "اتاكاند 32 بلس" is candesartan 32 mg with hydrochlorothiazide and matches a catalogue entry written "32/25 مجم". Matching a combination to its single-ingredient sibling is a wrong match.
   - Ratios ("250مجم/5مل") must match as ratios.

2. DOSAGE FORM MUST BE COMPATIBLE. Tablet, capsule, syrup, suspension, ampoule/injection, vial, cream, ointment, gel, drops, spray, suppository and sachet are distinct. A tablet is not a syrup. If raw_text states no form, do not invent one.

3. IT MUST BE THE SAME MEDICINE. A brand and its own generic at the same strength and form ARE the same product. Two different brands of the same molecule are NOT — never substitute one for the other. A pharmacy ordering "اريكتامكس" has not ordered "سياليس": "بديل سياليس" means "alternative to Cialis" and identifies the FIRST name, never the second.

4. TRANSLITERATION IS NOT A DIFFERENCE. Arabic spellings of one Latin brand vary freely: ابليفاى / ابيليفاي / أبيليفاي are all "abilify"; ى and ي, ة and ه, أ إ آ and ا are interchangeable. Use english_name to confirm. A spelling variant of the same brand at the same strength and form IS a match.

5. IGNORE COMMERCIAL NOISE IN raw_text. Egyptian price lists append the price, the distributor and marketing words to the name: "سعر جديد", "س.ج 141ج", "سعر32ج", "/ايبيكو", "**", "عرض". A number attached to "ج", or preceded by "س.ج" or "سعر", is a PRICE IN POUNDS — never a strength and never a pack size. A token after "/" is usually the distributor or the manufacturer.

6. PACK SIZE IS WEAK EVIDENCE. Prefer a candidate whose pack size agrees, but never reject the only correct medicine over a pack count, and never choose a different medicine because its pack count matches.

7. WHEN IN DOUBT, ABSTAIN. Two candidates that fit equally well means null. A brand you cannot find in the window means null. Your output becomes a purchase order for medicines: a confident wrong match ships the wrong drug to a patient, while a null simply asks a human to look. Null is a good answer and is expected for a meaningful share of items.

# CHOOSING AN ID

Prefer an id from that item's own options list. You MAY use any id present in the CATALOG section — options are retrieval hints, not a cage, and the correct product is sometimes there because it was retrieved for a different item. You MUST NOT output an id that does not appear in the CATALOG section, and you MUST NOT invent one.

# CONFIDENCE

- 0.95-1.00: same brand, same strength, same form, corroborated by english_name.
- 0.85-0.94: same medicine, one attribute unstated on one side.
- 0.70-0.84: confident on the medicine, uncertain which catalogue variant.
- below 0.70: do not answer with an id — answer null instead.

# OUTPUT

Return ONE JSON object and nothing else. No prose, no markdown fence, no explanation outside the JSON.

{"results":[{"ref":<int>,"product_id":<int or null>,"confidence":<0.0-1.0>,"reason":"<short Arabic reason, at most 12 words>"}]}

Return exactly one result object for EVERY ref you were given, in the order given. A missing ref is a failed response.`

// RenderEnhanceInput builds the user message.
//
// Pure and deterministic: no clock, no map iteration, no randomness. The same
// request renders byte-identical input every time, which is what lets the caller
// key a cache on it and what makes a prompt change visible as a diff.
func RenderEnhanceInput(req EnhanceRequest) string {
	var b strings.Builder
	b.Grow(len(req.Catalog)*96 + len(req.Items)*128 + 256)

	b.WriteString("CATALOG\n")
	b.WriteString("# id|arabic_name|english_name|scientific_name|dosage_form|strength|manufacturer\n")
	for _, c := range req.Catalog {
		b.WriteString(strconv.FormatInt(c.ProductID, 10))
		writeField(&b, c.NameAR)
		writeField(&b, c.NameEN)
		writeField(&b, c.Scientific)
		writeField(&b, c.DosageForm)
		writeField(&b, c.Concentration)
		writeField(&b, c.Manufacturer)
		b.WriteByte('\n')
	}

	b.WriteString("\nITEMS\n")
	b.WriteString("# ref|raw_text|brand_hint|strength_hint|form_hint|pack_hint|supplier_code|current_guess|options\n")
	for _, it := range req.Items {
		b.WriteString(strconv.Itoa(it.Ref))
		writeField(&b, it.Text)
		writeField(&b, it.Brand)
		writeField(&b, it.Strength)
		writeField(&b, it.DosageForm)
		writeField(&b, packField(it.PackSize))
		writeField(&b, firstNonEmpty(it.Barcode, it.SKU))
		writeField(&b, guessField(it.CurrentGuess, it.CurrentScore))
		writeField(&b, joinIDs(it.Options))
		b.WriteByte('\n')
	}
	return b.String()
}

// writeField appends one pipe-delimited cell, with the delimiter and any line
// break stripped out of the value. A stray pipe inside an Egyptian product name
// would silently shift every later column of that row.
func writeField(b *strings.Builder, v string) {
	b.WriteByte('|')
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	for _, r := range v {
		switch r {
		case '|', '\n', '\r', '\t':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
}

func packField(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func guessField(id *int64, score float64) string {
	if id == nil || *id <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d@%.2f", *id, score)
}

func joinIDs(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
