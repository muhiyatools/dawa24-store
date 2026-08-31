package compare

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// RunMarketBenchmarkDetailed compares a supplier file with platform-wide approved supplier discounts.
func (s *Service) RunMarketBenchmarkDetailed(ctx context.Context, filter MarketBenchmarkFilter) (*MarketBenchmarkResult, error) {
	if filter.FileID <= 0 {
		return nil, apperr.Validation("compare.invalid_file", "يرجى تحديد ملف المورد للمقارنة مع السوق.", nil)
	}

	supplierFile, err := s.repo.GetFileByID(ctx, filter.FileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get supplier file: %w", err)
	}

	supplierRows, err := s.repo.ListFileRows(ctx, filter.FileID, 100000, 0)
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

	// Index market offers by normalized name, core drug key, sorted key, and SKU
	type marketAgg struct {
		discounts    []float64
		bestDiscount float64
		bestSupplier string
		bestNet      money.Amount
		count        int
	}
	marketMap := make(map[string]*marketAgg)
	marketMapCore := make(map[string]*marketAgg)
	marketMapSorted := make(map[string]*marketAgg)
	marketMapSKU := make(map[string]*marketAgg)

	for _, item := range marketRes.Items {
		if item.FileID == filter.FileID {
			continue // Skip own file
		}
		norm := normalizeProductText(item.ProductName)
		coreKey := getCoreDrugMatchKey(item.ProductName)
		sortedKey := getSortedWordsKey(item.ProductName)
		cleanSKU := strings.ToLower(strings.TrimSpace(item.SKU))

		agg := &marketAgg{
			bestDiscount: item.DiscountPercent,
			bestSupplier: item.SupplierName,
			bestNet:      item.PriceAfterDiscount,
			count:        1,
			discounts:    []float64{item.DiscountPercent},
		}

		if norm != "" {
			if existing, ok := marketMap[norm]; ok {
				existing.discounts = append(existing.discounts, item.DiscountPercent)
				existing.count++
				if item.DiscountPercent > existing.bestDiscount {
					existing.bestDiscount = item.DiscountPercent
					existing.bestSupplier = item.SupplierName
					existing.bestNet = item.PriceAfterDiscount
				}
			} else {
				marketMap[norm] = agg
			}
		}

		if coreKey != "" {
			if existing, ok := marketMapCore[coreKey]; ok {
				if item.DiscountPercent > existing.bestDiscount {
					existing.bestDiscount = item.DiscountPercent
					existing.bestSupplier = item.SupplierName
					existing.bestNet = item.PriceAfterDiscount
				}
			} else {
				marketMapCore[coreKey] = agg
			}
		}

		if sortedKey != "" {
			if _, ok := marketMapSorted[sortedKey]; !ok {
				marketMapSorted[sortedKey] = agg
			}
		}

		if cleanSKU != "" {
			if _, ok := marketMapSKU[cleanSKU]; !ok {
				marketMapSKU[cleanSKU] = agg
			}
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
		coreKey := getCoreDrugMatchKey(sr.RawName)
		sortedKey := getSortedWordsKey(sr.RawName)
		cleanSKU := strings.ToLower(strings.TrimSpace(sr.SKU))

		marketInfo, hasMarket := marketMap[norm]
		if !hasMarket && coreKey != "" {
			marketInfo, hasMarket = marketMapCore[coreKey]
		}
		if !hasMarket && sortedKey != "" {
			marketInfo, hasMarket = marketMapSorted[sortedKey]
		}
		if !hasMarket && cleanSKU != "" {
			marketInfo, hasMarket = marketMapSKU[cleanSKU]
		}

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
