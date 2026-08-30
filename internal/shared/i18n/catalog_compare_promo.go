package i18n

func loadCompareAndPromoKeys(e *engine) {
	// --- Compare & Smart Savings ---
	addKey(e, "compare.title", "compare", "مقارنة الأسعار والعروض الذكية", "Smart Savings & Price Comparison", "Compare page title")
	addKey(e, "compare.subtitle", "compare", "قارن أسعار ونسب خصم الأدوية بين كافة الموردين المعتمدين لتوفير أعلى عائد ربحي لصيدليتك.", "Compare medicine prices and discount percentages across verified suppliers to maximize pharmacy profit.", "Compare page subtitle")
	addKey(e, "compare.search_drug", "compare", "ابحث عن دواء لمقارنة أسعاره...", "Search medicine to compare prices...", "Compare search placeholder")
	addKey(e, "compare.best_discount", "compare", "أعلى نسبة خصم متاحة", "Best Available Discount", "Best discount label")
	addKey(e, "compare.lowest_net_price", "compare", "أقل سعر صافي", "Lowest Net Price", "Lowest price label")
	addKey(e, "compare.supplier_offers", "compare", "عروض الموردين المتاحة", "Available Supplier Offers", "Supplier offers heading")
	addKey(e, "compare.public_price", "compare", "سعر الجمهور", "Public Price", "Public price column")
	addKey(e, "compare.discount_rate", "compare", "نسبة الخصم", "Discount %", "Discount column")
	addKey(e, "compare.net_unit_price", "compare", "السعر بعد الخصم", "Price After Discount", "Net price column")
	addKey(e, "compare.min_order_qty", "compare", "الحد الأدنى للطلب", "Min Order Qty", "Min qty column")
	addKey(e, "compare.stock_availability", "compare", "حالة التوافر بالمخزن", "Stock Status", "Stock status column")
	addKey(e, "compare.delivery_time", "compare", "مدة التوصيل التقديرية", "Est. Delivery Time", "Delivery time column")
	addKey(e, "compare.add_to_smart_cart", "compare", "إضافة لأفضل عرض في السلة", "Add Best Offer to Cart", "Add to cart button")
	addKey(e, "compare.alternative_drugs", "compare", "البدائل والمثائل الدوائية المتوفرة", "Available Drug Alternatives & Equivalents", "Alternatives title")
	addKey(e, "compare.same_active_ingredient", "compare", "نفس المادة الفعالة والتركيز", "Same Active Ingredient & Strength", "Same active badge")
	addKey(e, "compare.therapeutic_alternative", "compare", "بديل علاجي مماثل", "Therapeutic Equivalent", "Therapeutic badge")

	// --- Promos, Discounts & Highlighted Sections ---
	addKey(e, "promo.title", "promo", "العروض والخصومات الحصرية", "Deals & Exclusive Promotions", "Promos page title")
	addKey(e, "promo.subtitle", "promo", "تصفح أحدث العروض الخاصة، الخصومات الكمية، وباقات التوفير المقدمة من كبرى شركات توزيع الأدوية.", "Browse the latest special deals, volume discounts, and bundle offers from top pharmaceutical distributors.", "Promos page subtitle")
	addKey(e, "promo.hot_deals", "promo", "عروض اليوم المميزة", "Hot Deals of the Day", "Hot deals section")
	addKey(e, "promo.volume_discounts", "promo", "خصومات الشراء الكمي", "Volume / Bulk Discounts", "Volume discounts section")
	addKey(e, "promo.bonus_deals", "promo", "عروض البونص والكميات الإضافية", "Bonus Quantity Deals", "Bonus deals section")
	addKey(e, "promo.deal_expires_in", "promo", "ينتهي العرض خلال:", "Deal Expires In:", "Expiry countdown label")
	addKey(e, "promo.promo_code", "promo", "كود الخصم (Coupon)", "Promo Code", "Coupon code input label")
	addKey(e, "promo.apply_code", "promo", "تطبيق الكود", "Apply Code", "Apply code button")
	addKey(e, "promo.code_applied", "promo", "تم تطبيق كود الخصم بنجاح!", "Promo code applied successfully!", "Success code message")
	addKey(e, "promo.invalid_code", "promo", "كود الخصم غير صالح أو منتهي الصلاحية.", "Invalid or expired promo code.", "Invalid code message")
	addKey(e, "promo.discount_value", "promo", "قيمة الخصم المطبق", "Discount Value", "Discount value label")
	addKey(e, "promo.banner_title", "promo", "العنوان الترويجي", "Promotion Title", "Banner title")
	addKey(e, "promo.banner_link", "promo", "رابط الوجهة", "Destination Link", "Banner link")
	addKey(e, "promo.featured_suppliers", "promo", "الموردون المميزون", "Featured Suppliers", "Featured suppliers section")
}
