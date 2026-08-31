package compare

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

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

// drugPhoneticMap maps common Arabic drug brand names & modifiers to canonical forms for cross-language matching.
var drugPhoneticMap = map[string]string{
	"بانادول": "panadol", "بنادول": "panadol", "باراسيتامول": "paracetamol",
	"كونجستال": "congestal", "كتافلام": "cataflam", "فولتارين": "voltaren",
	"اوجمنتين": "augmentin", "اوجمينتين": "augmentin", "بروفين": "brufen",
	"انتينال": "antinal", "امريزول": "amrizole", "فلاجيل": "flagyl",
	"سيبتازول": "septazole", "اوميبرازول": "omeprazole", "اتورفاستاتين": "atorvastatin",
	"اميبريديل": "amipride", "كابوتن": "capoten", "الفينترن": "alphintern",
	"كونكور": "concor", "موبيتيل": "mobitil", "نوفالدول": "novaldol",
	"بروسبان": "prospan", "ستربسلز": "strepsils", "داونيل": "daonil",
	"جلوكوفاج": "glucophage", "سيرفيتام": "cervitam", "سبازموبيرالجين": "spasmopyralgin",
	"بوسكوبان": "buscopan", "ترايتيكو": "trittico", "نيوروتون": "neuroton",
	"نيوروبيون": "neurobion", "كيورام": "curam", "هاي بيوتك": "hibiotic",
	"كلافيموكس": "klavimox", "ميجا موكس": "megamox", "يونيكتام": "unicatam",
	"زيثروماكس": "zithromax", "زيثرون": "zithron", "سوبراكس": "suprax",
	"سيفاكسون": "cefaxone", "سيفوتاكس": "cefotax", "يوناسين": "unasyn",
	"كلاسيد": "klacid", "تارجو": "targo", "ليفانيك": "levanic",
	"سيبروفار": "ciprofar", "سيبروسين": "ciprocin", "تارينج": "taring",
	"اكسترا": "extra", "بلس": "plus", "فورت": "forte", "ماكس": "max",
	"ادفانس": "advance", "نايت": "night", "فاست": "fast", "كومبي": "combi",
	"ريتارد": "retard", "رابد": "rapid", "كولد": "cold", "فلو": "flu",
}

// pharmaNoiseWords contains common pharmaceutical dosage forms and noise words that should be stripped for matching core products.
var pharmaNoiseWords = map[string]bool{
	"اقراص": true, "قرص": true, "كبسول": true, "كبسولات": true, "امبول": true, "امبولات": true,
	"شرب": true, "شراب": true, "نقط": true, "نقطة": true, "دهان": true, "مرهم": true, "كريم": true,
	"فوار": true, "لبوس": true, "لبوسة": true, "بخاخ": true, "بخاخة": true, "قطرة": true,
	"محلول": true, "حقن": true, "حقنة": true, "شريط": true, "علبة": true, "عبوة": true,
	"تشغيلة": true, "tab": true, "tabs": true, "tablet": true, "tablets": true,
	"cap": true, "caps": true, "capsule": true, "capsules": true, "amp": true, "amps": true,
	"ampoule": true, "ampoules": true, "syr": true, "syrup": true, "susp": true, "suspension": true,
	"drops": true, "drop": true, "cream": true, "oint": true, "ointment": true, "gel": true,
	"vial": true, "vials": true, "supp": true, "suppositories": true, "spray": true,
	"sachet": true, "sachets": true, "eff": true, "effervescent": true, "solution": true,
	"sol": true, "inj": true, "injection": true, "strip": true, "box": true, "pack": true,
	"oral": true, "topical": true, "nasal": true, "eye": true, "ear": true,
}

var (
	packCountRegex = regexp.MustCompile(`(?i)\b\d+\s*(?:tab|tabs|tablet|tablets|cap|caps|capsule|capsules|amp|amps|ampoule|ampoules|sachet|sachets|قرص|اقراص|كبسول|كبسولات|امبول|امبولات|شريط|كيس|اكياس)\b`)
	strengthRegex  = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(mg|mcg|gm|g|ml|iu|مجم|جرام|جم|مل)`)
)

// getCoreDrugMatchKey extracts a unified, clean, phonetic, noise-free representation of a pharmaceutical product.
func getCoreDrugMatchKey(name string) string {
	norm := normalizeProductText(name)
	if norm == "" {
		return ""
	}

	// 1. Strip pack count phrases (e.g. "24 tab", "20 قرص", "14 tabs")
	norm = packCountRegex.ReplaceAllString(norm, " ")

	// 2. Standardize strengths (e.g. "50 mg" -> "50mg", "1 gm" -> "1g", "1000 mg" -> "1g", "120 ml" -> "120ml")
	norm = strengthRegex.ReplaceAllStringFunc(norm, func(m string) string {
		sub := strengthRegex.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		numStr := sub[1]
		unit := strings.ToLower(sub[2])
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return m
		}
		switch unit {
		case "mg", "مجم":
			if val == 1000 {
				return "1g"
			}
			return fmt.Sprintf("%gmg", val)
		case "g", "gm", "جم", "جرام":
			return fmt.Sprintf("%gg", val)
		case "ml", "مل":
			return fmt.Sprintf("%gml", val)
		case "mcg":
			return fmt.Sprintf("%gmcg", val)
		case "iu":
			return fmt.Sprintf("%giu", val)
		default:
			return fmt.Sprintf("%g%s", val, unit)
		}
	})

	rawTokens := strings.Fields(norm)
	var cleanTokens []string

	for _, token := range rawTokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}

		if mapped, ok := drugPhoneticMap[token]; ok {
			token = mapped
		}

		if pharmaNoiseWords[token] {
			continue
		}

		if len(token) <= 3 && isPureDigits(token) {
			continue
		}

		cleanTokens = append(cleanTokens, token)
	}

	if len(cleanTokens) == 0 {
		return norm
	}

	sort.Strings(cleanTokens)
	return strings.Join(cleanTokens, " ")
}

// GetCoreDrugMatchKeyForTest exports getCoreDrugMatchKey for package tests.
func GetCoreDrugMatchKeyForTest(name string) string {
	return getCoreDrugMatchKey(name)
}

func isPureDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
