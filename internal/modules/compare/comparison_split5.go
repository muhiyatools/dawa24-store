package compare

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

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
