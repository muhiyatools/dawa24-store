package workflow

import (
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// MatchLineAgainstOffers finds exact and similar candidate offers for a requested product line.
func MatchLineAgainstOffers(line ParsedProductLine, allCandidates []MatchedVendorOffer, prefs Priorities) MatchedProductEntry {
	var exact []MatchedVendorOffer
	var similar []MatchedVendorOffer

	normTarget := normalizePharmaText(line.ProductName)

	for _, cand := range allCandidates {
		sim := calculateOfferSimilarity(line, cand, normTarget)
		cand.SimilarityScore = sim

		// Calculate priority score for the offer
		candProd := CandidateProduct{
			ProductID:            cand.ProductID,
			ProductName:          cand.ProductName,
			ProductPrice:         cand.Price,
			ProductPriceDiscount: cand.FinalPrice,
			OrganizationID:       cand.OrganizationID,
			EstimatedDelivery:    1,
		}
		score, _ := Score(candProd, 10, 500, prefs)
		cand.PriorityScore = score

		if sim >= 0.55 {
			exact = append(exact, cand)
		} else if sim >= 0.30 {
			similar = append(similar, cand)
		}
	}

	// Sort matches descending by PriorityScore
	sort.SliceStable(exact, func(i, j int) bool {
		return exact[i].PriorityScore > exact[j].PriorityScore
	})
	sort.SliceStable(similar, func(i, j int) bool {
		return similar[i].PriorityScore > similar[j].PriorityScore
	})

	var best *MatchedVendorOffer
	conf := 0.0
	if len(exact) > 0 {
		b := exact[0]
		best = &b
		conf = exact[0].SimilarityScore
	} else if len(similar) > 0 {
		b := similar[0]
		best = &b
		conf = similar[0].SimilarityScore
	}

	return MatchedProductEntry{
		RequestedLine:   line,
		ExactMatches:    exact,
		SimilarMatches:  similar,
		BestOffer:       best,
		MatchConfidence: conf,
	}
}

func calculateOfferSimilarity(line ParsedProductLine, cand MatchedVendorOffer, normTarget string) float64 {
	// 1. Exact SKU or Barcode match (1.00)
	if line.ProductSKU != "" && strings.EqualFold(line.ProductSKU, cand.ProductSKU) {
		return 1.00
	}
	if line.ProductBarcode != "" && strings.EqualFold(line.ProductBarcode, cand.ProductSKU) {
		return 1.00
	}

	// 2. Exact name match (0.95)
	if strings.EqualFold(line.ProductName, cand.ProductName) {
		return 0.95
	}

	// 3. Normalized name match (0.85)
	normCand := normalizePharmaText(cand.ProductName)
	if normTarget != "" && normTarget == normCand {
		return 0.85
	}

	// 4. Substring / contains match (0.75)
	if normTarget != "" && normCand != "" {
		if strings.Contains(normCand, normTarget) || strings.Contains(normTarget, normCand) {
			return 0.75
		}
	}

	// 5. Trigram similarity
	sim := trigramSimilarity(normTarget, normCand)
	return math.Round(sim*100) / 100
}

func normalizePharmaText(s string) string {
	s = arabic.Normalize(s)
	s = strings.ToLower(s)
	return strings.TrimSpace(s)
}

func trigramSimilarity(s1, s2 string) float64 {
	if len(s1) == 0 || len(s2) == 0 {
		return 0
	}
	if s1 == s2 {
		return 1.0
	}
	tri1 := getTrigrams(s1)
	tri2 := getTrigrams(s2)
	if len(tri1) == 0 || len(tri2) == 0 {
		return 0
	}
	common := 0
	m := make(map[string]int)
	for _, t := range tri1 {
		m[t]++
	}
	for _, t := range tri2 {
		if m[t] > 0 {
			common++
			m[t]--
		}
	}
	return float64(2*common) / float64(len(tri1)+len(tri2))
}

func getTrigrams(s string) []string {
	runes := []rune(s)
	if len(runes) < 3 {
		return []string{s}
	}
	var tri []string
	for i := 0; i <= len(runes)-3; i++ {
		tri = append(tri, string(runes[i:i+3]))
	}
	return tri
}
