package compare

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SupplierOffer represents a single supplier's offer for a specific product.
type SupplierOffer struct {
	SupplierName       string       `json:"supplier_name"`
	Price              money.Amount `json:"price"`
	Discount           float64      `json:"discount"` // percentage e.g. 15.50
	PriceAfterDiscount money.Amount `json:"price_after_discount"`
	StockQuantity      int          `json:"stock_quantity"`
}

// ProductComparisonRow represents the aggregated multi-supplier comparison row for a product.
type ProductComparisonRow struct {
	MatchedProductID   *int64                   `json:"matched_product_id,omitempty"`
	InCatalog          bool                     `json:"in_catalog"`
	CatalogStatus      CatalogStatus            `json:"catalog_status"`
	ProductName        string                   `json:"product_name"`
	SKU                string                   `json:"sku"`
	Offers             map[string]SupplierOffer `json:"offers"` // supplier_name -> offer
	BestPrice          money.Amount             `json:"best_price"`
	BestDiscount       float64                  `json:"best_discount"`
	BestNetPrice       money.Amount             `json:"best_net_price"`
	BestSupplier       string                   `json:"best_supplier"`
	TotalSuppliers     int                      `json:"total_suppliers"`
	MissingSuppliers   []string                 `json:"missing_suppliers"`
	AvailabilityStatus string                   `json:"availability_status"`
}

// ComparisonSummary represents the aggregate metrics across all analyzed supplier files.
type ComparisonSummary struct {
	TotalProducts        int            `json:"total_products"`
	TotalSuppliers       int            `json:"total_suppliers"`
	SuppliersList        []string       `json:"suppliers_list"`
	AverageDiscount      float64        `json:"average_discount"`
	BestOffersBySupplier map[string]int `json:"best_offers_by_supplier"`
	TotalPotentialSaving money.Amount   `json:"total_potential_saving"`
}

// ComparisonResultSet is the complete payload of a multi-supplier comparison run.
type ComparisonResultSet struct {
	Rows    []*ProductComparisonRow `json:"rows"`
	Summary ComparisonSummary       `json:"summary"`
}

// HeadToHeadItem represents one product's head-to-head comparison between two suppliers.
type HeadToHeadItem struct {
	ProductName    string       `json:"product_name"`
	SKU            string       `json:"sku"`
	SourcePrice    money.Amount `json:"source_price"`
	SourceDiscount float64      `json:"source_discount"`
	SourceNet      money.Amount `json:"source_net"`
	TargetPrice    money.Amount `json:"target_price"`
	TargetDiscount float64      `json:"target_discount"`
	TargetNet      money.Amount `json:"target_net"`
	IsBetter       bool         `json:"is_better"`  // true if SourceNet <= TargetNet
	PriceDiff      money.Amount `json:"price_diff"` // TargetNet - SourceNet
}

// HeadToHeadStats represents aggregate head-to-head metrics.
type HeadToHeadStats struct {
	SharedCount  int          `json:"shared_count"`
	BetterCount  int          `json:"better_count"`
	SourceTotal  money.Amount `json:"source_total"`
	TargetTotal  money.Amount `json:"target_total"`
	QualityScore float64      `json:"quality_score"` // (BetterCount / SharedCount) * 100
	TotalSavings money.Amount `json:"total_savings"` // TargetTotal - SourceTotal
}

// MarketComparisonFilter represents the 5 filter modes for comparing a supplier with the market baseline.
type MarketComparisonFilter string

const (
	MarketFilterAll            MarketComparisonFilter = "all"
	MarketFilterLowerDiscount  MarketComparisonFilter = "lower_discount_than_market"
	MarketFilterEqualToMarket  MarketComparisonFilter = "equal_to_market"
	MarketFilterHigherDiscount MarketComparisonFilter = "higher_discount_than_market"
	MarketFilterExclusives     MarketComparisonFilter = "exclusives"
)

// MarketComparisonItem represents a supplier product compared against the market baseline.
type MarketComparisonItem struct {
	ProductName      string                 `json:"product_name"`
	SKU              string                 `json:"sku"`
	SupplierPrice    money.Amount           `json:"supplier_price"`
	SupplierDiscount float64                `json:"supplier_discount"`
	SupplierNet      money.Amount           `json:"supplier_net"`
	MarketPrice      money.Amount           `json:"market_price"`
	MarketDiscount   float64                `json:"market_discount"`
	MarketNet        money.Amount           `json:"market_net"`
	Classification   MarketComparisonFilter `json:"classification"`
	HasMarketOffer   bool                   `json:"has_market_offer"`
}

// CalculatePriceAfterDiscount calculates the exact net price given a base price and discount percentage.
// Single currency invariant (Rule R1) using exact integer money math and banker-free half-up rounding.
func CalculatePriceAfterDiscount(price money.Amount, discountPercent float64) money.Amount {
	if discountPercent <= 0 {
		return price
	}
	if discountPercent >= 100.0 {
		return money.Zero
	}
	bps := int64(math.Round(discountPercent * 100.0))
	discountAmt := price.ApplyPercent(bps)
	net, err := price.Sub(discountAmt)
	if err != nil || net.IsNegative() {
		return money.Zero
	}
	return net
}

// getSortedWordsKey generates a bag-of-words key for order-independent name matching (e.g. "Panadol Extra" == "Extra Panadol").
func getSortedWordsKey(name string) string {
	norm := normalizeProductText(name)
	if norm == "" {
		return ""
	}
	words := strings.Fields(norm)
	sort.Strings(words)
	return strings.Join(words, " ")
}

// RunMultiSupplierComparison executes multi-supplier analysis across 1 to 10 selected files (Plan V5 Phase 2 §2.5).
func (s *Service) RunMultiSupplierComparison(ctx context.Context, fileIDs []int64) (*ComparisonResultSet, error) {
	if len(fileIDs) < 1 {
		return nil, apperr.Validation("compare.no_suppliers", "يرجى اختيار مورد واحد على الأقل للمقارنة.", nil)
	}
	if len(fileIDs) > 10 {
		return nil, apperr.Validation("compare.max_suppliers", "الحد الأقصى للمقارنة هو 10 موردين في المرة الواحدة.", nil)
	}

	// Fetch all rows for the selected files
	var files []*CompareFile
	for _, id := range fileIDs {
		f, err := s.repo.GetFileByID(ctx, id)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	var resultRows []*ProductComparisonRow
	bySKU := make(map[string]*ProductComparisonRow)
	byNorm := make(map[string]*ProductComparisonRow)
	bySorted := make(map[string]*ProductComparisonRow)
	byCatalogID := make(map[int64]*ProductComparisonRow)

	supplierBestCounts := make(map[string]int)
	var suppliersList []string

	for _, f := range files {
		suppliersList = append(suppliersList, f.SupplierName)
		rows, err := s.repo.ListFileRows(ctx, f.ID, 10000, 0)
		if err != nil {
			return nil, err
		}

		for _, r := range rows {
			normText := normalizeProductText(r.RawName)
			if normText == "" {
				normText = r.NormalizedName
			}
			sortedKey := getSortedWordsKey(r.RawName)
			cleanSKU := strings.ToLower(strings.TrimSpace(r.SKU))

			var compRow *ProductComparisonRow

			// 1. Match by Catalog ID if linked
			if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
				compRow = byCatalogID[*r.MatchedProductID]
			}
			// 2. Match by SKU
			if compRow == nil && cleanSKU != "" {
				compRow = bySKU[cleanSKU]
			}
			// 3. Match by exact normalized Arabic/English name
			if compRow == nil && normText != "" {
				compRow = byNorm[normText]
			}
			// 4. Match by bag-of-words sorted key
			if compRow == nil && sortedKey != "" {
				compRow = bySorted[sortedKey]
			}

			netPrice := r.PriceAfterDiscount
			if netPrice.IsZero() && r.Price.IsPositive() {
				netPrice = CalculatePriceAfterDiscount(r.Price, r.Discount)
			}

			offer := SupplierOffer{
				SupplierName:       f.SupplierName,
				Price:              r.Price,
				Discount:           r.Discount,
				PriceAfterDiscount: netPrice,
			}

			if compRow == nil {
				compRow = &ProductComparisonRow{
					MatchedProductID: r.MatchedProductID,
					ProductName:      r.RawName,
					SKU:              r.SKU,
					Offers:           make(map[string]SupplierOffer),
					BestPrice:        r.Price,
					BestDiscount:     r.Discount,
					BestNetPrice:     netPrice,
					BestSupplier:     f.SupplierName,
				}
				resultRows = append(resultRows, compRow)

				// Register in all lookup indices
				if r.MatchedProductID != nil && *r.MatchedProductID > 0 {
					byCatalogID[*r.MatchedProductID] = compRow
				}
				if cleanSKU != "" {
					bySKU[cleanSKU] = compRow
				}
				if normText != "" {
					byNorm[normText] = compRow
				}
				if sortedKey != "" {
					bySorted[sortedKey] = compRow
				}
			}

			compRow.Offers[f.SupplierName] = offer

			// Update best offer if this supplier has a better (lower) net price or higher discount
			if netPrice.Minor() < compRow.BestNetPrice.Minor() || (netPrice.Minor() == compRow.BestNetPrice.Minor() && r.Discount > compRow.BestDiscount) {
				compRow.BestNetPrice = netPrice
				compRow.BestPrice = r.Price
				compRow.BestDiscount = r.Discount
				compRow.BestSupplier = f.SupplierName
			}
		}
	}

	totalDiscountSum := 0.0
	discountCount := 0

	for _, row := range resultRows {
		row.TotalSuppliers = len(row.Offers)
		supplierBestCounts[row.BestSupplier]++

		// Set 3-way catalog status
		if row.MatchedProductID != nil && *row.MatchedProductID > 0 {
			row.InCatalog = true
			row.CatalogStatus = StatusCatalogAndSuppliers
		} else {
			row.InCatalog = false
			row.CatalogStatus = StatusSupplierCustom
		}

		// Calculate missing suppliers from selected comparison files
		for _, sup := range suppliersList {
			if _, hasOffer := row.Offers[sup]; !hasOffer {
				row.MissingSuppliers = append(row.MissingSuppliers, sup)
			}
		}

		// Determine availability status
		if len(row.MissingSuppliers) == 0 {
			row.AvailabilityStatus = "all_suppliers"
		} else if len(row.Offers) > 1 {
			row.AvailabilityStatus = "multi_supplier"
		} else {
			row.AvailabilityStatus = "single_exclusive"
		}

		for _, off := range row.Offers {
			if off.Discount > 0 {
				totalDiscountSum += off.Discount
				discountCount++
			}
		}
	}

	// Sort results alphabetically by product name by default
	sort.Slice(resultRows, func(i, j int) bool {
		return resultRows[i].ProductName < resultRows[j].ProductName
	})

	avgDiscount := 0.0
	if discountCount > 0 {
		avgDiscount = math.Round((totalDiscountSum/float64(discountCount))*100.0) / 100.0
	}

	summary := ComparisonSummary{
		TotalProducts:        len(resultRows),
		TotalSuppliers:       len(files),
		SuppliersList:        suppliersList,
		AverageDiscount:      avgDiscount,
		BestOffersBySupplier: supplierBestCounts,
	}

	return &ComparisonResultSet{
		Rows:    resultRows,
		Summary: summary,
	}, nil
}

// RunSupplierVsSupplier executes head-to-head comparison between two suppliers (Plan V5 Phase 2 §2.5.1).
func (s *Service) RunSupplierVsSupplier(ctx context.Context, sourceFileID, targetFileID int64) ([]*HeadToHeadItem, *HeadToHeadStats, error) {
	sourceRows, err := s.repo.ListFileRows(ctx, sourceFileID, 10000, 0)
	if err != nil {
		return nil, nil, err
	}
	targetRows, err := s.repo.ListFileRows(ctx, targetFileID, 10000, 0)
	if err != nil {
		return nil, nil, err
	}

	// Index target rows by SKU, normalized name, and bag-of-words
	targetBySKU := make(map[string]*CompareFileRow)
	targetByName := make(map[string]*CompareFileRow)
	targetBySorted := make(map[string]*CompareFileRow)

	for _, tr := range targetRows {
		cleanSKU := strings.ToLower(strings.TrimSpace(tr.SKU))
		if cleanSKU != "" {
			targetBySKU[cleanSKU] = tr
		}
		norm := normalizeProductText(tr.RawName)
		if norm != "" {
			targetByName[norm] = tr
		}
		sortedKey := getSortedWordsKey(tr.RawName)
		if sortedKey != "" {
			targetBySorted[sortedKey] = tr
		}
	}

	var matched []*HeadToHeadItem
	betterOrEqualCount := 0
	var sourceTotal money.Amount
	var targetTotal money.Amount

	for _, sr := range sourceRows {
		var tr *CompareFileRow
		cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))
		if cleanSKU != "" {
			tr = targetBySKU[cleanSKU]
		}
		if tr == nil {
			norm := normalizeProductText(sr.RawName)
			tr = targetByName[norm]
		}
		if tr == nil {
			sortedKey := getSortedWordsKey(sr.RawName)
			tr = targetBySorted[sortedKey]
		}

		if tr != nil {
			sNet := sr.PriceAfterDiscount
			if sNet.IsZero() && sr.Price.IsPositive() {
				sNet = CalculatePriceAfterDiscount(sr.Price, sr.Discount)
			}
			tNet := tr.PriceAfterDiscount
			if tNet.IsZero() && tr.Price.IsPositive() {
				tNet = CalculatePriceAfterDiscount(tr.Price, tr.Discount)
			}

			sourceTotal, _ = sourceTotal.Add(sNet)
			targetTotal, _ = targetTotal.Add(tNet)

			isBetter := sNet.Minor() <= tNet.Minor()
			if isBetter {
				betterOrEqualCount++
			}

			priceDiff, _ := tNet.Sub(sNet)

			matched = append(matched, &HeadToHeadItem{
				ProductName:    sr.RawName,
				SKU:            sr.SKU,
				SourcePrice:    sr.Price,
				SourceDiscount: sr.Discount,
				SourceNet:      sNet,
				TargetPrice:    tr.Price,
				TargetDiscount: tr.Discount,
				TargetNet:      tNet,
				IsBetter:       isBetter,
				PriceDiff:      priceDiff,
			})
		}
	}

	sharedCount := len(matched)
	qualityScore := 0.0
	if sharedCount > 0 {
		qualityScore = math.Round((float64(betterOrEqualCount)/float64(sharedCount))*1000.0) / 10.0
	}

	totalSavings, _ := targetTotal.Sub(sourceTotal)

	stats := &HeadToHeadStats{
		SharedCount:  sharedCount,
		BetterCount:  betterOrEqualCount,
		SourceTotal:  sourceTotal,
		TargetTotal:  targetTotal,
		QualityScore: qualityScore,
		TotalSavings: totalSavings,
	}

	return matched, stats, nil
}

// ClassifyMarketComparison classifies a supplier row against market baseline into one of the 5 filter modes (Plan V5 §2.5.2).
func ClassifyMarketComparison(supplierNet, marketNet money.Amount, supplierDiscount, marketDiscount float64, hasMarketOffer bool) MarketComparisonFilter {
	if !hasMarketOffer {
		return MarketFilterExclusives
	}
	if supplierNet.Minor() < marketNet.Minor() || supplierDiscount > marketDiscount {
		return MarketFilterHigherDiscount
	}
	if supplierNet.Minor() > marketNet.Minor() || supplierDiscount < marketDiscount {
		return MarketFilterLowerDiscount
	}
	return MarketFilterEqualToMarket
}
