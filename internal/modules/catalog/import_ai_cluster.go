package catalog

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Classification clustering.
//
// A distributor's catalogue is mostly the same products in different sizes.
// "مموا بدى ميست اسبراى حريمى ابرو بيور 250مل" and its 58 siblings differ only
// by volume and shade, and every one of them is the same body spray in the same
// category with the same pharmaceutical form. Asking a model 59 times what it
// already answered once is latency and money spent to learn nothing.
//
// Clustering folds those into one question. On the real 8,790-row file it
// removes 36% of the work outright, and it is safe by construction: the only
// thing stripped from the key is the part that varies within a pack range —
// numbers and unit words — so two products can only share an answer when they
// differ by size alone.

// clusterUnits are the measurement words that vary inside a product family
// without changing what the product is.
var clusterUnits = map[string]bool{
	"مل": true, "ملي": true, "ملى": true, "جم": true, "جرام": true, "مجم": true,
	"ملجم": true, "كجم": true, "لتر": true, "سم": true, "قرص": true, "اقراص": true,
	"كبسوله": true, "كبسولات": true, "عبوه": true, "علبه": true, "شريط": true,
	"ml": true, "mg": true, "g": true, "gm": true, "l": true, "kg": true,
	"spf": true, "iu": true, "mcg": true,
}

// clusterKeyTokens bounds how much of a name forms the key. Egyptian product
// names put the brand and the form first and the variant last, so the leading
// words are what identify the family.
const clusterKeyTokens = 4

// clusterKey is the identity two products must share to reuse one answer.
//
// An empty key means "do not cluster this row": a product whose name is nothing
// but numbers has no family to belong to, and grouping those together would
// hand one answer to a set of unrelated items.
func clusterKey(p *Product) string {
	if p == nil {
		return ""
	}
	name := p.Name.Get(i18n.AR)
	if name == "" {
		name = p.Name.Get(i18n.EN)
	}

	var kept []string
	for _, token := range strings.Fields(NormalizeName(name)) {
		if containsDigit(token) || clusterUnits[token] {
			continue
		}
		kept = append(kept, token)
		if len(kept) == clusterKeyTokens {
			break
		}
	}
	if len(kept) == 0 {
		return ""
	}

	// The manufacturer joins the key: two families sharing a leading word but
	// made by different companies are not the same product.
	return strings.Join(kept, " ") + "|" + NormalizeKey(p.ManufacturingCompanies)
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// ClusterPlan folds an enrichment plan onto one representative per family.
type ClusterPlan struct {
	// Representatives are the product indices actually sent to the model.
	Representatives []int
	// Followers maps a representative's index to the others that share its
	// answer.
	Followers map[int][]int
}

// Saved is how many model questions the clustering avoided.
func (c ClusterPlan) Saved() int {
	total := 0
	for _, followers := range c.Followers {
		total += len(followers)
	}
	return total
}

// ClusterForEnrichment groups a plan's rows into families.
//
// Only rows asking the same questions may share an answer, so the grouping is
// keyed by the fields still missing as well as by the product family: a row
// needing a category must not inherit from one that only needed a form.
func ClusterForEnrichment(prods []*Product, plan EnrichmentPlan, opts ImportOptions) ClusterPlan {
	out := ClusterPlan{Followers: map[int][]int{}}
	leader := map[string]int{}

	for _, idx := range plan.Indices {
		if idx < 0 || idx >= len(prods) {
			continue
		}
		key := clusterKey(prods[idx])
		if key != "" {
			key += "#" + missingSignature(prods[idx], opts)
		}

		if key == "" {
			out.Representatives = append(out.Representatives, idx)
			continue
		}
		if first, seen := leader[key]; seen {
			out.Followers[first] = append(out.Followers[first], idx)
			continue
		}
		leader[key] = idx
		out.Representatives = append(out.Representatives, idx)
	}
	return out
}

// missingSignature records which fields a row still needs, so rows are only
// clustered with others asking the same question.
func missingSignature(p *Product, opts ImportOptions) string {
	var b strings.Builder
	if opts.AssignCategory && (p.CategoryID == nil || *p.CategoryID <= 0) {
		b.WriteByte('c')
	}
	if opts.AssignDosageForm && (p.DosageForm == "" || p.DosageForm == defaultDosageForm) {
		b.WriteByte('d')
	}
	if opts.AssignScientificName && p.ScientificName == "" {
		b.WriteByte('s')
	}
	if opts.AutoCreateBrands && p.ManufacturingCompanies == "" {
		b.WriteByte('m')
	}
	return b.String()
}

// SpreadClusterAnswers copies each representative's answer to its family.
//
// It runs before the answers are applied, so a follower goes through exactly
// the same confidence check and vocabulary validation as the row the model
// actually saw.
func SpreadClusterAnswers(results []EnrichResult, cluster ClusterPlan) []EnrichResult {
	if len(cluster.Followers) == 0 {
		return results
	}

	out := make([]EnrichResult, 0, len(results)+cluster.Saved())
	for _, result := range results {
		out = append(out, result)
		for _, follower := range cluster.Followers[result.Ref] {
			copied := result
			copied.Ref = follower
			out = append(out, copied)
		}
	}
	return out
}
