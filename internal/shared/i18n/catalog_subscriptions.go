package i18n

// The subscriber log's vocabulary.
//
// Kept in its own file rather than appended to catalog_billing_hr.go because
// that file is already near the size ceiling, and because these keys describe
// one screen: /admin/plans?tab=subscriptions.

func loadSubscriptionLogKeys(e *engine) {
	addKey(e, "admin.subs.empty_title", "admin",
		"لا توجد اشتراكات مطابقة", "No matching subscriptions",
		"Subscriber log empty state title")
	addKey(e, "admin.subs.empty_body", "admin",
		"جرّب توسيع الفلاتر أو إزالتها لعرض سجلات الاشتراكات.",
		"Try widening or clearing the filters to see subscription records.",
		"Subscriber log empty state body")

	addKey(e, "admin.subs.col_ref", "admin", "رقم الاشتراك", "Reference", "Subscriber log column")
	addKey(e, "admin.subs.col_buyer", "admin", "المنشأة / المستفيد", "Organization / subscriber", "Subscriber log column")
	addKey(e, "admin.subs.col_plan", "admin", "الباقة", "Plan", "Subscriber log column")
	addKey(e, "admin.subs.col_amount", "admin", "المبلغ المدفوع", "Amount paid", "Subscriber log column")
	addKey(e, "admin.subs.col_status", "admin", "الحالة", "Status", "Subscriber log column")
	addKey(e, "admin.subs.col_cycle", "admin", "دورة الفوترة", "Billing cycle", "Subscriber log column")
	addKey(e, "admin.subs.col_renew", "admin", "التجديد التلقائي", "Auto-renew", "Subscriber log column")
	addKey(e, "admin.subs.col_starts", "admin", "تاريخ البدء", "Starts", "Subscriber log column")
	addKey(e, "admin.subs.col_expires", "admin", "تاريخ الانتهاء", "Expires", "Subscriber log column")
	addKey(e, "admin.subs.col_remaining", "admin", "الأيام المتبقية", "Days left", "Subscriber log column")
	addKey(e, "admin.subs.col_date", "admin", "التاريخ", "Date", "Subscription history column")
	addKey(e, "admin.subs.col_action", "admin", "العملية", "Action", "Subscription history column")
	addKey(e, "admin.subs.col_details", "admin", "التفاصيل", "Details", "Subscription history column")

	addKey(e, "admin.subs.personal", "admin", "اشتراك شخصي", "Personal", "Badge for a subscription with no organization")
	addKey(e, "admin.subs.free", "admin", "مجانية", "Free", "Badge for a free plan subscription")

	addKey(e, "admin.subs.status_active", "admin", "ساري وفعال", "Active", "Subscription status")
	addKey(e, "admin.subs.status_trialing", "admin", "فترة تجريبية", "Trialing", "Subscription status")
	addKey(e, "admin.subs.status_past_due", "admin", "متأخر السداد", "Past due", "Subscription status")
	addKey(e, "admin.subs.status_cancelled", "admin", "ملغي", "Cancelled", "Subscription status")
	// A term that has run out is reported from expires_at rather than from the
	// status column: nothing sweeps status when a term lapses, so a row can read
	// active months after it ended.
	addKey(e, "admin.subs.status_expired", "admin", "منتهي", "Expired", "Subscription status")

	addKey(e, "admin.subs.cycle_monthly", "admin", "شهري", "Monthly", "Billing cycle")
	addKey(e, "admin.subs.cycle_annual", "admin", "سنوي", "Annual", "Billing cycle")

	addKey(e, "admin.subs.filter_org", "admin", "المنشأة أو المستفيد", "Organization or subscriber", "Subscriber log filter")
	addKey(e, "admin.subs.filter_org_ph", "admin", "ابحث بالاسم التجاري أو البريد...", "Search by trade name or email…", "Subscriber log filter placeholder")
	addKey(e, "admin.subs.filter_expiring", "admin", "ينتهي خلال", "Expiring within", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expiring_days", "admin", "%d يوم", "%d days", "Subscriber log expiring window option")
	addKey(e, "admin.subs.filter_starts_from", "admin", "بدأ من", "Started from", "Subscriber log filter")
	addKey(e, "admin.subs.filter_starts_to", "admin", "بدأ حتى", "Started to", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expires_from", "admin", "ينتهي من", "Expires from", "Subscriber log filter")
	addKey(e, "admin.subs.filter_expires_to", "admin", "ينتهي حتى", "Expires to", "Subscriber log filter")

	addKey(e, "admin.subs.history_action", "admin", "السجل", "History", "Subscription history button")
	addKey(e, "admin.subs.history_title", "admin", "سجل تغييرات الاشتراك", "Subscription history", "Subscription history modal title")
	addKey(e, "admin.subs.history_empty", "admin", "لا توجد حركات مسجلة لهذا الاشتراك.", "No recorded changes for this subscription.", "Subscription history empty state")

	addKey(e, "admin.subs.action_subscription_checkout", "admin", "اشتراك جديد", "New subscription", "Subscription history action")
	addKey(e, "admin.subs.action_renewed", "admin", "تجديد", "Renewal", "Subscription history action (legacy value)")
	addKey(e, "admin.subs.action_subscription_renewal", "admin", "تجديد", "Renewal", "Subscription history action")
	addKey(e, "admin.subs.action_subscription_change", "admin", "تغيير باقة", "Plan change", "Subscription history action")
	addKey(e, "admin.subs.action_subscription_upgrade", "admin", "ترقية", "Upgrade", "Subscription history action")
	addKey(e, "admin.subs.action_cancel", "admin", "إلغاء", "Cancellation", "Subscription history action")
	addKey(e, "admin.subs.action_", "admin", "غير محدد", "Unspecified", "Subscription history action fallback")

	addKey(e, "common.apply_filters", "common", "تطبيق الفلاتر", "Apply filters", "Filter bar submit")
	addKey(e, "common.reset", "common", "إعادة تعيين", "Reset", "Filter bar reset")
}

// loadSharedYesNoAndOrgTypeKeys fills gaps the subscriber log surfaced.
//
// common.yes/common.no and the org.type.* labels were being rendered from
// hardcoded strings at each call site, so a screen showing an organisation's
// type in Arabic had no English at all.
func loadSharedYesNoAndOrgTypeKeys(e *engine) {
	addKey(e, "common.yes", "common", "نعم", "Yes", "Boolean yes")
	addKey(e, "common.no", "common", "لا", "No", "Boolean no")

	addKey(e, "org.type.vendor", "org", "مورد", "Supplier", "Organization type")
	addKey(e, "org.type.supplier", "org", "مورد", "Supplier", "Organization type")
	addKey(e, "org.type.pharmacy", "org", "صيدلية", "Pharmacy", "Organization type")
	addKey(e, "org.type.customer", "org", "صيدلية", "Pharmacy", "Organization type (legacy value)")
	addKey(e, "org.type.company", "org", "شركة", "Company", "Organization type")
	addKey(e, "org.type.agency", "org", "وكالة", "Agency", "Organization type")
	addKey(e, "org.type.", "org", "غير محدد", "Unspecified", "Organization type fallback")
}

// loadAdminVariantListingKeys is /admin/product-child: every supplier's stock,
// with the branch and warehouse detail the screen was missing.
func loadAdminVariantListingKeys(e *engine) {
	addKey(e, "admin.child.title", "admin", "أصناف الموردين وعروض التوريد", "Supplier items and offers", "Page title")
	addKey(e, "admin.child.heading", "admin", "أصناف الموردين وعروض الفروع (%d)", "Supplier items and branch offers (%d)", "Page heading with total")
	addKey(e, "admin.child.subtitle", "admin",
		"استعراض أصناف الموردين مع الفرع والمخازن والكميات المتاحة في كل مخزن.",
		"Supplier items with their branch, warehouses and the quantity held in each.",
		"Page subtitle")
	addKey(e, "admin.child.back", "admin", "العودة لكتالوج المنتجات", "Back to the product catalogue", "Back link")

	addKey(e, "admin.child.col_image", "admin", "الصورة", "Image", "Column")
	addKey(e, "admin.child.col_item", "admin", "الصنف / SKU", "Item / SKU", "Column")
	addKey(e, "admin.child.col_supplier", "admin", "المورد", "Supplier", "Column")
	addKey(e, "admin.child.col_branch", "admin", "الفرع", "Branch", "Column")
	addKey(e, "admin.child.col_price", "admin", "السعر", "Price", "Column")
	addKey(e, "admin.child.col_stock", "admin", "الرصيد", "Stock", "Column")
	addKey(e, "admin.child.col_warehouses", "admin", "المخازن والكميات", "Warehouses and quantities", "Column")
	addKey(e, "admin.child.col_status", "admin", "الحالة", "Status", "Column")

	addKey(e, "admin.child.status_active", "admin", "نشط ومعروض", "Active", "Variant status")
	addKey(e, "admin.child.status_inactive", "admin", "موقوف", "Inactive", "Variant status")
	addKey(e, "admin.child.out_of_stock", "admin", "لا يوجد رصيد", "Out of stock", "Stock badge")
	addKey(e, "admin.child.parent_image", "admin", "صورة المنتج الأساسي", "Master product image", "Image provenance badge")
	addKey(e, "admin.child.any_branch", "admin", "كل الفروع", "Any branch", "Variant with no branch of its own")

	addKey(e, "admin.child.filter_search", "admin", "بحث", "Search", "Filter label")
	addKey(e, "admin.child.filter_search_ph", "admin",
		"اسم الصنف أو المورد أو SKU أو الباركود أو رقم التشغيلة...",
		"Item, supplier, SKU, barcode or batch…", "Filter placeholder")
	addKey(e, "admin.child.filter_warehouse", "admin", "المخزن", "Warehouse", "Filter label")
	addKey(e, "admin.child.filter_expiring", "admin", "قارب على الانتهاء", "Expiring soon", "Filter label")
	addKey(e, "admin.child.stock_in", "admin", "متوفر", "In stock", "Stock filter")
	addKey(e, "admin.child.stock_low", "admin", "رصيد منخفض", "Low stock", "Stock filter")
	addKey(e, "admin.child.stock_out", "admin", "نافد", "Out of stock", "Stock filter")

	addKey(e, "admin.child.empty_title", "admin", "لا توجد أصناف مطابقة", "No matching items", "Empty state")
	addKey(e, "admin.child.empty_body", "admin",
		"جرّب توسيع الفلاتر أو إزالتها لعرض أصناف الموردين.",
		"Try widening or clearing the filters to see supplier items.", "Empty state")
}
