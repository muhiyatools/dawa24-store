package compare

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CandidateProduct represents a catalog product candidate evaluated during matching.
type CandidateProduct struct {
	ID                     int64        `json:"id"`
	SKU                    string       `json:"sku"`
	NameAr                 string       `json:"name_ar"`
	NameEn                 string       `json:"name_en"`
	ScientificName         string       `json:"scientific_name"`
	ManufacturingCompanies string       `json:"manufacturing_companies"`
	Pharmacology           string       `json:"pharmacology"`
	Price                  money.Amount `json:"price"`
	Image                  string       `json:"image"`
	SearchSimple           string       `json:"search_simple"`
}

// MatchResult encapsulates the resolved product match and confidence score.
type MatchResult struct {
	ProductID   *int64      `json:"product_id"`
	Confidence  float64     `json:"confidence"` // 0..100 scale (matching Laravel parity)
	Method      MatchMethod `json:"method"`
	MethodLabel string      `json:"method_label"`
}

// AIMatcher provides AI-augmented matching for low-confidence rows via the platform gateway (Rule R2, Rule R3).
type AIMatcher interface {
	MatchCandidate(ctx context.Context, query string, candidateNames []string) (string, float64)
}

// MatchLadder resolves a single uploaded row against the catalog using the deterministic match ladder (Plan V5 Phase 2 §2.4).
// Strategy 0: Saved Customer Product Mapping (100%)
// Strategy 1: SKU / Barcode Match (100%)
// Strategy 2: Exact Name Match (100%)
// Strategy 3: Trigram / Candidate Fuzzy Match (>= 90% or >= 60%)
// Strategy 4: First Meaningful Word / Token-subset Match (>= 55%)
// Strategy 5: Unmatched (< 55%)
func (s *Service) MatchLadder(ctx context.Context, orgID *int64, rawName string, sku string, barcode string, candidates []*CandidateProduct) (*MatchResult, error) {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		return &MatchResult{
			ProductID:   nil,
			Confidence:  0,
			Method:      MatchMethodUnmatched,
			MethodLabel: "غير مطابق",
		}, nil
	}

	normName := normalizeProductText(rawName)
	cleanSKU := strings.ToLower(strings.TrimSpace(sku))
	cleanBarcode := strings.ToLower(strings.TrimSpace(barcode))

	// -----------------------------------------------------------------------
	// Strategy 0: Saved Customer Product Mappings (catalog.customer_product_mappings)
	// -----------------------------------------------------------------------
	if s.repo != nil {
		if mappedID, err := s.repo.GetSavedProductMapping(ctx, orgID, rawName); err == nil && mappedID != nil {
			return &MatchResult{
				ProductID:   mappedID,
				Confidence:  100.0,
				Method:      MatchMethodSavedMapping,
				MethodLabel: "تطابق محفوظ مسبقاً ⚡",
			}, nil
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 1: SKU / Barcode Match
	// -----------------------------------------------------------------------
	if cleanSKU != "" && cleanSKU != "0" {
		for _, c := range candidates {
			if strings.ToLower(strings.TrimSpace(c.SKU)) == cleanSKU {
				return &MatchResult{
					ProductID:   &c.ID,
					Confidence:  100.0,
					Method:      MatchMethodSKU,
					MethodLabel: "تطابق بالـ SKU",
				}, nil
			}
		}
	}
	if cleanBarcode != "" && cleanBarcode != "0" {
		for _, c := range candidates {
			if strings.ToLower(strings.TrimSpace(c.SKU)) == cleanBarcode {
				return &MatchResult{
					ProductID:   &c.ID,
					Confidence:  100.0,
					Method:      MatchMethodBarcode,
					MethodLabel: "تطابق بالباركود",
				}, nil
			}
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 2: Exact Name Match
	// -----------------------------------------------------------------------
	for _, c := range candidates {
		cNormAr := normalizeProductText(c.NameAr)
		cNormEn := normalizeProductText(c.NameEn)
		if (cNormAr != "" && cNormAr == normName) || (cNormEn != "" && cNormEn == normName) {
			return &MatchResult{
				ProductID:   &c.ID,
				Confidence:  100.0,
				Method:      MatchMethodExactName,
				MethodLabel: "تطابق تام بالاسم",
			}, nil
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 3: Trigram / Candidate Fuzzy Match
	// -----------------------------------------------------------------------
	var bestCandidate *CandidateProduct
	bestScore := 0.0

	for _, c := range candidates {
		scoreAr := CalculateTextSimilarity(normName, normalizeProductText(c.NameAr))
		scoreEn := CalculateTextSimilarity(normName, normalizeProductText(c.NameEn))
		score := math.Max(scoreAr, scoreEn)

		if c.ScientificName != "" {
			scoreSci := CalculateTextSimilarity(normName, normalizeProductText(c.ScientificName))
			score = math.Max(score, scoreSci)
		}
		if c.ManufacturingCompanies != "" {
			scoreMfg := CalculateTextSimilarity(normName, normalizeProductText(c.ManufacturingCompanies))
			score = math.Max(score, scoreMfg)
		}
		if c.Pharmacology != "" {
			scorePh := CalculateTextSimilarity(normName, normalizeProductText(c.Pharmacology))
			score = math.Max(score, scorePh)
		}

		if score > bestScore {
			bestScore = score
			bestCandidate = c
		}
	}

	if bestCandidate != nil && bestScore >= 0.60 {
		confidence := math.Round(bestScore * 100.0)
		if bestScore >= 0.90 {
			return &MatchResult{
				ProductID:   &bestCandidate.ID,
				Confidence:  confidence,
				Method:      MatchMethodFuzzy,
				MethodLabel: fmt.Sprintf("تطابق ذكي (%d%%)", int(confidence)),
			}, nil
		}
		return &MatchResult{
			ProductID:   &bestCandidate.ID,
			Confidence:  confidence,
			Method:      MatchMethodPartial,
			MethodLabel: fmt.Sprintf("تطابق جزئي (%d%%)", int(confidence)),
		}, nil
	}

	// -----------------------------------------------------------------------
	// Strategy 4: Token-subset / First Meaningful Word Match
	// -----------------------------------------------------------------------
	firstWord := extractFirstMeaningfulWord(rawName)
	if len([]rune(firstWord)) >= 3 {
		for _, c := range candidates {
			cSearch := normalizeProductText(c.NameAr + " " + c.NameEn + " " + c.ScientificName)
			if strings.Contains(cSearch, firstWord) {
				score := CalculateTextSimilarity(normName, normalizeProductText(c.NameAr))
				if score > bestScore {
					bestScore = score
					bestCandidate = c
				}
			}
		}

		if bestCandidate != nil && bestScore >= 0.55 {
			confidence := math.Round(bestScore * 100.0)
			return &MatchResult{
				ProductID:   &bestCandidate.ID,
				Confidence:  confidence,
				Method:      MatchMethodPartial,
				MethodLabel: fmt.Sprintf("تطابق جزئي (%d%%)", int(confidence)),
			}, nil
		}
	}

	// -----------------------------------------------------------------------
	// Wave B: AI Gateway Enhancement (Plan V5 Phase 2 §2.6)
	// Only runs on rows the deterministic matcher left below the confidence cutoff (Rule R3).
	// -----------------------------------------------------------------------
	if s.aiMatcher != nil && len(candidates) > 0 {
		var candidateNames []string
		candMap := make(map[string]*CandidateProduct)
		for _, c := range candidates {
			candidateNames = append(candidateNames, c.NameAr)
			candMap[c.NameAr] = c
			if c.NameEn != "" {
				candidateNames = append(candidateNames, c.NameEn)
				candMap[c.NameEn] = c
			}
		}

		matchedName, aiScore := s.aiMatcher.MatchCandidate(ctx, rawName, candidateNames)
		if matchedName != "" && aiScore >= 0.60 {
			if matchedCand, ok := candMap[matchedName]; ok {
				aiConf := math.Round(aiScore * 100.0)
				return &MatchResult{
					ProductID:   &matchedCand.ID,
					Confidence:  aiConf,
					Method:      MatchMethodAI,
					MethodLabel: fmt.Sprintf("تطابق بالذكاء الاصطناعي (%d%%)", int(aiConf)),
				}, nil
			}
		}
	}

	// -----------------------------------------------------------------------
	// Strategy 5: Unmatched (< 55%)
	// -----------------------------------------------------------------------
	return &MatchResult{
		ProductID:   nil,
		Confidence:  0.0,
		Method:      MatchMethodUnmatched,
		MethodLabel: "غير مطابق",
	}, nil
}

// SaveManualCorrection records user-corrected match in compare.file_rows and persists it to catalog.customer_product_mappings (Plan V5 §2.4.3).
func (s *Service) SaveManualCorrection(ctx context.Context, orgID *int64, rowID int64, rawName string, productID int64) error {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" || productID <= 0 {
		return apperr.Validation("correction.invalid", "Invalid product or name for correction.", nil)
	}

	// 1. Update the row in compare.file_rows
	if err := s.repo.UpdateFileRowMatch(ctx, rowID, &productID, MatchMethodManual, 100.0); err != nil {
		return err
	}

	// 2. Persist to catalog.customer_product_mappings for future auto-match reuse
	return s.repo.SaveCustomerProductMapping(ctx, orgID, rawName, productID, "manual")
}

// Helper utilities

func normalizeProductText(s string) string {
	s = arabic.Normalize(s)
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func extractFirstMeaningfulWord(s string) string {
	normalized := normalizeProductText(s)
	words := strings.Fields(normalized)
	for _, w := range words {
		// Skip short stop words
		if len([]rune(w)) >= 3 {
			return w
		}
	}
	if len(words) > 0 {
		return words[0]
	}
	return ""
}

// CalculateTextSimilarity computes string similarity between two normalized strings.
func CalculateTextSimilarity(s1, s2 string) float64 {
	if s1 == "" || s2 == "" {
		return 0.0
	}
	if s1 == s2 {
		return 1.0
	}

	// Substring bonus
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		lenMin := min(len(s1), len(s2))
		lenMax := max(len(s1), len(s2))
		ratio := float64(lenMin) / float64(lenMax)
		return 0.70 + (0.30 * ratio)
	}

	// Character bigram similarity (Dice coefficient)
	bigrams1 := getBigrams(s1)
	bigrams2 := getBigrams(s2)

	bigramScore := 0.0
	if len(bigrams1) > 0 && len(bigrams2) > 0 {
		intersection := 0
		for bg := range bigrams1 {
			if bigrams2[bg] {
				intersection++
			}
		}
		bigramScore = (2.0 * float64(intersection)) / float64(len(bigrams1)+len(bigrams2))
	}

	// Word token overlap bonus
	w1 := strings.Fields(s1)
	w2 := strings.Fields(s2)
	if len(w1) > 0 && len(w2) > 0 {
		commonWords := 0
		w2Map := make(map[string]bool)
		for _, w := range w2 {
			w2Map[w] = true
		}
		for _, w := range w1 {
			if len([]rune(w)) >= 3 && w2Map[w] {
				commonWords++
			}
		}
		if commonWords > 0 {
			overlapScore := float64(commonWords) / float64(max(len(w1), len(w2)))
			// If primary initial drug token matches, baseline similarity is 0.60
			if w1[0] == w2[0] {
				return math.Max(0.60+(0.35*overlapScore), bigramScore)
			}
			return math.Max(0.50+(0.40*overlapScore), bigramScore)
		}
	}

	return bigramScore
}
