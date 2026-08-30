package i18n

func loadAdminKeys(e *engine) {
	// Admin Sections & Headers
	addKey(e, "admin.dashboard_title", "admin", "لوحة المعلومات والإحصائيات الرئيسية", "Platform Overview & Realtime Analytics", "Admin dashboard title")
	addKey(e, "admin.total_users", "admin", "إجمالي المستخدمين", "Total Users", "Dashboard metric users")
	addKey(e, "admin.total_organizations", "admin", "إجمالي المنشآت", "Total Organizations", "Dashboard metric orgs")
	addKey(e, "admin.total_orders", "admin", "إجمالي الطلبات", "Total Orders", "Dashboard metric orders")
	addKey(e, "admin.total_revenue", "admin", "إجمالي حجم التداول", "Total Trade Volume", "Dashboard metric revenue")
	addKey(e, "admin.jobs_unknown_company", "admin", "منشأة معتمدة", "Verified organization", "Fallback company name on the admin jobs list")

	// Translations Manager
	addKey(e, "admin.translations_title", "admin", "اللغات وإدارة الترجمة والتدويل (i18n)", "Localization & Translations Manager", "Translations manager page title")
	addKey(e, "admin.translations_sub", "admin", "إدارة كافة نصوص ومصطلحات المنصة باللغتين العربية والإنجليزية، وتعديل الترجمات ديناميكياً مع التطبيق الفوري.", "Manage all platform text and terms in Arabic & English with real-time runtime override support.", "Translations page subtitle")
	addKey(e, "admin.translations_sync", "admin", "مزامنة المفاتيح الافتراضية", "Sync Default Keys", "Sync default keys button")
	addKey(e, "admin.translations_total_keys", "admin", "إجمالي المفاتيح", "Total Keys", "Stats total keys")
	addKey(e, "admin.translations_custom_overrides", "admin", "الترجمات المخصصة", "Custom Overrides", "Stats custom overrides")
	addKey(e, "admin.translations_namespaces", "admin", "الأقسام والتصنيفات", "Namespaces", "Stats namespaces")
	addKey(e, "admin.translations_all_namespaces", "admin", "كل التصنيفات (All Namespaces)", "All Namespaces", "Namespace filter all")
	addKey(e, "admin.translations_key_col", "admin", "مفتاح الترجمة (Key)", "Translation Key", "Table key header")
	addKey(e, "admin.translations_ns_col", "admin", "القسم (Namespace)", "Namespace", "Table namespace header")
	addKey(e, "admin.translations_ar_col", "admin", "النص العربي (Arabic)", "Arabic Text", "Table AR header")
	addKey(e, "admin.translations_en_col", "admin", "النص الإنجليزي (English)", "English Text", "Table EN header")
	addKey(e, "admin.translations_type_col", "admin", "النوع", "Type", "Table type header")
	addKey(e, "admin.translations_default_badge", "admin", "افتراضي", "Default", "Default badge")
	addKey(e, "admin.translations_custom_badge", "admin", "مخصص", "Custom", "Custom badge")
	addKey(e, "admin.translations_edit_modal", "admin", "تعديل نص الترجمة", "Edit Translation", "Edit modal title")
	addKey(e, "admin.translations_reset_btn", "admin", "استعادة الافتراضي", "Reset to Default", "Reset button")
	addKey(e, "admin.translations_reset_confirm", "admin", "هل أنت متأكد من استعادة الترجمة الافتراضية لهذا المفتاح؟", "Are you sure you want to revert this key to its code default?", "Reset confirmation")
	addKey(e, "admin.translations_updated_success", "admin", "تم تحديث نص الترجمة وتطبيقه في النظام فوراً.", "Translation updated and applied immediately.", "Update success notice")
	addKey(e, "admin.translations_reset_success", "admin", "تمت استعادة الترجمة الافتراضية بنجاح.", "Reverted to default translation successfully.", "Reset success notice")
	addKey(e, "admin.translations_sync_success", "admin", "تمت مزامنة كافة مفاتيح النظام مع قاعدة البيانات بنجاح.", "Synced all system keys to database successfully.", "Sync success notice")

	// Chat History Audit
	addKey(e, "admin.chat_history_title", "admin", "سجلات محادثات المساعد الذكي كبسولة AI", "AI Capsule Assistant Audit & Chat Logs", "Admin chat history page title")
	addKey(e, "admin.chat_history_sub", "admin", "مراقبة وتدقيق كافة جلسات ومحادثات الذكاء الاصطناعي مع المستخدمين (صيدليات وموردين) واستهلاك الرموز وتفاصيل الحوارات.", "Audit all user AI conversations across pharmacies and vendors with token usage and full dialog inspection.", "Admin chat history subtitle")

	// Decision Memory
	addKey(e, "admin.decision_memory.toggle_error", "admin", "حدث خطأ أثناء تحديث حالة ذاكرة القرارات.", "Error updating decision memory state.", "Toggle error")
	addKey(e, "admin.decision_memory.state_enabled", "admin", "تفعيل", "Enable", "Enabled state word")
	addKey(e, "admin.decision_memory.state_disabled", "admin", "إيقاف", "Disable", "Disabled state word")
	addKey(e, "admin.decision_memory.toggle_success", "admin", "تم %s نظام ذاكرة القرارات بنجاح على مستوى المنصة بالكامل.", "Decision memory system was %sd successfully across the platform.", "Toggle success")
	addKey(e, "admin.decision_memory.invalid_id", "admin", "معرف القرار غير صالح.", "Invalid decision ID.", "Validation error")
	addKey(e, "admin.decision_memory.delete_error", "admin", "حدث خطأ أثناء حذف القرار.", "Error deleting decision.", "Delete error")
	addKey(e, "admin.decision_memory.delete_success", "admin", "تم حذف القرار من ذاكرة المطابقة بنجاح.", "Decision deleted from match memory successfully.", "Delete success")
	addKey(e, "admin.decision_memory.clear_error", "admin", "حدث خطأ أثناء مسح ذاكرة القرارات.", "Error clearing decision memory.", "Clear error")
	addKey(e, "admin.decision_memory.clear_success", "admin", "تم مسح كافة قرارات الذاكرة بنجاح.", "All memory decisions cleared successfully.", "Clear success")
	addKey(e, "decision_memory.item_and_drug_required", "catalog", "يرجى إدخال اسم الصنف واختيار الدواء المطابق من الكتالوج.", "Please enter item name and select matched medicine from catalog.", "Validation error")
	addKey(e, "decision_memory.customer_manual_reason", "catalog", "قرار يدوي مضاف من الصيدلية", "Manual decision added by pharmacy", "Default reason")
	addKey(e, "decision_memory.vendor_manual_reason", "catalog", "قرار يدوي مضاف من المورد", "Manual decision added by vendor", "Default reason")
	addKey(e, "decision_memory.save_error", "catalog", "حدث خطأ أثناء حفظ القرار في الذاكرة.", "Error saving decision to memory.", "Save error")
	addKey(e, "decision_memory.customer_saved_success", "catalog", "تم حفظ قرار المطابقة بنجاح في ذاكرة الصيدلية.", "Match decision saved successfully in pharmacy memory.", "Save success")
	addKey(e, "decision_memory.vendor_saved_success", "catalog", "تم حفظ قرار المطابقة بنجاح في ذاكرة المورد.", "Match decision saved successfully in vendor memory.", "Save success")
	addKey(e, "decision_memory.org_cleared_success", "catalog", "تم مسح ذاكرة قرارات المطابقة بنجاح.", "Match decision memory cleared successfully.", "Clear success")

	// --- Admin CMS Content ---
	addKey(e, "admin.content.service_unavailable", "admin", "خدمة المحتوى غير متاحة حالياً.", "Content service is currently unavailable.", "Service error")
	addKey(e, "admin.content.key_required", "admin", "يرجى تحديد المفتاح التعريفي للكتلة.", "Please specify the content block key.", "Validation error")
	addKey(e, "admin.content.saved_success", "admin", "تم حفظ كتلة المحتوى بنجاح وتحديثها في المنصة.", "Content block saved and updated successfully on the platform.", "Success notice")
	addKey(e, "admin.content.invalid_id", "admin", "معرف الكتلة غير صالح.", "Invalid content block ID.", "Validation error")
	addKey(e, "admin.content.status_updated_success", "admin", "تم تحديث حالة تفعيل الكتلة بنجاح.", "Content block status updated successfully.", "Success notice")
	addKey(e, "admin.content.deleted_success", "admin", "تم حذف كتلة المحتوى بنجاح.", "Content block deleted successfully.", "Success notice")
}
