package ui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// MatchStrategy determines how rows in saving product imports are linked to the master catalog.
type MatchStrategy string

const (
	StrategySmartAuto MatchStrategy = "smart_auto" // SKU -> Exact Name -> Normalized Name -> Core Name -> Fuzzy Similarity (Default & Recommended)
	StrategySKUOnly   MatchStrategy = "sku_only"   // Exact & Cleaned SKU / Barcode only
	StrategyNameOnly  MatchStrategy = "name_only"  // Exact, Normalized, Core, & Fuzzy Name only
	StrategyIDOnly    MatchStrategy = "id_only"    // Direct ProductID only
)

// MatchResult captures the resolution of a single saving product row against the catalog.
type MatchResult struct {
	ProductID  *int64  `json:"product_id,omitempty"`
	MatchType  string  `json:"match_type"`
	Confidence float64 `json:"confidence"`
}

// CatalogItemIndex is an in-memory representation of a catalog product for ultra-fast matching.
type CatalogItemIndex struct {
	ID      int64
	SKU     string
	Barcode string
	NameAr  string
	NameEn  string
	NormAr  string
	NormEn  string
	CoreAr  string
	CoreEn  string
}

// SavingProductMatchEngine handles multi-tier resilient matching against catalog items.
type SavingProductMatchEngine struct {
	items        []*CatalogItemIndex
	idMap        map[int64]bool
	skuExactMap  map[string]int64
	skuCleanMap  map[string]int64
	nameExactMap map[string]int64
	nameNormMap  map[string]int64
	nameCoreMap  map[string]int64
}

// NewSavingProductMatchEngine indexes catalog items for instant matching.
func NewSavingProductMatchEngine(sources []*catalog.CatalogMatchSource) *SavingProductMatchEngine {
	engine := &SavingProductMatchEngine{
		items:        make([]*CatalogItemIndex, 0, len(sources)),
		idMap:        make(map[int64]bool, len(sources)),
		skuExactMap:  make(map[string]int64, len(sources)*2),
		skuCleanMap:  make(map[string]int64, len(sources)*2),
		nameExactMap: make(map[string]int64, len(sources)*2),
		nameNormMap:  make(map[string]int64, len(sources)*2),
		nameCoreMap:  make(map[string]int64, len(sources)*2),
	}

	for _, s := range sources {
		if s == nil || s.ID <= 0 {
			continue
		}
		engine.idMap[s.ID] = true

		normAr := arabic.Normalize(s.NameAr)
		normEn := normalizeLatin(s.NameEn)
		coreAr := extractCoreDrugName(normAr)
		coreEn := extractCoreDrugName(normEn)

		idxItem := &CatalogItemIndex{
			ID:      s.ID,
			SKU:     s.SKU,
			Barcode: s.Barcode,
			NameAr:  s.NameAr,
			NameEn:  s.NameEn,
			NormAr:  normAr,
			NormEn:  normEn,
			CoreAr:  coreAr,
			CoreEn:  coreEn,
		}
		engine.items = append(engine.items, idxItem)

		// 1. SKU / Barcode maps
		if s.SKU != "" {
			exactSKU := strings.ToLower(strings.TrimSpace(s.SKU))
			engine.skuExactMap[exactSKU] = s.ID
			cleanSKU := SanitizeSKUCode(exactSKU)
			if cleanSKU != "" {
				engine.skuCleanMap[cleanSKU] = s.ID
			}
		}
		if s.Barcode != "" {
			exactBarcode := strings.ToLower(strings.TrimSpace(s.Barcode))
			engine.skuExactMap[exactBarcode] = s.ID
			cleanBarcode := SanitizeSKUCode(exactBarcode)
			if cleanBarcode != "" {
				engine.skuCleanMap[cleanBarcode] = s.ID
			}
		}

		// 2. Name exact maps
		if s.NameAr != "" {
			engine.nameExactMap[strings.ToLower(strings.TrimSpace(s.NameAr))] = s.ID
		}
		if s.NameEn != "" {
			engine.nameExactMap[strings.ToLower(strings.TrimSpace(s.NameEn))] = s.ID
		}

		// 3. Name normalized maps
		if normAr != "" {
			engine.nameNormMap[normAr] = s.ID
		}
		if normEn != "" {
			engine.nameNormMap[normEn] = s.ID
		}

		// 4. Name core maps
		if coreAr != "" && len(coreAr) >= 3 {
			engine.nameCoreMap[coreAr] = s.ID
		}
		if coreEn != "" && len(coreEn) >= 3 {
			engine.nameCoreMap[coreEn] = s.ID
		}
	}

	return engine
}

// Match executes the matching pipeline for an item according to the strategy.
func (e *SavingProductMatchEngine) Match(strategy MatchStrategy, productID *int64, rawSKU, rawName string) MatchResult {
	if e == nil || len(e.items) == 0 {
		return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
	}

	if strategy == "" {
		strategy = StrategySmartAuto
	}

	// 1. Direct Product ID
	if productID != nil && *productID > 0 {
		if e.idMap[*productID] {
			return MatchResult{ProductID: productID, MatchType: "direct_id", Confidence: 1.0}
		}
		if strategy == StrategyIDOnly {
			return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
		}
	}
	if strategy == StrategyIDOnly {
		return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
	}

	cleanSKU := SanitizeSKUCode(rawSKU)
	exactSKU := strings.ToLower(strings.TrimSpace(rawSKU))

	// 2. SKU matching if allowed
	if strategy == StrategySmartAuto || strategy == StrategySKUOnly {
		if exactSKU != "" {
			if id, ok := e.skuExactMap[exactSKU]; ok {
				matchedID := id
				return MatchResult{ProductID: &matchedID, MatchType: "exact_sku", Confidence: 1.0}
			}
		}
		if cleanSKU != "" {
			if id, ok := e.skuCleanMap[cleanSKU]; ok {
				matchedID := id
				return MatchResult{ProductID: &matchedID, MatchType: "clean_sku", Confidence: 0.98}
			}
		}
		if strategy == StrategySKUOnly {
			return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
		}
	}

	// 3. Name matching if allowed
	if strategy == StrategySmartAuto || strategy == StrategyNameOnly {
		cleanName := strings.TrimSpace(rawName)
		if cleanName == "" {
			return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
		}

		lowerName := strings.ToLower(cleanName)
		if id, ok := e.nameExactMap[lowerName]; ok {
			matchedID := id
			return MatchResult{ProductID: &matchedID, MatchType: "exact_name", Confidence: 1.0}
		}

		normName := arabic.Normalize(cleanName)
		if id, ok := e.nameNormMap[normName]; ok {
			matchedID := id
			return MatchResult{ProductID: &matchedID, MatchType: "norm_name", Confidence: 0.95}
		}

		normLatin := normalizeLatin(cleanName)
		if id, ok := e.nameNormMap[normLatin]; ok {
			matchedID := id
			return MatchResult{ProductID: &matchedID, MatchType: "norm_name_latin", Confidence: 0.95}
		}

		coreName := extractCoreDrugName(normName)
		if coreName != "" && len(coreName) >= 4 {
			if id, ok := e.nameCoreMap[coreName]; ok {
				matchedID := id
				return MatchResult{ProductID: &matchedID, MatchType: "core_name", Confidence: 0.90}
			}
		}

		// 4. Resilient Fuzzy Scoring (with token overlap, brand prefix, & Levenshtein similarity)
		var bestID int64
		var bestScore float64

		targetNorm := normName
		if targetNorm == "" {
			targetNorm = normLatin
		}

		for _, item := range e.items {
			var score float64
			if item.NormAr != "" {
				score = calculatePharmaceuticalSimilarity(targetNorm, item.NormAr)
			}
			if item.NormEn != "" {
				enScore := calculatePharmaceuticalSimilarity(targetNorm, item.NormEn)
				if enScore > score {
					score = enScore
				}
			}

			if score > bestScore {
				bestScore = score
				bestID = item.ID
			}
		}

		if bestScore >= 0.75 && bestID > 0 {
			matchedID := bestID
			return MatchResult{ProductID: &matchedID, MatchType: "fuzzy_name", Confidence: bestScore}
		}
	}

	return MatchResult{ProductID: nil, MatchType: "unlinked", Confidence: 0.0}
}

// calculatePharmaceuticalSimilarity combines Levenshtein/containment similarity with token Jaccard overlap and prefix matching.
func calculatePharmaceuticalSimilarity(a, b string) float64 {
	levScore := arabic.Similarity(a, b)

	tokensA := strings.Fields(a)
	tokensB := strings.Fields(b)
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return levScore
	}

	mapB := make(map[string]bool, len(tokensB))
	for _, tb := range tokensB {
		mapB[tb] = true
	}

	overlapCount := 0
	for _, ta := range tokensA {
		if mapB[ta] {
			overlapCount++
		}
	}

	shorterLen := len(tokensA)
	if len(tokensB) < shorterLen {
		shorterLen = len(tokensB)
	}
	longerLen := len(tokensA)
	if len(tokensB) > longerLen {
		longerLen = len(tokensB)
	}

	overlapRatio := float64(overlapCount) / float64(longerLen)
	shorterCoverage := float64(overlapCount) / float64(shorterLen)

	// Bonus if the first word / brand token matches
	prefixBonus := 0.0
	if len(tokensA) > 0 && len(tokensB) > 0 && tokensA[0] == tokensB[0] {
		prefixBonus = 0.10
		if len(tokensA) > 1 && len(tokensB) > 1 && tokensA[1] == tokensB[1] {
			prefixBonus = 0.18
		}
	}

	combinedTokenScore := (overlapRatio*0.60 + shorterCoverage*0.40) + prefixBonus
	if combinedTokenScore > 1.0 {
		combinedTokenScore = 1.0
	}

	if combinedTokenScore > levScore {
		return combinedTokenScore
	}
	return levScore
}

var scientificNotationRegex = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?e\+[0-9]+$`)

// NormalizeDigitsOnly converts Arabic-Indic digits ٠-٩ to 0-9 without altering periods, commas, or letters.
func NormalizeDigitsOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '٠' && r <= '٩' {
			b.WriteRune('0' + (r - '٠'))
		} else if r >= '۰' && r <= '۹' {
			b.WriteRune('0' + (r - '۰'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeSKUCode cleans common spreadsheet artifacts like .0, scientific notation, leading/trailing noise.
func SanitizeSKUCode(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// 1. Normalize Arabic digits to ASCII
	s = NormalizeDigitsOnly(s)

	// 2. If scientific notation: 6.22123E+12
	if scientificNotationRegex.MatchString(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			s = fmt.Sprintf("%.0f", f)
		}
	}

	// 3. If float formatted integer: 20203380.0
	if strings.HasSuffix(s, ".0") {
		s = strings.TrimSuffix(s, ".0")
	}

	// 4. Strip noise dashes / spaces / slashes
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "/", "")

	// 5. Strip leading zeros if numeric and length > 6
	if isNumericOnly(s) {
		trimmed := strings.TrimLeft(s, "0")
		if trimmed != "" {
			return trimmed
		}
	}

	return s
}

func isNumericOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func normalizeLatin(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "/", " ")
	s = strings.ReplaceAll(s, "+", " ")
	return strings.Join(strings.Fields(s), " ")
}

var dosageSuffixes = []string{
	" اقراص", " أقراص", " قرص", " كبسول", " كبسولات", " شراب", " معلق", " فوار",
	" نقط", " مرهم", " جل", " كريم", " امبول", " فيال", " ساشيت", " لبوس", " بخاخ", " غسول",
	" مجم", " ملغ", " جم", " جرام", " مل", " ملى", " لتر",
	" tablets", " tablet", " tabs", " tab", " capsules", " capsule", " caps", " cap",
	" syrup", " syr", " suspension", " susp", " drops", " ointment", " oint",
	" cream", " crm", " gel", " ampoule", " amp", " vial", " sachet", " suppository", " supp",
	" spray", " lotion", " wash", " mg", " gm", " g", " ml", " l",
}

// extractCoreDrugName strips dosage form noise to allow matching root pharmaceutical trade names.
func extractCoreDrugName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, suffix := range dosageSuffixes {
		s = strings.ReplaceAll(s, suffix, "")
	}
	return strings.Join(strings.Fields(s), " ")
}

// ParseFlexibleQuantity parses various spreadsheet quantity formats (scientific, float, comma formatted).
func ParseFlexibleQuantity(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return f, true
}

// ParseFlexibleMoney parses price strings containing currency codes, commas, or extra formatting.
func ParseFlexibleMoney(raw string) (money.Amount, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return money.Amount{}, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "ج.م", "")
	s = strings.ReplaceAll(s, "جم", "")
	s = strings.ReplaceAll(s, "جنيه", "")
	s = strings.ReplaceAll(s, "egp", "")
	s = strings.ReplaceAll(s, "EGP", "")
	s = strings.ReplaceAll(s, "le", "")
	s = strings.ReplaceAll(s, "LE", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.TrimSpace(s)

	amt, err := money.Parse(s)
	if err != nil {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return money.Amount{}, false
		}
		return money.FromMinor(int64(math.Round(f * 100))), true
	}
	if amt.IsNegative() {
		return money.Amount{}, false
	}
	return amt, true
}

// IsSummaryOrTotalRow checks if a row is an Excel summary / subtotal row to skip.
func IsSummaryOrTotalRow(row []string) bool {
	if len(row) == 0 {
		return true
	}
	for _, cell := range row {
		clean := strings.TrimSpace(strings.ToLower(cell))
		if clean == "" {
			continue
		}
		norm := arabic.Normalize(clean)
		summaryWords := []string{
			"اجمالي", "الاجمالي", "مجموع", "المجموع", "المجموع الكلي", "العدد",
			"total", "grand total", "subtotal", "sum", "summary", "count",
		}
		for _, w := range summaryWords {
			if norm == w || strings.HasPrefix(norm, w+" ") || strings.HasPrefix(norm, w+":") {
				return true
			}
		}
		break
	}
	return false
}

// IsAllEmptyRow checks if all cells in a row are whitespace or empty.
func IsAllEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
