package compare

import (
	"fmt"
)

// The generated commentary: التحليل الاستراتيجي التلقائي, نصيحة استراتيجية and
// توجيهات الشراء المثالي.
//
// What was here before was a slice of three constant Arabic sentences,
// returned unconditionally, recommending that the reader consolidate purchasing
// on high-spread items and focus on local manufacturers. Both sentences may
// well be good advice. Neither was derived from the reader's data, both
// appeared on an empty market as readily as on a full one, and a report that
// says the same thing whatever it is looking at teaches its reader to stop
// reading it.
//
// Every sentence below is a function of numbers already computed. Where the
// numbers do not support a statement, no statement is produced — an analysis
// section with two entries because only two things could be said is more useful
// than one with six because six were written down in advance.

// emptyMarketAnalysis explains an empty report in terms of what is missing,
// which is the only thing a reader can act on.
func emptyMarketAnalysis(c MarketCoverage) []Insight {
	switch {
	case c.Offers == 0:
		return []Insight{{
			Tone: ToneWarn,
			Text: "لا توجد عروض منشورة في السوق حالياً. تظهر بيانات هذا التقرير بعد رفع قوائم موردين ومشاركتها كمستودعات مؤقتة أو عروض عامة.",
		}}
	case c.Products <= 1:
		return []Insight{{
			Tone: ToneWarn,
			Text: fmt.Sprintf("السوق يحتوي على %d عرضاً من مورد واحد فقط، ولا تتوفر مقارنة قبل وجود موردين اثنين على الأقل لنفس الصنف.", c.Offers),
		}}
	default:
		return []Insight{{
			Tone: ToneWarn,
			Text: fmt.Sprintf(
				"تم رصد %d عرضاً عبر %d مورد، لكن لا يوجد صنف واحد يعرضه أكثر من مورد. غالباً لم تُشغَّل مطابقة الأصناف على الملفات المرفوعة: بدونها يُعامَل كل اختلاف في كتابة الاسم كصنف مستقل.",
				c.Offers, c.Suppliers),
		}}
	}
}

// strategicAnalysis is التحليل الاستراتيجي التلقائي.
func strategicAnalysis(r *StrategicSavingReport) []Insight {
	var out []Insight

	if r.PotentialSavings.IsPositive() {
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(
				"شراء كل صنف من أرخص مورد له يخفض تكلفة سلة من %s إلى %s، أي وفر قدره %s (%.1f%%) على %d صنفاً قابلاً للمقارنة.",
				r.WorstCost.String(), r.OptimalCost.String(),
				r.PotentialSavings.String(), r.SavingsPercent, r.Coverage.ComparableProduct),
		})
	} else {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(
				"الأسعار متقاربة عبر الموردين على %d صنفاً قابلاً للمقارنة، ولا يوجد فارق تكلفة يُذكر بين الشراء المجزأ والشراء الموحد.",
				r.Coverage.ComparableProduct),
		})
	}

	// How much of the market the comparison actually reached.
	uncomparable := r.Coverage.Products - r.Coverage.ComparableProduct
	if uncomparable > 0 {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(
				"من %d صنف مرصود، %d صنفاً يعرضه أكثر من مورد و%d صنفاً متاح لدى مورد واحد فقط — هذه الأخيرة لا تقبل المقارنة وتمثل نقاط ارتكاز تفاوضية أو نواقص سوق.",
				r.Coverage.Products, r.Coverage.ComparableProduct, uncomparable),
		})
	}

	// The honest statement about matching coverage, which is the single biggest
	// determinant of whether this report is trustworthy.
	matched := r.Coverage.MatchedPercent()
	switch {
	case r.Coverage.Offers == 0:
	case matched >= 70:
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(
				"%.0f%% من عروض السوق مرتبطة بالكتالوج المركزي (%d من %d)، فالمقارنة تتم على مستوى الصنف نفسه وليس على تشابه كتابة الاسم.",
				matched, r.Coverage.MatchedOffers, r.Coverage.Offers),
		})
	case matched > 0:
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(
				"%.0f%% فقط من عروض السوق مرتبطة بالكتالوج المركزي (%d من %d). تشغيل المطابقة على بقية الملفات يرفع دقة هذا التقرير مباشرة.",
				matched, r.Coverage.MatchedOffers, r.Coverage.Offers),
		})
	default:
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(
				"لم تُربط أي من عروض السوق (%d عرضاً) بالكتالوج المركزي بعد، والمقارنة الحالية مبنية على تطابق كتابة الأسماء وحدها. شغّل المطابقة على الملفات لرفع الدقة.",
				r.Coverage.Offers),
		})
	}

	// The rejected rows. Naming them is the difference between a supplier
	// concluding the tool is broken and a supplier fixing their column mapping.
	if r.Coverage.Rejected > 0 {
		text := fmt.Sprintf(
			"استُبعد %d صفاً من المقارنة لأن سعره النهائي صفر — غالباً عمود «الخصم» في الملف لا يحمل نسبة خصم.",
			r.Coverage.Rejected)
		if len(r.RemapNeeded) > 0 {
			text += " الأكثر تأثراً: "
			for i, sr := range r.RemapNeeded {
				if i > 0 {
					text += "، "
				}
				text += fmt.Sprintf("%s (%d صف)", sr.SupplierName, sr.Rows)
			}
			text += ". أعد ربط أعمدة هذه الملفات من مركز الملفات."
		}
		out = append(out, Insight{Tone: ToneWarn, Text: text})
	}

	if !r.Coverage.AIEnabled {
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: "مطابقة الذكاء الاصطناعي غير مفعّلة على المنصة حالياً، والمطابقة تعمل بالقواعد الحتمية وحدها. تفعيلها يزيد عدد الأصناف المرتبطة بالكتالوج وبالتالي دقة هذا التقرير.",
		})
	}

	return out
}

// strategicAdvice is نصيحة استراتيجية: what to do about the numbers above.
func strategicAdvice(r *StrategicSavingReport) []Insight {
	var out []Insight

	// The negotiation opportunity, named with the actual product and figures.
	if len(r.TopSavings) > 0 {
		t := r.TopSavings[0]
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(
				"أكبر فرصة تفاوض: «%s» — %s لدى %s مقابل %s لدى %s، بفارق %s للوحدة (%.1f%%). اعرض سعر السوق الأفضل على الأغلى للحصول على مطابقة سعرية.",
				t.ProductName, t.BestNet.String(), t.BestSupplier,
				t.WorstNet.String(), t.WorstSupplier,
				t.PriceDifference.String(), t.PricePercent),
		})
	}

	// A supplier who is systematically dearer is a supplier to renegotiate with
	// or replace, and naming them with the figure is the whole point.
	if worst := dearestStanding(r.Standings); worst != nil && worst.AvgPremium >= 5 {
		out = append(out, Insight{
			Tone: ToneWarn,
			Text: fmt.Sprintf(
				"«%s» أغلى من أفضل عرض بالسوق بمتوسط %.1f%% على %d صنفاً يعرضها. سلة مشترياتك منه تكلف %s بينما تكلف %s عند الشراء الأمثل — فارق %s.",
				worst.SupplierName, worst.AvgPremium, worst.Offers,
				worst.BasketCost.String(), worst.BasketBest.String(), worst.Excess().String()),
		})
	}

	// Discount terms as distinct from price. A supplier may be cheap on list
	// price and stingy on terms, and buyers negotiate the terms.
	if len(r.TopSavings) > 0 {
		if d := widestDiscountGap(r.TopSavings); d != nil && d.DiscountDifference >= 5 {
			out = append(out, Insight{
				Tone: ToneGood,
				Text: fmt.Sprintf(
					"أكبر فارق في شروط الخصم: «%s» بخصم %.1f%% لدى %s مقابل %.1f%% لدى %s — فارق %.1f نقطة يصلح كأساس لمراجعة شروط التعاقد.",
					d.ProductName, d.BestDiscount, d.BestSupplier,
					d.WorstDiscount, d.WorstSupplier, d.DiscountDifference),
			})
		}
	}

	if r.Coverage.ExclusiveProduct > 0 {
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(
				"%d صنفاً متاح لدى مورد واحد فقط (يُعرض منها أعلى %d قيمةً أدناه). راجعها كفرص لتأمين مصدر بديل، أو كأصناف يمكن التفاوض عليها من موقع الحاجة لا من موقع المقارنة.",
				r.Coverage.ExclusiveProduct, len(r.Exclusives)),
		})
	}

	return out
}

// purchasingGuidance is توجيهات الشراء المثالي: the concrete plan.
func purchasingGuidance(r *StrategicSavingReport) []Insight {
	var out []Insight

	if r.BestSupplier != nil {
		b := r.BestSupplier
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(
				"ابدأ من «%s»: يعرض %d صنفاً من الأصناف المقارَنة، وهو الأرخص في %d منها (%.0f%%)، بمتوسط فارق %.1f%% عن أفضل سعر بالسوق ومتوسط خصم %.1f%%.",
				b.SupplierName, b.Offers, b.Wins, b.WinRate, b.AvgPremium, b.AvgDiscount),
		})
	}

	if r.PotentialSavings.IsPositive() && len(r.TopSavings) > 0 {
		// Concentration: how much of the total prize sits in the visible table.
		var top int64
		for _, t := range r.TopSavings {
			top += t.PriceDifference.Minor()
		}
		share := float64(top) / float64(r.PotentialSavings.Minor()) * 100
		out = append(out, Insight{
			Tone: ToneGood,
			Text: fmt.Sprintf(
				"أعلى %d صنفاً في الجدول أدناه تمثل %.0f%% من إجمالي الوفر الممكن. معالجتها وحدها تحقق معظم المكسب دون إعادة توزيع كل المشتريات.",
				len(r.TopSavings), share),
		})
	}

	if len(r.Standings) >= 2 && r.PotentialSavings.IsPositive() {
		// How many suppliers the optimal basket actually draws on, not how many
		// were ranked. "الشراء المجزأ عبر 15 مورداً" when only four of them are
		// ever the cheapest is a sentence that misstates the work involved.
		winners := 0
		for _, st := range r.Standings {
			if st.Wins > 0 {
				winners++
			}
		}
		if winners < 2 {
			winners = 2
		}
		out = append(out, Insight{
			Tone: ToneNeutral,
			Text: fmt.Sprintf(
				"الشراء المجزأ يوزّع الطلب على %d مورداً على الأقل مقابل وفر %s. إذا كانت تكلفة التعامل مع موردين متعددين أعلى من ذلك، فالشراء الموحد من «%s» هو الخيار الأفضل عملياً.",
				winners, r.PotentialSavings.String(), r.Standings[0].SupplierName),
		})
	}

	return out
}

// dearestStanding is the ranked supplier with the largest average premium.
func dearestStanding(standings []*SupplierStanding) *SupplierStanding {
	var worst *SupplierStanding
	for _, s := range standings {
		if worst == nil || s.AvgPremium > worst.AvgPremium {
			worst = s
		}
	}
	return worst
}

// widestDiscountGap is the listed opportunity with the largest gap in terms.
func widestDiscountGap(items []*SavingOpportunity) *SavingOpportunity {
	var best *SavingOpportunity
	for _, it := range items {
		if best == nil || it.DiscountDifference > best.DiscountDifference {
			best = it
		}
	}
	return best
}
