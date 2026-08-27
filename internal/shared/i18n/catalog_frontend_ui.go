package i18n

func loadFrontendUIKeys(e *engine) {
	// --- Smart Order Wizard & Results ---
	addKey(e, "smart_order.title", "smart_order", "الطلب الذكي", "Smart Order", "Smart Order title")
	addKey(e, "smart_order.step_upload", "smart_order", "رفع الملف", "Upload File", "Step 1")
	addKey(e, "smart_order.step_mapping", "smart_order", "مطابقة الأعمدة", "Column Mapping", "Step 2")
	addKey(e, "smart_order.step_processing", "smart_order", "معالجة الطلب", "Processing", "Step 3")
	addKey(e, "smart_order.step_results", "smart_order", "نتائج المطابقة", "Matching Results", "Step 4")
	addKey(e, "smart_order.step_review", "smart_order", "مراجعة الطلب والشراء", "Order Review & Purchase", "Step 5")
	addKey(e, "smart_order.unmatched", "smart_order", "غير مطابق", "Unmatched", "Unmatched tab/status")
	addKey(e, "smart_order.matched", "smart_order", "مطابق", "Matched", "Matched tab/status")
	addKey(e, "smart_order.needs_review", "smart_order", "يحتاج مراجعة", "Needs Review", "Needs review tab")
	addKey(e, "smart_order.ready_to_order", "smart_order", "جاهز للطلب", "Ready to Order", "Ordered outcome")
	addKey(e, "smart_order.no_supplier", "smart_order", "لا يوجد مورد", "No Supplier", "No supplier outcome")
	addKey(e, "smart_order.coverage_blocked", "smart_order", "خارج التغطية", "Outside Coverage", "Coverage blocked outcome")
	addKey(e, "smart_order.manual_rematch", "smart_order", "ربط بالكتالوج", "Match with Catalog", "Action button")
	addKey(e, "smart_order.change_rematch", "smart_order", "تغيير الربط", "Change Match", "Action button")
	addKey(e, "smart_order.unlink", "smart_order", "إلغاء الربط", "Unlink Product", "Unlink button")
	addKey(e, "smart_order.confidence", "smart_order", "نسبة الثقة", "Confidence", "Confidence score")
	addKey(e, "smart_order.export_csv", "smart_order", "تصدير النتائج (CSV)", "Export Results (CSV)", "Export button")
	addKey(e, "smart_order.continue_review", "smart_order", "مراجعة الطلب والشراء ←", "Review Order & Purchase →", "Continue button")
	addKey(e, "smart_order.budget_warning", "smart_order", "تنبيه: تجاوز الميزانية المحددة", "Warning: Budget Exceeded", "Budget warning alert")

	// --- Inventory & Warehouses ---
	addKey(e, "inventory.warehouses", "inventory", "المستودعات والمخازن", "Warehouses & Stock", "Warehouses page title")
	addKey(e, "inventory.warehouse_name", "inventory", "اسم المستودع", "Warehouse Name", "Warehouse name")
	addKey(e, "inventory.stock_level", "inventory", "مستوى الرصيد", "Stock Level", "Stock level")
	addKey(e, "inventory.min_threshold", "inventory", "الحد الأدنى للإنذار", "Minimum Alert Threshold", "Threshold")
	addKey(e, "inventory.out_of_stock", "inventory", "نفد من المخزون", "Out of Stock", "OOS badge")
	addKey(e, "inventory.low_stock", "inventory", "رصيد منخفض", "Low Stock", "Low stock badge")

	// --- Notifications & Modals ---
	addKey(e, "modal.confirm_title", "modal", "تأكيد الإجراء", "Confirm Action", "Confirm dialog title")
	addKey(e, "modal.confirm_msg", "modal", "هل أنت متأكد من رغبتك في متابعة هذا الإجراء؟", "Are you sure you want to proceed with this action?", "Confirm body text")
	addKey(e, "modal.confirm_yes", "modal", "نعم، متأكد", "Yes, Proceed", "Confirm button")
	addKey(e, "modal.confirm_cancel", "modal", "تراجع", "Dismiss", "Dismiss button")
	addKey(e, "notice.saved_success", "notice", "تم حفظ التعديلات بنجاح.", "Changes saved successfully.", "Success message")
	addKey(e, "notice.delete_success", "notice", "تم الحذف بنجاح.", "Deleted successfully.", "Success message")
	addKey(e, "notice.error_generic", "notice", "حدث خطأ غير متوقع. يرجى المحاولة مرة أخرى.", "An unexpected error occurred. Please try again.", "Error message")
}
