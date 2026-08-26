package ingest

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// vocabulary loads the catalogue's own brand and category names, which let the
// analyser recognise a column by what its values are.
//
// A failure here is not fatal: without the vocabulary the header and the value
// shapes still decide every column, and refusing to analyse a file because the
// brand list would not load would be a poor trade.
func (s *Service) vocabulary(ctx context.Context, orgID int64) *productmatch.Vocabulary {
	if s.catalog == nil {
		return productmatch.NewVocabulary(nil, nil, nil, nil)
	}
	vocab, err := s.catalog.ImportVocabulary(ctx, orgID)
	if err != nil {
		s.log.WarnContext(ctx, "import vocabulary unavailable", "error", err)
		return productmatch.NewVocabulary(nil, nil, nil, nil)
	}
	brands := make([]string, 0, len(vocab.Brands))
	for _, b := range vocab.Brands {
		brands = append(brands, b.Name)
	}
	categories := make([]string, 0, len(vocab.Categories))
	for _, c := range vocab.Categories {
		categories = append(categories, c.Name)
	}

	var warehouses, branches []string
	if s.inventory != nil {
		if list, err := s.inventory.ListWarehouses(ctx); err == nil {
			for _, w := range list {
				warehouses = append(warehouses, w.Name)
				if w.Code != "" {
					warehouses = append(warehouses, w.Code)
				}
			}
		}
	}
	return productmatch.NewVocabulary(brands, categories, warehouses, branches)
}

// describeStore is used by the handlers to explain an unavailable feature.
func (s *Service) describeStore() error {
	if s.imports == nil {
		return fmt.Errorf("%w", ErrImportStoreUnavailable)
	}
	return nil
}