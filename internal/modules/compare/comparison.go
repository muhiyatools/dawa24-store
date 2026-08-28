package compare

import (
	"context"
	"fmt"
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
		rows, err := s.repo.ListFileRows(ctx, f.ID, 100000, 0)
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
	detailed, err := s.RunSupplierVsSupplierDetailed(ctx, HeadToHeadFilter{
		SourceFileID: sourceFileID,
		TargetFileID: targetFileID,
	})
	if err != nil {
		return nil, nil, err
	}

	var items []*HeadToHeadItem
	for _, r := range detailed.Rows {
		items = append(items, &HeadToHeadItem{
			ProductName:    r.ProductName,
			SKU:            r.SKU,
			SourcePrice:    r.Price,
			SourceDiscount: r.YourDiscount,
			SourceNet:      r.YourNetPrice,
			TargetPrice:    r.Price,
			TargetDiscount: r.CompetitorDiscount,
			TargetNet:      r.CompetitorNetPrice,
			IsBetter:       r.Outcome == OutcomeYourBetter || r.Outcome == OutcomeEqual,
			PriceDiff:      r.SavingsDifference,
		})
	}

	stats := &HeadToHeadStats{
		SharedCount:  detailed.TotalShared,
		BetterCount:  detailed.YourBetterCount + detailed.EqualCount,
		SourceTotal:  detailed.SourceTotal,
		TargetTotal:  detailed.TargetTotal,
		QualityScore: detailed.WinRatePercent,
		TotalSavings: detailed.TotalSavings,
	}

	return items, stats, nil
}

// RunSupplierVsSupplierDetailed executes full head-to-head comparison with custom filtering, matching, and metrics.
func (s *Service) RunSupplierVsSupplierDetailed(ctx context.Context, filter HeadToHeadFilter) (*HeadToHeadComparisonResult, error) {
	if filter.SourceFileID <= 0 || filter.TargetFileID <= 0 {
		return nil, apperr.Validation("compare.invalid_files", "يرجى تحديد المورد الأساسي والمورد المقارن به للمتابعة.", nil)
	}

	sourceFile, err := s.repo.GetFileByID(ctx, filter.SourceFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source file: %w", err)
	}
	targetFile, err := s.repo.GetFileByID(ctx, filter.TargetFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target file: %w", err)
	}

	sourceRows, err := s.repo.ListFileRows(ctx, filter.SourceFileID, 100000, 0)
	if err != nil {
		return nil, err
	}
	targetRows, err := s.repo.ListFileRows(ctx, filter.TargetFileID, 100000, 0)
	if err != nil {
		return nil, err
	}

	// Index target rows
	targetBySKU := make(map[string]*CompareFileRow)
	targetByName := make(map[string]*CompareFileRow)
	targetBySorted := make(map[string]*CompareFileRow)
	targetByProductID := make(map[int64]*CompareFileRow)

	for _, tr := range targetRows {
		if tr.MatchedProductID != nil && *tr.MatchedProductID > 0 {
			targetByProductID[*tr.MatchedProductID] = tr
		}
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

	var allSharedRows []*HeadToHeadRow
	yourBetterCount := 0
	equalCount := 0
	competitorBetterCount := 0
	var sourceTotal money.Amount
	var targetTotal money.Amount

	qLower := strings.ToLower(strings.TrimSpace(filter.Query))

	for _, sr := range sourceRows {
		var tr *CompareFileRow
		if sr.MatchedProductID != nil && *sr.MatchedProductID > 0 {
			tr = targetByProductID[*sr.MatchedProductID]
		}
		if tr == nil {
			cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))
			if cleanSKU != "" {
				tr = targetBySKU[cleanSKU]
			}
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
			price := sr.Price
			if price.IsZero() && tr.Price.IsPositive() {
				price = tr.Price
			}

			sNet := sr.PriceAfterDiscount
			if sNet.IsZero() && price.IsPositive() {
				sNet = CalculatePriceAfterDiscount(price, sr.Discount)
			}
			tNet := tr.PriceAfterDiscount
			if tNet.IsZero() && price.IsPositive() {
				tNet = CalculatePriceAfterDiscount(price, tr.Discount)
			}

			sourceTotal, _ = sourceTotal.Add(sNet)
			targetTotal, _ = targetTotal.Add(tNet)

			var outcome HeadToHeadOutcome
			betterDiff := 0.0
			compDiff := 0.0

			discDiff := sr.Discount - tr.Discount
			if math.Abs(discDiff) < 0.01 {
				outcome = OutcomeEqual
				equalCount++
			} else if sr.Discount > tr.Discount {
				outcome = OutcomeYourBetter
				betterDiff = math.Round((sr.Discount-tr.Discount)*100.0) / 100.0
				yourBetterCount++
			} else {
				outcome = OutcomeCompetitorBetter
				compDiff = math.Round((tr.Discount-sr.Discount)*100.0) / 100.0
				competitorBetterCount++
			}

			savingsDiff, _ := tNet.Sub(sNet)

			row := &HeadToHeadRow{
				ProductID:          sr.MatchedProductID,
				ProductName:        sr.RawName,
				SKU:                sr.SKU,
				Price:              price,
				YourDiscount:       sr.Discount,
				YourNetPrice:       sNet,
				CompetitorDiscount: tr.Discount,
				CompetitorNetPrice: tNet,
				Outcome:            outcome,
				BetterDiff:         betterDiff,
				CompetitorDiff:     compDiff,
				SavingsDifference:  savingsDiff,
			}

			// Apply in-memory filters
			if qLower != "" {
				nameMatch := strings.Contains(strings.ToLower(row.ProductName), qLower)
				skuMatch := strings.Contains(strings.ToLower(row.SKU), qLower)
				if !nameMatch && !skuMatch {
					continue
				}
			}

			if filter.MinPrice != nil && row.Price.Minor() < int64(*filter.MinPrice*100) {
				continue
			}
			if filter.MaxPrice != nil && row.Price.Minor() > int64(*filter.MaxPrice*100) {
				continue
			}
			if filter.MinDiscount != nil && (row.YourDiscount < *filter.MinDiscount && row.CompetitorDiscount < *filter.MinDiscount) {
				continue
			}
			if filter.MaxDiscount != nil && (row.YourDiscount > *filter.MaxDiscount && row.CompetitorDiscount > *filter.MaxDiscount) {
				continue
			}
			if filter.Outcome != nil && *filter.Outcome != "" && row.Outcome != *filter.Outcome {
				continue
			}

			allSharedRows = append(allSharedRows, row)
		}
	}

	totalShared := yourBetterCount + equalCount + competitorBetterCount
	winRate := 0.0
	if totalShared > 0 {
		winRate = math.Round((float64(yourBetterCount+equalCount)/float64(totalShared))*1000.0) / 10.0
	}
	totalSavings, _ := targetTotal.Sub(sourceTotal)

	return &HeadToHeadComparisonResult{
		SourceSupplierName: sourceFile.SupplierName,
		TargetSupplierName: targetFile.SupplierName,
		SourceFileID:       filter.SourceFileID,
		TargetFileID:       filter.TargetFileID,
		Rows:               allSharedRows,
		TotalShared:        totalShared,
		YourBetterCount:    yourBetterCount,
		EqualCount:         equalCount,
		CompetitorBetter:   competitorBetterCount,
		WinRatePercent:     winRate,
		TotalSavings:       totalSavings,
		SourceTotal:        sourceTotal,
		TargetTotal:        targetTotal,
	}, nil
}

// RunMarketBenchmarkDetailed compares a supplier file with platform-wide approved supplier discounts.
func (s *Service) RunMarketBenchmarkDetailed(ctx context.Context, filter MarketBenchmarkFilter) (*MarketBenchmarkResult, error) {
	if filter.FileID <= 0 {
		return nil, apperr.Validation("compare.invalid_file", "يرجى تحديد ملف المورد للمقارنة مع السوق.", nil)
	}

	supplierFile, err := s.repo.GetFileByID(ctx, filter.FileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get supplier file: %w", err)
	}

	supplierRows, err := s.repo.ListFileRows(ctx, filter.FileID, 20000, 0)
	if err != nil {
		return nil, err
	}

	// Fetch distinct suppliers and sample market dataset
	availableSuppliers, _ := s.repo.ListDistinctSuppliers(ctx)

	// Fetch market dataset
	marketRes, err := s.repo.ListMarketDiscounts(ctx, MarketDiscountsFilter{
		Limit: 50000,
	})
	if err != nil {
		return nil, err
	}

	// Index market offers by normalized name and SKU
	type marketAgg struct {
		discounts    []float64
		bestDiscount float64
		bestSupplier string
		bestNet      money.Amount
		count        int
	}
	marketMap := make(map[string]*marketAgg)

	for _, item := range marketRes.Items {
		if item.FileID == filter.FileID {
			continue // Skip own file
		}
		norm := normalizeProductText(item.ProductName)
		if norm == "" {
			continue
		}
		agg, ok := marketMap[norm]
		if !ok {
			agg = &marketAgg{
				bestDiscount: item.DiscountPercent,
				bestSupplier: item.SupplierName,
				bestNet:      item.PriceAfterDiscount,
				count:        0,
			}
			marketMap[norm] = agg
		}
		agg.discounts = append(agg.discounts, item.DiscountPercent)
		agg.count++
		if item.DiscountPercent > agg.bestDiscount {
			agg.bestDiscount = item.DiscountPercent
			agg.bestSupplier = item.SupplierName
			agg.bestNet = item.PriceAfterDiscount
		}
	}

	var rows []*MarketBenchmarkRow
	higherCount := 0
	equalCount := 0
	lowerCount := 0
	exclusivesCount := 0
	totalSuppDisc := 0.0
	totalMarketDisc := 0.0
	marketComparedCount := 0

	qLower := strings.ToLower(strings.TrimSpace(filter.Query))

	for _, sr := range supplierRows {
		norm := normalizeProductText(sr.RawName)
		marketInfo, hasMarket := marketMap[norm]

		var classification string
		avgMarketDisc := 0.0
		bestMarketDisc := 0.0
		bestMarketSupplier := "—"
		var bestMarketNet money.Amount
		supplierCount := 0

		if !hasMarket || marketInfo == nil || len(marketInfo.discounts) == 0 {
			classification = "exclusive"
			exclusivesCount++
		} else {
			sum := 0.0
			for _, d := range marketInfo.discounts {
				sum += d
			}
			avgMarketDisc = math.Round((sum/float64(len(marketInfo.discounts)))*100.0) / 100.0
			bestMarketDisc = marketInfo.bestDiscount
			bestMarketSupplier = marketInfo.bestSupplier
			bestMarketNet = marketInfo.bestNet
			supplierCount = marketInfo.count

			diff := sr.Discount - avgMarketDisc
			if math.Abs(diff) < 0.25 {
				classification = "equal"
				equalCount++
			} else if sr.Discount > avgMarketDisc {
				classification = "higher"
				higherCount++
			} else {
				classification = "lower"
				lowerCount++
			}

			totalMarketDisc += avgMarketDisc
			marketComparedCount++
		}

		totalSuppDisc += sr.Discount

		diffVsAvg := 0.0
		if hasMarket {
			diffVsAvg = math.Round((sr.Discount-avgMarketDisc)*100.0) / 100.0
		}

		netPrice := sr.PriceAfterDiscount
		if netPrice.IsZero() && sr.Price.IsPositive() {
			netPrice = CalculatePriceAfterDiscount(sr.Price, sr.Discount)
		}

		row := &MarketBenchmarkRow{
			ProductID:          sr.MatchedProductID,
			ProductName:        sr.RawName,
			SKU:                sr.SKU,
			YourPrice:          sr.Price,
			YourDiscount:       sr.Discount,
			YourNetPrice:       netPrice,
			MarketAvgDiscount:  avgMarketDisc,
			MarketBestDiscount: bestMarketDisc,
			MarketBestSupplier: bestMarketSupplier,
			MarketBestNetPrice: bestMarketNet,
			DiffVsMarketAvg:    diffVsAvg,
			Classification:     classification,
			SupplierCount:      supplierCount,
		}

		// Filtering
		if qLower != "" {
			nameMatch := strings.Contains(strings.ToLower(row.ProductName), qLower)
			skuMatch := strings.Contains(strings.ToLower(row.SKU), qLower)
			if !nameMatch && !skuMatch {
				continue
			}
		}

		if filter.MinPrice != nil && row.YourPrice.Minor() < int64(*filter.MinPrice*100) {
			continue
		}
		if filter.MaxPrice != nil && row.YourPrice.Minor() > int64(*filter.MaxPrice*100) {
			continue
		}
		if filter.MinDiscount != nil && row.YourDiscount < *filter.MinDiscount {
			continue
		}
		if filter.MaxDiscount != nil && row.YourDiscount > *filter.MaxDiscount {
			continue
		}

		if filter.Tab != "" && filter.Tab != "all" {
			if filter.Tab == "higher" && row.Classification != "higher" {
				continue
			}
			if filter.Tab == "equal" && row.Classification != "equal" {
				continue
			}
			if filter.Tab == "lower" && row.Classification != "lower" {
				continue
			}
			if filter.Tab == "exclusives" && row.Classification != "exclusive" {
				continue
			}
		}

		rows = append(rows, row)
	}

	avgSupp := 0.0
	if len(supplierRows) > 0 {
		avgSupp = math.Round((totalSuppDisc/float64(len(supplierRows)))*100.0) / 100.0
	}
	avgMkt := 0.0
	if marketComparedCount > 0 {
		avgMkt = math.Round((totalMarketDisc/float64(marketComparedCount))*100.0) / 100.0
	}

	return &MarketBenchmarkResult{
		SupplierName:       supplierFile.SupplierName,
		FileID:             filter.FileID,
		Rows:               rows,
		TotalItems:         len(supplierRows),
		HigherThanMarket:   higherCount,
		EqualToMarket:      equalCount,
		LowerThanMarket:    lowerCount,
		ExclusivesCount:    exclusivesCount,
		AvgSupplierDisc:    avgSupp,
		AvgMarketDisc:      avgMkt,
		AvailableSuppliers: availableSuppliers,
	}, nil
}

// GetMarketIntelligenceReport aggregates platform-wide market discount intelligence, arbitrage deals, brand stats, and gaps.
func (s *Service) GetMarketIntelligenceReport(ctx context.Context) (*MarketIntelligenceReport, error) {
	marketRes, err := s.repo.ListMarketDiscounts(ctx, MarketDiscountsFilter{
		Limit: 50000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch market discounts: %w", err)
	}

	suppliers, _ := s.repo.ListDistinctSuppliers(ctx)

	// Aggregate by product
	type prodOffers struct {
		name         string
		sku          string
		price        money.Amount
		offers       []SupplierOffer
		bestDiscount float64
		bestSupplier string
		bestNet      money.Amount
		worstDisc    float64
		worstSupp    string
		worstNet     money.Amount
	}
	byProduct := make(map[string]*prodOffers)
	var totalDiscountSum float64
	var discountCount int
	highestMarketDisc := 0.0

	// Common pharma brands in Egypt for categorization
	knownBrands := []string{
		"نوفارتس", "سانوفي", "فايزر", "أسترازينيكا", "إيفا فارما", "أمون", "إيبيكو", "فاركو",
		"ماركيرل", "جلوبال نابي", "أكتوبر فارما", "العامرية", "سيد", "النيل", "ممفيس",
		"Novartis", "Sanofi", "Pfizer", "AstraZeneca", "Eva Pharma", "Amoun", "EIPICO", "Pharco",
	}
	brandMap := make(map[string]*BrandDiscountStat)
	for _, b := range knownBrands {
		brandMap[b] = &BrandDiscountStat{
			BrandName: b,
		}
	}

	for _, item := range marketRes.Items {
		norm := normalizeProductText(item.ProductName)
		if norm == "" {
			continue
		}

		if item.DiscountPercent > highestMarketDisc {
			highestMarketDisc = item.DiscountPercent
		}
		if item.DiscountPercent > 0 {
			totalDiscountSum += item.DiscountPercent
			discountCount++
		}

		// Brand attribution
		for _, b := range knownBrands {
			if strings.Contains(strings.ToLower(item.ProductName), strings.ToLower(b)) {
				stat := brandMap[b]
				stat.ProductCount++
				stat.AvgDiscount += item.DiscountPercent
				if item.DiscountPercent > stat.MaxDiscount {
					stat.MaxDiscount = item.DiscountPercent
					stat.TopSupplier = item.SupplierName
				}
				break
			}
		}

		p, ok := byProduct[norm]
		if !ok {
			p = &prodOffers{
				name:         item.ProductName,
				sku:          item.SKU,
				price:        item.OriginalPrice,
				bestDiscount: item.DiscountPercent,
				bestSupplier: item.SupplierName,
				bestNet:      item.PriceAfterDiscount,
				worstDisc:    item.DiscountPercent,
				worstSupp:    item.SupplierName,
				worstNet:     item.PriceAfterDiscount,
			}
			byProduct[norm] = p
		}

		p.offers = append(p.offers, SupplierOffer{
			SupplierName:       item.SupplierName,
			Price:              item.OriginalPrice,
			Discount:           item.DiscountPercent,
			PriceAfterDiscount: item.PriceAfterDiscount,
		})

		if item.DiscountPercent > p.bestDiscount {
			p.bestDiscount = item.DiscountPercent
			p.bestSupplier = item.SupplierName
			p.bestNet = item.PriceAfterDiscount
		}
		if item.DiscountPercent < p.worstDisc {
			p.worstDisc = item.DiscountPercent
			p.worstSupp = item.SupplierName
			p.worstNet = item.PriceAfterDiscount
		}
	}

	var arbitrageDeals []*ArbitrageOpportunity
	var marketGaps []*MarketGapItem

	for _, p := range byProduct {
		if len(p.offers) >= 2 {
			spread := p.bestDiscount - p.worstDisc
			if spread >= 3.0 { // Notable discount spread >= 3%
				unitSavings, _ := p.worstNet.Sub(p.bestNet)
				arbitrageDeals = append(arbitrageDeals, &ArbitrageOpportunity{
					ProductName:    p.name,
					SKU:            p.sku,
					BestSupplier:   p.bestSupplier,
					BestDiscount:   p.bestDiscount,
					BestNetPrice:   p.bestNet,
					WorstSupplier:  p.worstSupp,
					WorstDiscount:  p.worstDisc,
					WorstNetPrice:  p.worstNet,
					DiscountSpread: math.Round(spread*100.0) / 100.0,
					UnitSavings:    unitSavings,
				})
			}
		} else if len(p.offers) == 1 {
			marketGaps = append(marketGaps, &MarketGapItem{
				ProductName:  p.name,
				SKU:          p.sku,
				SoleSupplier: p.offers[0].SupplierName,
				Price:        p.offers[0].Price,
				Discount:     p.offers[0].Discount,
			})
		}
	}

	// Sort arbitrage by discount spread descending
	sort.Slice(arbitrageDeals, func(i, j int) bool {
		return arbitrageDeals[i].DiscountSpread > arbitrageDeals[j].DiscountSpread
	})
	if len(arbitrageDeals) > 25 {
		arbitrageDeals = arbitrageDeals[:25]
	}

	// Clean Brand stats
	var activeBrands []*BrandDiscountStat
	for _, b := range knownBrands {
		stat := brandMap[b]
		if stat.ProductCount > 0 {
			stat.AvgDiscount = math.Round((stat.AvgDiscount/float64(stat.ProductCount))*100.0) / 100.0
			activeBrands = append(activeBrands, stat)
		}
	}
	sort.Slice(activeBrands, func(i, j int) bool {
		return activeBrands[i].ProductCount > activeBrands[j].ProductCount
	})

	overallAvg := 0.0
	if discountCount > 0 {
		overallAvg = math.Round((totalDiscountSum/float64(discountCount))*100.0) / 100.0
	}

	if len(marketGaps) > 30 {
		marketGaps = marketGaps[:30]
	}

	recommendations := []string{
		"قم بمراجعة الأصناف ذات الفارق الخصمي المرتفع (> 5%) وتوحيد الشراء من المورد الأفضل لتعظيم وفورات الصيدلية.",
		"التركيز على أصناف الشركات المحلية الكبرى (إيفا، آمون، إيبيكو) يتيح الحصول على خصومات تنافسية مستقرة تتجاوز متوسط السوق.",
		"الأصناف الحصرية لدى مورد واحد تمثل نقطة ارتكاز تفاوضية أو فرص لتوفير النواقص لعملاء الصيدليات في منطقتك.",
	}

	return &MarketIntelligenceReport{
		KPIs: MarketVitalKPIs{
			TotalTrackedProducts: len(byProduct),
			TotalActiveSuppliers: len(suppliers),
			OverallAvgDiscount:   overallAvg,
			HighestMarketDisc:    highestMarketDisc,
			TotalArbitrageDeals:  len(arbitrageDeals),
			TotalMarketGaps:      len(marketGaps),
		},
		TopArbitrage:    arbitrageDeals,
		BrandStats:      activeBrands,
		MarketGaps:      marketGaps,
		Recommendations: recommendations,
	}, nil
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
