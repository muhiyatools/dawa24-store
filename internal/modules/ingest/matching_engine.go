package ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
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
	byBarcode      map[string]*MasterProductData
	bySKU          map[string]*MasterProductData
	byExactName    map[string]*MasterProductData
	bySavingsName  map[string]*MasterProductData
	bySavingsSKU   map[string]*MasterProductData
	savingsTokens  map[string][]string // normalized savings name -> significant tokens
	tokenIndex     map[string][]*MasterProductData
	allProducts    []*MasterProductData
	productsByID   map[int64]*MasterProductData
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
	RawName        string
	Barcode        string
	SKU            string
	DosageForm     string
	Concentration  string
	Unit           string
	Manufacturer   string
	EnableAI       bool
	EnableSavings  bool
	MinSimilarity  float64
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

// Match stages an imported row through the deterministic, fuzzy, savings, and AI pipeline.
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

	// Stage 1: Exact Identifier Match (Barcode / SKU)
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

	// Stage 2: Exact Normalized Name Match
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

	// Stage 3: Savings Products Matching (if enabled)
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
	// Fuzzy sweep over the savings index. The expensive similarity scores are
	// guarded by cheap pre-filters with identical outcomes: containment is
	// checked directly, the token overlap uses precomputed token lists, and
	// the edit-distance branch of arabic.Similarity can never reach 0.65 when
	// the shorter string holds fewer than 65% of the longer one's runes.
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
		if tokOverlap >= 0.50 {
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
		if runeLengthRatio(normName, sNorm) < 0.65 {
			continue
		}
		if arabic.Similarity(normName, sNorm) >= 0.65 {
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

	// Stage 4: Multi-Signal Fuzzy Candidate Search & Scoring
	candidates := idx.findCandidates(normName, normDosage, normConc, normManuf)

	var topCand *candidateScore
	if len(candidates) > 0 {
		topCand = &candidates[0]
	}

	// Format candidate matches for UI
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

	// Stage 5: AI-Assisted Resolution (if enabled and ambiguous or score < 0.85)
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

		if len(poolCandidates) > 0 {
			shouldCallAI := false
			if topCand == nil || topCand.score < 0.95 {
				shouldCallAI = true
			} else if len(poolCandidates) >= 2 && (poolCandidates[0].score-poolCandidates[1].score) < 0.08 {
				shouldCallAI = true
			}

			if shouldCallAI {
				candNames := make([]string, 0, min(10, len(poolCandidates)))
				for i := 0; i < len(poolCandidates) && i < 10; i++ {
					p := poolCandidates[i].product
					candLabel := p.NameAR
					if p.NameEN != "" {
						candLabel += " (" + p.NameEN + ")"
					}
					if p.ScientificName != "" {
						candLabel += " [" + p.ScientificName + "]"
					}
					if p.DosageForm != "" || p.Concentration != "" {
						candLabel += " - " + p.DosageForm + " " + p.Concentration
					}
					candNames = append(candNames, candLabel)
				}
				bestName, aiScore := aiMatcher.MatchCandidate(ctx, rawName, candNames)
				if aiScore >= 0.50 && bestName != "" {
					normBest := normalizePharmaceutical(bestName)
					for _, c := range poolCandidates {
						p := c.product
						matched := false
						if p.NameAR == bestName || p.NameEN == bestName ||
							normalizePharmaceutical(p.NameAR) == normBest ||
							normalizePharmaceutical(p.NameEN) == normBest ||
							strings.Contains(bestName, p.NameAR) ||
							strings.Contains(p.NameAR, bestName) {
							matched = true
						} else if p.NameEN != "" && (strings.Contains(strings.ToLower(bestName), strings.ToLower(p.NameEN)) || strings.Contains(strings.ToLower(p.NameEN), strings.ToLower(bestName))) {
							matched = true
						} else if p.ScientificName != "" && strings.Contains(strings.ToLower(bestName), strings.ToLower(p.ScientificName)) {
							matched = true
						} else if tokenOverlapScore(bestName, p.NameAR) >= 0.40 || (p.NameEN != "" && tokenOverlapScore(bestName, p.NameEN) >= 0.40) {
							matched = true
						}

						if matched {
							pID := p.ID
							confLvl := ConfidenceHigh
							if aiScore < 0.85 {
								confLvl = ConfidenceReview
							}
							return MatchRowResult{
								MatchedProductID: &pID,
								MatchedProduct:   p,
								ConfidenceScore:  aiScore,
								ConfidenceLevel:  confLvl,
								MatchReason:      fmt.Sprintf("مطابقة ذكية عبر محرك الذكاء الاصطناعي AI (%d%%)", int(aiScore*100)),
								CandidateMatches: candidateMatchDTOs,
								Status:           "matched",
							}
						}
					}
				}
			}
		}
	}

	// Evaluate top candidate from deterministic fuzzy scoring
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
		} else if topCand.score >= 0.65 {
			pID := topCand.product.ID
			return MatchRowResult{
				MatchedProductID: &pID,
				MatchedProduct:   topCand.product,
				ConfidenceScore:  topCand.score,
				ConfidenceLevel:  ConfidenceReview,
				MatchReason:      fmt.Sprintf("مطابقة متوسطة (%d%%) — يرجى مراجعة الصنف وتأكيده", int(topCand.score*100)),
				CandidateMatches: candidateMatchDTOs,
				Status:           "matched",
			}
		} else if topCand.score >= 0.50 {
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
) []candidateScore {
	if normName == "" {
		return nil
	}

	tokens := extractSignificantTokens(normName)
	candidateSet := make(map[int64]*MasterProductData)

	for _, tok := range tokens {
		if prods, ok := idx.tokenIndex[tok]; ok {
			for _, p := range prods {
				candidateSet[p.ID] = p
			}
		}
	}

	// If token index returned too few, fallback to top products
	var pool []*MasterProductData
	if len(candidateSet) > 0 {
		for _, p := range candidateSet {
			pool = append(pool, p)
		}
	} else {
		// Sample first 100 products for general fuzzy comparison
		maxPool := min(100, len(idx.allProducts))
		pool = idx.allProducts[:maxPool]
	}

	var scored []candidateScore
	for _, p := range pool {
		simAR := arabic.Similarity(normName, p.NormalizedNameAR)
		simEN := arabic.Similarity(normName, p.NormalizedNameEN)
		tokAR := tokenOverlapScore(normName, p.NormalizedNameAR)
		tokEN := tokenOverlapScore(normName, p.NormalizedNameEN)
		baseSim := maxFloat(maxFloat(simAR, simEN), maxFloat(tokAR, tokEN))

		bonus := 0.0
		if normConc != "" && p.ConcentrationNorm != "" && strings.Contains(p.ConcentrationNorm, normConc) {
			bonus += 0.08
		}
		if normDosage != "" && p.DosageFormNorm != "" && (p.DosageFormNorm == normDosage || strings.Contains(p.DosageFormNorm, normDosage)) {
			bonus += 0.05
		}
		if normManuf != "" && p.ManufacturerNorm != "" && (p.ManufacturerNorm == normManuf || strings.Contains(p.ManufacturerNorm, normManuf)) {
			bonus += 0.05
		}

		finalScore := minFloat(0.95, baseSim*0.82+bonus)
		if finalScore >= 0.35 {
			scored = append(scored, candidateScore{
				product: p,
				score:   finalScore,
				reason:  fmt.Sprintf("تشابه اسم %d%%", int(baseSim*100)),
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

func tokenOverlapScore(a, b string) float64 {
	return overlapScore(extractSignificantTokens(a), extractSignificantTokens(b))
}

// overlapScore is the shared body of tokenOverlapScore: matched tokens over
// the larger token count, 0 when either side has no significant tokens.
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

// runeLengthRatio reports shorter/longer rune lengths of the raw strings.
// For non-containment pairs, arabic.Similarity's edit-distance branch scores
// at most this ratio, so anything below the match threshold lets a caller
// skip the O(n·m) levenshtein entirely.
func runeLengthRatio(a, b string) float64 {
	ra, rb := len([]rune(a)), len([]rune(b))
	if ra == 0 || rb == 0 {
		return 0
	}
	shorter, longer := ra, rb
	if shorter > longer {
		shorter, longer = longer, shorter
	}
	return float64(shorter) / float64(longer)
}

// normalizePharmaceutical strips common pharmaceutical punctuation, suffixes, and noise.
func normalizePharmaceutical(s string) string {
	clean := arabic.Normalize(s)
	clean = strings.ToLower(clean)

	// Remove common noise words in catalogue exports
	noiseWords := []string{
		"tab", "tabs", "tablet", "tablets", "cap", "caps", "capsule", "capsules",
		"amp", "amps", "ampoule", "ampoules", "vial", "vials", "syr", "syrup",
		"susp", "suspension", "drops", "drop", "cream", "crm", "ointment", "oint",
		"gel", "spray", "solution", "sol", "eff", "sachet", "sachets", "supp",
		"أقراص", "قرص", "كبسول", "كبسولات", "شراب", "نقط", "أمبول", "أمبولات",
		"فيال", "مرهم", "كريم", "بخاخ", "محلول", "فوار", "لبوس", "تحاميل", "أكياس",
		"توفير", "عرض", "خصم", "مجانا", "جديد", "savings", "offer", "discount",
	}

	words := strings.Fields(clean)
	var filtered []string
	for _, w := range words {
		isNoise := false
		for _, nw := range noiseWords {
			if w == nw {
				isNoise = true
				break
			}
		}
		if !isNoise {
			filtered = append(filtered, w)
		}
	}

	if len(filtered) > 0 {
		return strings.Join(filtered, " ")
	}
	return clean
}

func extractSignificantTokens(text string) []string {
	clean := arabic.Normalize(text)
	words := strings.Fields(clean)
	var tokens []string
	for _, w := range words {
		r := []rune(w)
		if len(r) >= 3 && !isPureNumber(w) {
			tokens = append(tokens, w)
		}
	}
	return tokens
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
