package catalog

import (
	"context"
	"fmt"
	"strings"
)

// Categorising the products a file gave no category word for.
//
// resolveCategories translates the file's own category column onto the
// platform's categories. That is the easy half, and it is all there used to be:
// a file with no category column produced nine thousand products with a null
// category, because there was nothing to translate.
//
// Most real administrative extracts — the drug registry, a distributor's master
// list — have no category column at all. So the harder half is here: work out
// the category from what the file DOES say about the product.
//
// The signal used is the scientific name, not the product name, and that choice
// is the whole design. A brand name says nothing generalisable — بانادول and
// أدول are the same category and share no letters — while a molecule says it
// outright, and the same molecule appears on hundreds of rows. Grouping by
// molecule turns "categorise fifty thousand products" into "categorise nine
// hundred molecules", which is both affordable and far more accurate: the model
// is asked a pharmacological question it can answer rather than a branding one
// it cannot.
//
// Rows with no scientific name fall back to the product's leading word, which
// is the brand, and the brand is at least stable across a manufacturer's line.
//
// This runs in the admin main-catalogue import and nowhere else. A vendor
// import does not write catalog.products.category_id — it does not write
// catalog.products at all — so offering it there would be a switch that does
// nothing.

// categorySignal is the text a product is categorised by, and how sure we are
// that it means anything.
func categorySignal(p *Product) (signal string, strong bool) {
	if p == nil {
		return "", false
	}
	if s := CleanCellString(p.ScientificName); s != "" {
		return s, true
	}
	if s := CleanCellString(p.Active); s != "" {
		return s, true
	}
	// The brand, taken as the first word of the name. Everything after it is
	// dose, pack size and form, which vary within one product line and would
	// scatter a single brand across dozens of one-row groups.
	name := CleanCellString(p.Name.Get("ar"))
	if name == "" {
		name = CleanCellString(p.Name.Get("en"))
	}
	if name == "" {
		return "", false
	}
	if head, _, cut := strings.Cut(name, " "); cut && len([]rune(head)) >= 3 {
		return head, false
	}
	return name, false
}

// inferCategories assigns a platform category to every product still without
// one, from the molecule or the brand.
//
// It never creates a category: session.NewCategories is deliberately left
// alone. A word the file stated and the catalogue lacks is a defensible new
// category; a molecule the model could not place is not, and inventing a
// category named "Paracetamol" is how a category tree becomes a drug index.
func (s *Service) inferCategories(
	ctx context.Context, session *ImportSession, parsed *ParseResult, vocab EnrichVocabulary,
) []string {
	if parsed == nil || len(vocab.Categories) == 0 {
		return nil
	}

	pending := make([]*Product, 0, len(parsed.Products))
	for _, p := range parsed.Products {
		if p == nil {
			continue
		}
		if p.CategoryID != nil && *p.CategoryID > 0 {
			continue
		}
		pending = append(pending, p)
	}
	if len(pending) == 0 {
		return nil
	}

	signals := make(map[*Product]string, len(pending))
	var sources []string
	seen := map[string]bool{}
	for _, p := range pending {
		signal, _ := categorySignal(p)
		if signal == "" {
			continue
		}
		signals[p] = signal
		key := NormalizeKey(signal)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, signal)
	}
	if len(sources) == 0 {
		return nil
	}

	targets := make([]string, 0, len(vocab.Categories))
	idByName := make(map[string]int64, len(vocab.Categories))
	for _, option := range vocab.Categories {
		targets = append(targets, option.Name)
		idByName[option.Name] = option.ID
	}

	mapping := s.mapValuesBatched(ctx, session, ValueMapCategory, sources, targets)

	assigned := 0
	for _, p := range pending {
		signal, ok := signals[p]
		if !ok {
			continue
		}
		name, found := mapping.Lookup(signal)
		if !found {
			continue
		}
		id := idByName[name]
		if id <= 0 {
			continue
		}
		resolved := id
		p.CategoryID = &resolved
		assigned++
	}

	if assigned == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"تصنيف تلقائي: %d صنف عبر %d مادة/علامة مطابقة لفئات المنصة.",
		assigned, mapping.Matched())}
}

// mapValuesBatched runs value mapping over an unbounded source list.
//
// mapValues caps one request at maxDistinctValues, which is right for a
// category column — a file has twenty of those — and wrong for a molecule list,
// where a national drug registry has hundreds. Truncating at three hundred
// there does not degrade the answer, it silently leaves four fifths of the
// catalogue uncategorised, so the list is chunked and the answers merged.
func (s *Service) mapValuesBatched(
	ctx context.Context, session *ImportSession, kind ValueMapKind, sources, targets []string,
) ValueMapping {
	merged := ValueMapping{resolved: map[string]string{}}
	for start := 0; start < len(sources); start += maxDistinctValues {
		end := min(start+maxDistinctValues, len(sources))
		batch := s.mapValues(ctx, session, kind, sources[start:end], targets)
		for k, v := range batch.resolved {
			if _, taken := merged.resolved[k]; !taken {
				merged.resolved[k] = v
			}
		}
		// The unmatched list is not carried: for an inferred category it is a
		// list of molecules, and its only consumer creates categories from it.
	}
	return merged
}
