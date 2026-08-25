package ingest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// MasterProductData contains indexed fields from catalog.products for in-memory matching.
type MasterProductData struct {
	ID                 int64
	NameAR             string
	NameEN             string
	NormalizedNameAR   string
	NormalizedNameEN   string
	SKU                string
	Barcode            string
	DosageForm         string
	DosageFormNorm     string
	Concentration      string
	ConcentrationNorm  string
	Unit               string
	Manufacturer       string
	ManufacturerNorm   string
	ScientificName     string
	ScientificNameNorm string
	PublicPrice        string
	ConcentrationNum   float64
	ConcentrationUnit  string
	FormKey            string
}

// SavingProductData contains indexed entries from catalog.saving_products.
type SavingProductData struct {
	ProductID      int64
	NameProduct    string
	NormalizedName string
	SKU            string
}

// CatalogMatchIndex provides high-performance multi-index candidate lookups.
type CatalogMatchIndex struct {
	byBarcode     map[string]*MasterProductData
	bySKU         map[string]*MasterProductData
	byExactName   map[string]*MasterProductData
	bySavingsName map[string]*MasterProductData
	bySavingsSKU  map[string]*MasterProductData
	savingsTokens map[string][]string
	tokenIndex    map[string][]*MasterProductData
	trigramIndex  map[string][]*MasterProductData
	allProducts   []*MasterProductData
	productsByID  map[int64]*MasterProductData
}

// NewCatalogMatchIndex builds in-memory inverted indices from master catalog and saving products.
func NewCatalogMatchIndex(
	masterProducts []*MasterProductData,
	savingProducts []*SavingProductData,
) *CatalogMatchIndex {
	idx := &CatalogMatchIndex{
		byBarcode:     make(map[string]*MasterProductData),
		bySKU:         make(map[string]*MasterProductData),
		byExactName:   make(map[string]*MasterProductData),
		bySavingsName: make(map[string]*MasterProductData),
		bySavingsSKU:  make(map[string]*MasterProductData),
		savingsTokens: make(map[string][]string),
		tokenIndex:    make(map[string][]*MasterProductData),
		trigramIndex:  make(map[string][]*MasterProductData),
		allProducts:   masterProducts,
		productsByID:  make(map[int64]*MasterProductData),
	}

	for _, p := range masterProducts {
		if p == nil || p.ID <= 0 {
			continue
		}
		p.NormalizedNameAR = normalizePharmaceutical(p.NameAR)
		p.NormalizedNameEN = normalizePharmaceutical(p.NameEN)
		p.DosageFormNorm = normalizePharmaceutical(p.DosageForm)
		p.ConcentrationNorm = normalizePharmaceutical(p.Concentration)
		p.ManufacturerNorm = normalizePharmaceutical(p.Manufacturer)
		p.ScientificNameNorm = normalizePharmaceutical(p.ScientificName)
		p.ConcentrationNum, p.ConcentrationUnit = extractConcentration(p.NameAR + " " + p.NameEN + " " + p.Concentration)
		p.FormKey = extractFormKey(p.NameAR + " " + p.NameEN + " " + p.DosageForm)

		idx.productsByID[p.ID] = p

		if cleanBarcode := strings.TrimSpace(p.Barcode); cleanBarcode != "" {
			idx.byBarcode[cleanBarcode] = p
		}
		if cleanSKU := strings.ToUpper(strings.TrimSpace(p.SKU)); cleanSKU != "" {
			idx.bySKU[cleanSKU] = p
		}
		if p.NormalizedNameAR != "" {
			idx.byExactName[p.NormalizedNameAR] = p
		}
		if p.NormalizedNameEN != "" {
			idx.byExactName[p.NormalizedNameEN] = p
		}

		// Build token index
		tokens := extractSignificantTokens(p.NameAR + " " + p.NameEN + " " + p.ScientificName)
		seenTokens := make(map[string]bool)
		for _, tok := range tokens {
			if !seenTokens[tok] {
				seenTokens[tok] = true
				idx.tokenIndex[tok] = append(idx.tokenIndex[tok], p)
			}
		}

		// Build trigram index for fast candidate retrieval
		trigrams := extractTrigrams(p.NormalizedNameAR + " " + p.NormalizedNameEN)
		seenTrigrams := make(map[string]bool)
		for _, tri := range trigrams {
			if !seenTrigrams[tri] {
				seenTrigrams[tri] = true
				idx.trigramIndex[tri] = append(idx.trigramIndex[tri], p)
			}
		}
	}

	for _, s := range savingProducts {
		if s == nil || s.ProductID <= 0 {
			continue
		}
		master, exists := idx.productsByID[s.ProductID]
		if !exists || master == nil {
			continue
		}
		normName := normalizePharmaceutical(s.NameProduct)
		if normName != "" {
			idx.bySavingsName[normName] = master
			idx.savingsTokens[normName] = extractSignificantTokens(normName)
		}
		if cleanSKU := strings.ToUpper(strings.TrimSpace(s.SKU)); cleanSKU != "" {
			idx.bySavingsSKU[cleanSKU] = master
		}
		sTokens := extractSignificantTokens(s.NameProduct)
		for _, tok := range sTokens {
			idx.tokenIndex[tok] = append(idx.tokenIndex[tok], master)
		}
	}

	return idx
}

// MatchRowInput encapsulates extracted data from an imported spreadsheet row.
type MatchRowInput struct {
	RawName       string
	Barcode       string
	SKU           string
	DosageForm    string
	Concentration string
	Unit          string
	Manufacturer  string
	EnableAI      bool
	EnableSavings bool
	MinSimilarity float64
}

// MatchRowResult contains the complete outcome of multi-stage matching.
type MatchRowResult struct {
	MatchedProductID *int64
	MatchedProduct   *MasterProductData
	ConfidenceScore  float64
	ConfidenceLevel  ConfidenceLevel
	MatchReason      string
	CandidateMatches []CandidateMatch
	Status           string
}

// Match stages an imported row through the deterministic, fuzzy, savings, and attribute validation pipeline.
func (idx *CatalogMatchIndex) Match(
	ctx context.Context,
	input MatchRowInput,
	aiMatcher AIMatcher,
) MatchRowResult {
	if idx == nil || len(idx.allProducts) == 0 {
		return MatchRowResult{
			ConfidenceScore: 0,
			ConfidenceLevel: ConfidenceUnmatched,
			MatchReason:     "الكتالوج العام فارغ",
			Status:          "unmatched",
		}
	}

	rawName := strings.TrimSpace(input.RawName)
	cleanBarcode := strings.TrimSpace(input.Barcode)
	cleanSKU := strings.ToUpper(strings.TrimSpace(input.SKU))
	normName := normalizePharmaceutical(rawName)
	normDosage := normalizePharmaceutical(input.DosageForm)
	normConc := normalizePharmaceutical(input.Concentration)
	normManuf := normalizePharmaceutical(input.Manufacturer)
	rowConcNum, rowConcUnit := extractConcentration(rawName + " " + input.Concentration)
	rowFormKey := extractFormKey(rawName + " " + input.DosageForm)

	// Stage 1: Exact Identifier Match (Barcode / SKU) - Tier 1
	if cleanBarcode != "" {
		if p, ok := idx.byBarcode[cleanBarcode]; ok && p != nil {
			pID := p.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   p,
				ConfidenceScore:  1.0,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      "مطابقة تامة عبر الباركود الدولي (100%)",
				Status:           "matched",
			}
		}
	}
	if cleanSKU != "" {
		if p, ok := idx.bySKU[cleanSKU]; ok && p != nil {
			pID := p.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   p,
				ConfidenceScore:  0.98,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      "مطابقة تامة عبر كود الصنف SKU (98%)",
				Status:           "matched",
			}
		}
	}

	// Stage 2: Exact Normalized Name Match - Tier 2
	if normName != "" {
		if p, ok := idx.byExactName[normName]; ok && p != nil {
			pID := p.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   p,
				ConfidenceScore:  0.96,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      "مطابقة تامة لاسم الصنف بعد المعايرة (96%)",
				Status:           "matched",
			}
		}
	}

	// Stage 3: Savings Products Matching (if enabled) - Tier 3
	if input.EnableSavings && normName != "" {
		if p, ok := idx.bySavingsName[normName]; ok && p != nil {
			pID := p.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   p,
				ConfidenceScore:  0.94,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      "مطابقة فائقة عبر قائمة منتجات التوفير المعتمدة 🛒 (94%)",
				Status:           "matched",
			}
		}
		rowTokens := extractSignificantTokens(normName)
		for sNorm, master := range idx.bySavingsName {
			if strings.Contains(normName, sNorm) || strings.Contains(sNorm, normName) {
				pID := master.ID
				return MatchRowResult{
					MatchedProductID: &pID,
					MatchedProduct:   master,
					ConfidenceScore:  0.92,
					ConfidenceLevel:  ConfidenceHigh,
					MatchReason:      "مطابقة فائقة عبر قائمة منتجات التوفير المعتمدة 🛒 (92%)",
					Status:           "matched",
				}
			}
			tokOverlap := overlapScore(rowTokens, idx.savingsTokens[sNorm])
			if tokOverlap >= 0.60 {
				pID := master.ID
				return MatchRowResult{
					MatchedProductID: &pID,
					MatchedProduct:   master,
					ConfidenceScore:  0.90,
					ConfidenceLevel:  ConfidenceHigh,
					MatchReason:      "مطابقة فائقة عبر قائمة منتجات التوفير المعتمدة 🛒 (90%)",
					Status:           "matched",
				}
			}
		}
	}
	if input.EnableSavings && cleanSKU != "" {
		if p, ok := idx.bySavingsSKU[cleanSKU]; ok && p != nil {
			pID := p.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   p,
				ConfidenceScore:  0.92,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      "مطابقة عبر كود منتجات التوفير 🛒 (92%)",
				Status:           "matched",
			}
		}
	}

	// Stage 4: Multi-Signal Fuzzy Candidate Search & Composite Scoring - Tier 4
	candidates := idx.findCandidates(normName, normDosage, normConc, normManuf, rowConcNum, rowConcUnit, rowFormKey)

	var topCand *candidateScore
	if len(candidates) > 0 {
		topCand = &candidates[0]
	}

	// Format candidate matches for UI review
	var candidateMatchDTOs []CandidateMatch
	for i, c := range candidates {
		if i >= 5 {
			break
		}
		candidateMatchDTOs = append(candidateMatchDTOs, CandidateMatch{
			ProductID:      c.product.ID,
			ProductName:    c.product.NameAR,
			ScientificName: c.product.ScientificName,
			DosageForm:     c.product.DosageForm,
			Concentration:  c.product.Concentration,
			Manufacturer:   c.product.Manufacturer,
			PublicPrice:    c.product.PublicPrice,
			Similarity:     c.score,
			Reason:         c.reason,
		})
	}

	// Stage 5: Optional AI resolution if configured and enabled
	if input.EnableAI && aiMatcher != nil {
		poolCandidates := candidates
		if len(poolCandidates) == 0 && len(idx.allProducts) > 0 {
			maxPool := min(8, len(idx.allProducts))
			for i := 0; i < maxPool; i++ {
				poolCandidates = append(poolCandidates, candidateScore{
					product: idx.allProducts[i],
					score:   0.30,
				})
			}
		}

		if len(poolCandidates) > 0 && (topCand == nil || (topCand.score < 0.90 && topCand.score >= 0.40)) {
			candNames := make([]string, 0, min(8, len(poolCandidates)))
			for i := 0; i < len(poolCandidates) && i < 8; i++ {
				p := poolCandidates[i].product
				label := p.NameAR
				if p.NameEN != "" {
					label += " (" + p.NameEN + ")"
				}
				if p.Concentration != "" || p.DosageForm != "" {
					label += " - " + p.DosageForm + " " + p.Concentration
				}
				candNames = append(candNames, label)
			}
			bestName, aiScore := aiMatcher.MatchCandidate(ctx, rawName, candNames)
			if aiScore >= 0.50 && bestName != "" {
				normBest := normalizePharmaceutical(bestName)
				for _, c := range poolCandidates {
					p := c.product
					if p.NormalizedNameAR == normBest || p.NormalizedNameEN == normBest || strings.Contains(bestName, p.NameAR) || strings.Contains(p.NameAR, bestName) {
						pID := p.ID
						conf := ConfidenceHigh
						if aiScore < 0.85 {
							conf = ConfidenceReview
						}
						return MatchRowResult{
							MatchedProductID: &pID,
							MatchedProduct:   p,
							ConfidenceScore:  aiScore,
							ConfidenceLevel:  conf,
							MatchReason:      fmt.Sprintf("مطابقة ذكية عبر محرك AI (%d%%)", int(aiScore*100)),
							CandidateMatches: candidateMatchDTOs,
							Status:           "matched",
						}
					}
				}
			}
		}
	}

	// Evaluate top candidate from composite scoring
	if topCand != nil {
		if topCand.score >= 0.85 {
			pID := topCand.product.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   topCand.product,
				ConfidenceScore:  topCand.score,
				ConfidenceLevel:  ConfidenceHigh,
				MatchReason:      fmt.Sprintf("مطابقة قوية للاسم والخصائص الدوائية (%d%%)", int(topCand.score*100)),
				CandidateMatches: candidateMatchDTOs,
				Status:           "matched",
			}
		} else if topCand.score >= 0.60 {
			pID := topCand.product.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   topCand.product,
				ConfidenceScore:  topCand.score,
				ConfidenceLevel:  ConfidenceReview,
				MatchReason:      fmt.Sprintf("مطابقة مقترحة (%d%%) — يرجى مراجعة الصنف وتأكيده", int(topCand.score*100)),
				CandidateMatches: candidateMatchDTOs,
				Status:           "matched",
			}
		} else if topCand.score >= 0.45 {
			pID := topCand.product.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   topCand.product,
				ConfidenceScore:  topCand.score,
				ConfidenceLevel:  ConfidenceLow,
				MatchReason:      fmt.Sprintf("مطابقة منخفضة (%d%%) — غير مؤكدة", int(topCand.score*100)),
				CandidateMatches: candidateMatchDTOs,
				Status:           "matched",
			}
		}
	}

	// Stage 6: Unmatched
	return MatchRowResult{
		MatchedProductID: nil,
		ConfidenceScore:  0,
		ConfidenceLevel:  ConfidenceUnmatched,
		MatchReason:      "لم يتم العثور على صنف مطابق بالكتالوج العام",
		CandidateMatches: candidateMatchDTOs,
		Status:           "unmatched",
	}
}

type candidateScore struct {
	product *MasterProductData
	score   float64
	reason  string
}

func (idx *CatalogMatchIndex) findCandidates(
	normName, normDosage, normConc, normManuf string,
	rowConcNum float64, rowConcUnit, rowFormKey string,
) []candidateScore {
	if normName == "" {
		return nil
	}

	candidateSet := make(map[int64]*MasterProductData)

	// 1. Inverted Token Lookup
	tokens := extractSignificantTokens(normName)
	for _, tok := range tokens {
		if prods, ok := idx.tokenIndex[tok]; ok {
			for _, p := range prods {
				candidateSet[p.ID] = p
			}
		}
	}

	// 2. Trigram Lookup (if token match returned few candidates)
	if len(candidateSet) < 15 {
		trigrams := extractTrigrams(normName)
		for _, tri := range trigrams {
			if prods, ok := idx.trigramIndex[tri]; ok {
				for _, p := range prods {
					candidateSet[p.ID] = p
				}
			}
		}
	}

	// 3. Collect candidate pool
	var pool []*MasterProductData
	if len(candidateSet) > 0 {
		for _, p := range candidateSet {
			pool = append(pool, p)
		}
	} else {
		maxPool := min(150, len(idx.allProducts))
		pool = idx.allProducts[:maxPool]
	}

	var scored []candidateScore
	for _, p := range pool {
		simAR := computeStringSimilarity(normName, p.NormalizedNameAR)
		simEN := computeStringSimilarity(normName, p.NormalizedNameEN)
		tokAR := tokenOverlapScore(normName, p.NormalizedNameAR)
		tokEN := tokenOverlapScore(normName, p.NormalizedNameEN)
		baseSim := maxFloat(maxFloat(simAR, simEN), maxFloat(tokAR, tokEN))

		if baseSim < 0.25 {
			continue
		}

		score := baseSim * 0.85

		// Attribute Bonuses & Strict Penalties
		// Concentration Check
		if rowConcNum > 0 && p.ConcentrationNum > 0 {
			if rowConcNum == p.ConcentrationNum && (rowConcUnit == "" || p.ConcentrationUnit == "" || rowConcUnit == p.ConcentrationUnit) {
				score += 0.15 // Exact strength match bonus
			} else {
				score -= 0.35 // Strict conflict penalty (e.g. 500mg vs 1000mg)
			}
		} else if normConc != "" && p.ConcentrationNorm != "" {
			if strings.Contains(p.ConcentrationNorm, normConc) || strings.Contains(normConc, p.ConcentrationNorm) {
				score += 0.10
			}
		}

		// Dosage Form Check
		if rowFormKey != "" && p.FormKey != "" {
			if rowFormKey == p.FormKey {
				score += 0.10 // Matching form (e.g. tablet == tablet)
			} else {
				score -= 0.25 // Form conflict penalty (e.g. syrup vs tablet)
			}
		} else if normDosage != "" && p.DosageFormNorm != "" {
			if strings.Contains(p.DosageFormNorm, normDosage) || strings.Contains(normDosage, p.DosageFormNorm) {
				score += 0.08
			}
		}

		// Manufacturer / Brand Check
		if normManuf != "" && p.ManufacturerNorm != "" {
			if strings.Contains(p.ManufacturerNorm, normManuf) || strings.Contains(normManuf, p.ManufacturerNorm) {
				score += 0.10
			}
		}

		finalScore := maxFloat(0.0, minFloat(0.99, score))
		if finalScore >= 0.35 {
			reason := fmt.Sprintf("تشابه اسم %d%%", int(baseSim*100))
			if rowConcNum > 0 && p.ConcentrationNum > 0 && rowConcNum == p.ConcentrationNum {
				reason += " + تطابق التركيز"
			}
			if rowFormKey != "" && p.FormKey != "" && rowFormKey == p.FormKey {
				reason += " + تطابق الشكل"
			}
			scored = append(scored, candidateScore{
				product: p,
				score:   finalScore,
				reason:  reason,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

func computeStringSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1.0
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		ratio := float64(min(len(a), len(b))) / float64(max(len(a), len(b)))
		return 0.75 + 0.25*ratio
	}

	triA := extractTrigrams(a)
	triB := extractTrigrams(b)
	if len(triA) == 0 || len(triB) == 0 {
		return 0
	}
	return overlapScore(triA, triB)
}

func tokenOverlapScore(a, b string) float64 {
	return overlapScore(extractSignificantTokens(a), extractSignificantTokens(b))
}

func overlapScore(toksA, toksB []string) float64 {
	if len(toksA) == 0 || len(toksB) == 0 {
		return 0
	}
	setB := make(map[string]bool, len(toksB))
	for _, t := range toksB {
		setB[t] = true
	}
	matches := 0
	for _, t := range toksA {
		if setB[t] {
			matches++
		}
	}
	maxLen := max(len(toksA), len(toksB))
	if maxLen == 0 {
		return 0
	}
	return float64(matches) / float64(maxLen)
}

func extractTrigrams(s string) []string {
	runes := []rune(s)
	if len(runes) < 3 {
		if len(runes) > 0 {
			return []string{string(runes)}
		}
		return nil
	}
	trigrams := make([]string, 0, len(runes)-2)
	for i := 0; i <= len(runes)-3; i++ {
		trigrams = append(trigrams, string(runes[i:i+3]))
	}
	return trigrams
}

var arabicLetterFolds = map[rune]rune{
	'\u0623': '\u0627', // أ -> ا
	'\u0625': '\u0627', // إ -> ا
	'\u0622': '\u0627', // آ -> ا
	'\u0671': '\u0627', // ٱ -> ا
	'\u0649': '\u064A', // ى -> ي
	'\u0626': '\u064A', // ئ -> ي
	'\u0624': '\u0648', // ؤ -> و
	'\u0629': '\u0647', // ة -> ه
	'\u06A4': '\u0641', // ڤ -> ف
	'\u0686': '\u062C', // چ -> ج
	'\u06AF': '\u062C', // گ -> ج
	'\u067E': '\u0628', // پ -> ب
}

func normalizePharmaceutical(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		// Convert Arabic-Indic digits
		if r >= '\u0660' && r <= '\u0669' {
			b.WriteRune('0' + (r - '\u0660'))
			continue
		}
		if r >= '\u06F0' && r <= '\u06F9' {
			b.WriteRune('0' + (r - '\u06F0'))
			continue
		}
		// Strip harakat & tatweel
		if (r >= '\u064B' && r <= '\u0652') || r == '\u0640' || r == '\u0670' {
			continue
		}
		// Zero-width & bidi chars
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\u200E' || r == '\u200F' || r == '\uFEFF' {
			continue
		}
		// Apply letter folding
		if folded, ok := arabicLetterFolds[r]; ok {
			b.WriteRune(folded)
			continue
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}

	clean := b.String()
	words := strings.Fields(clean)

	noiseWords := map[string]bool{
		"tab": true, "tabs": true, "tablet": true, "tablets": true,
		"cap": true, "caps": true, "capsule": true, "capsules": true,
		"amp": true, "amps": true, "ampoule": true, "ampoules": true,
		"vial": true, "vials": true, "syr": true, "syrup": true,
		"susp": true, "suspension": true, "drops": true, "drop": true,
		"cream": true, "crm": true, "ointment": true, "oint": true,
		"gel": true, "spray": true, "solution": true, "sol": true,
		"eff": true, "sachet": true, "sachets": true, "supp": true,
		"اقراص": true, "قرص": true, "كبسول": true, "كبسولات": true,
		"شراب": true, "نقط": true, "امبول": true, "امبولات": true,
		"فيال": true, "مرهم": true, "كريم": true, "بخاخ": true,
		"محلول": true, "فوار": true, "لبوس": true, "تحاميل": true, "اكياس": true,
		"توفير": true, "عرض": true, "خصم": true, "مجانا": true, "جديد": true,
		"savings": true, "offer": true, "discount": true,
	}

	var filtered []string
	for _, w := range words {
		if !noiseWords[w] {
			filtered = append(filtered, w)
		}
	}

	if len(filtered) > 0 {
		return strings.Join(filtered, " ")
	}
	return strings.Join(words, " ")
}

func extractSignificantTokens(text string) []string {
	norm := normalizePharmaceutical(text)
	words := strings.Fields(norm)
	var tokens []string
	for _, w := range words {
		r := []rune(w)
		if len(r) >= 3 && !isPureNumber(w) {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func extractConcentration(text string) (float64, string) {
	clean := strings.ToLower(text)
	clean = strings.ReplaceAll(clean, "مجم", "mg")
	clean = strings.ReplaceAll(clean, "مكجم", "mcg")
	clean = strings.ReplaceAll(clean, "جم", "g")
	clean = strings.ReplaceAll(clean, "جرام", "g")
	words := strings.Fields(clean)
	units := []string{"mg", "gm", "g", "mcg", "iu", "ml", "%"}

	for _, w := range words {
		for _, u := range units {
			if strings.HasSuffix(w, u) {
				numStr := strings.TrimSuffix(w, u)
				if val, err := strconv.ParseFloat(numStr, 64); err == nil && val > 0 {
					if u == "g" || u == "gm" {
						return val * 1000, "mg"
					}
					return val, u
				}
			}
		}
		if val, err := strconv.ParseFloat(w, 64); err == nil && val >= 10 && val <= 5000 {
			return val, "mg"
		}
	}
	return 0, ""
}

func extractFormKey(text string) string {
	clean := strings.ToLower(text)
	switch {
	case strings.Contains(clean, "tab") || strings.Contains(clean, "قرص") || strings.Contains(clean, "اقراص"):
		return "tablet"
	case strings.Contains(clean, "cap") || strings.Contains(clean, "كبسول"):
		return "capsule"
	case strings.Contains(clean, "syr") || strings.Contains(clean, "susp") || strings.Contains(clean, "شراب") || strings.Contains(clean, "معلق"):
		return "liquid"
	case strings.Contains(clean, "amp") || strings.Contains(clean, "vial") || strings.Contains(clean, "امبول") || strings.Contains(clean, "حقن"):
		return "injectable"
	case strings.Contains(clean, "cream") || strings.Contains(clean, "oint") || strings.Contains(clean, "gel") || strings.Contains(clean, "مرهم") || strings.Contains(clean, "كريم") || strings.Contains(clean, "جل"):
		return "topical"
	case strings.Contains(clean, "drop") || strings.Contains(clean, "نقط") || strings.Contains(clean, "قطرة"):
		return "drops"
	case strings.Contains(clean, "supp") || strings.Contains(clean, "لبوس") || strings.Contains(clean, "تحاميل"):
		return "suppository"
	case strings.Contains(clean, "spray") || strings.Contains(clean, "بخاخ"):
		return "spray"
	case strings.Contains(clean, "sachet") || strings.Contains(clean, "eff") || strings.Contains(clean, "فوار") || strings.Contains(clean, "كيس") || strings.Contains(clean, "اكياس"):
		return "sachet"
	}
	return ""
}

func isPureNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
