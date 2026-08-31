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

	// --- Platform Audit Trail ---
	addKey(e, "admin.audit.severity_info", "admin", "عادي (Info)", "Info", "Audit severity info")
	addKey(e, "admin.audit.severity_warning", "admin", "متوسط (Warning)", "Warning", "Audit severity warning")
	addKey(e, "admin.audit.severity_critical", "admin", "حرج (Critical)", "Critical", "Audit severity critical")

	addKey(e, "admin.audit.mod_orgs", "admin", "المنشآت", "Organizations", "Audit module organizations")
	addKey(e, "admin.audit.mod_users", "admin", "المستخدمين", "Users", "Audit module users")
	addKey(e, "admin.audit.mod_security", "admin", "الأمان والصلاحيات", "Security & Permissions", "Audit module security")
	addKey(e, "admin.audit.mod_catalog", "admin", "الكتالوج", "Catalogue", "Audit module catalog")
	addKey(e, "admin.audit.mod_offers", "admin", "عروض الموردين", "Supplier Offers", "Audit module offers")
	addKey(e, "admin.audit.mod_orders", "admin", "أوامر التوريد", "Supply Orders", "Audit module orders")
	addKey(e, "admin.audit.mod_institutional", "admin", "الهيكل المؤسسي", "Institutional Structure", "Audit module institutional")
	addKey(e, "admin.audit.mod_system", "admin", "النظام", "System", "Audit module system")

	addKey(e, "admin.audit.action_org_registered", "admin", "تسجيل منشأة جديدة", "Register New Organization", "Audit action")
	addKey(e, "admin.audit.title_org_registered", "admin", "طلب تسجيل منشأة جديدة", "New Organization Registration Request", "Audit title")
	addKey(e, "admin.audit.desc_org_registered", "admin", "تم تقديم ملف ترخيص وسجل تجاري لمنشأة دوائية جديدة", "A license file and commercial register were submitted for a new pharmaceutical organization", "Audit desc")

	addKey(e, "admin.audit.action_org_status", "admin", "تحديث حالة اعتماد المنشأة", "Update Organization Approval Status", "Audit action")
	addKey(e, "admin.audit.title_org_approved", "admin", "اعتماد أو ترخيص منشأة", "Approve or License Organization", "Audit title")
	addKey(e, "admin.audit.desc_org_approved", "admin", "تم التحقق من الوثائق والموافقة على حساب المنشأة", "Documents verified and organization account approved", "Audit desc")

	addKey(e, "admin.audit.action_org_rejected", "admin", "رفض اعتماد المنشأة", "Reject Organization Approval", "Audit action")
	addKey(e, "admin.audit.title_org_rejected", "admin", "رفض اعتماد منشأة", "Reject Organization Approval", "Audit title")
	addKey(e, "admin.audit.desc_org_rejected", "admin", "تم رفض ملف المنشأة بسبب عدم استيفاء التراخيص", "Organization application rejected due to incomplete licensing", "Audit desc")

	addKey(e, "admin.audit.action_org_suspended", "admin", "إيقاف المنشأة مؤقتاً", "Suspend Organization Temporarily", "Audit action")
	addKey(e, "admin.audit.title_org_suspended", "admin", "إيقاف حساب منشأة", "Suspend Organization Account", "Audit title")
	addKey(e, "admin.audit.desc_org_suspended", "admin", "تم تعليق حساب المنشأة مؤقتاً لمخالفة اللوائح", "Organization account temporarily suspended for regulatory violation", "Audit desc")

	addKey(e, "admin.audit.action_user_registered", "admin", "تسجيل حساب مستخدم جديد", "Register New User Account", "Audit action")
	addKey(e, "admin.audit.title_user_registered", "admin", "إنشاء حساب مستخدم", "Create User Account", "Audit title")
	addKey(e, "admin.audit.desc_user_registered", "admin", "تم تسجيل عضو أو صيدلي جديد في النظام", "A new member or pharmacist was registered in the system", "Audit desc")

	addKey(e, "admin.audit.action_user_status", "admin", "تغيير حالة حساب المستخدم", "Change User Account Status", "Audit action")
	addKey(e, "admin.audit.title_user_status", "admin", "تعديل حالة الحساب", "Modify Account Status", "Audit title")
	addKey(e, "admin.audit.desc_user_status", "admin", "تحديث حالة التفعيل أو الإيقاف لحساب المستخدم", "User account activation or suspension status updated", "Audit desc")

	addKey(e, "admin.audit.action_user_role", "admin", "تعيين دور وصلاحية للمستخدم", "Assign Role and Permission to User", "Audit action")
	addKey(e, "admin.audit.title_user_role", "admin", "إسناد صلاحية أمنية", "Assign Security Permission", "Audit title")
	addKey(e, "admin.audit.desc_user_role", "admin", "تعديل رتبة وصلاحيات المستخدم داخل المنصة", "User platform rank and permissions modified", "Audit desc")

	addKey(e, "admin.audit.action_user_mfa_reset", "admin", "إعادة ضبط التحقق الثنائي (MFA)", "Reset Two-Factor Authentication (MFA)", "Audit action")
	addKey(e, "admin.audit.title_user_mfa_reset", "admin", "إعادة ضبط أمني (MFA)", "Security Reset (MFA)", "Audit title")
	addKey(e, "admin.audit.desc_user_mfa_reset", "admin", "إعادة ضبط مفاتيح المصادقة الثنائية لحساب المستخدم", "Two-factor authentication keys reset for user account", "Audit desc")

	addKey(e, "admin.audit.action_product_created", "admin", "إضافة صنف دوائي جديد", "Add New Medicine", "Audit action")
	addKey(e, "admin.audit.title_product_created", "admin", "إضافة دواء للكتالوج", "Add Product to Catalogue", "Audit title")
	addKey(e, "admin.audit.desc_product_created", "admin", "إدراج صنف دوائي ومستحضر معتمد في الكتالوج الموحد", "Certified pharmaceutical item added to the unified catalogue", "Audit desc")

	addKey(e, "admin.audit.action_product_updated", "admin", "تعديل بيانات الصنف الدوائي", "Edit Product Details", "Audit action")
	addKey(e, "admin.audit.title_product_updated", "admin", "تحديث بيانات دواء", "Update Medicine Details", "Audit title")
	addKey(e, "admin.audit.desc_product_updated", "admin", "تعديل الأسعار أو المادة الفعالة أو بيانات الصنف", "Prices, active ingredients, or product details updated", "Audit desc")

	addKey(e, "admin.audit.action_product_deleted", "admin", "حذف صنف من الكتالوج", "Delete Product from Catalogue", "Audit action")
	addKey(e, "admin.audit.title_product_deleted", "admin", "حذف صنف دوائي", "Delete Medicine", "Audit title")
	addKey(e, "admin.audit.desc_product_deleted", "admin", "إلغاء أو حذف صنف دوائي من الكتالوج المعتمد", "Pharmaceutical item removed from certified catalogue", "Audit desc")

	addKey(e, "admin.audit.action_variant_created", "admin", "إضافة عرض توريد جديد", "Add New Supplier Offer", "Audit action")
	addKey(e, "admin.audit.title_variant_created", "admin", "إضافة عرض سعر دوائي", "Add Medicine Price Offer", "Audit title")
	addKey(e, "admin.audit.desc_variant_created", "admin", "طرح عرض أسعار وتوريد جديد لصنف معتمد", "New supply and pricing offer listed for approved product", "Audit desc")

	addKey(e, "admin.audit.action_order_created", "admin", "إنشاء طلب توريد جديد", "Create New Supply Order", "Audit action")
	addKey(e, "admin.audit.title_order_created", "admin", "إنشاء أمر توريد", "Create Supply Order", "Audit title")
	addKey(e, "admin.audit.desc_order_created", "admin", "تم تقديم أمر توريد دوائي جديد من صيدلية", "A new pharmaceutical supply order was submitted by a pharmacy", "Audit desc")

	addKey(e, "admin.audit.action_order_status", "admin", "تحديث حالة أمر التوريد", "Update Supply Order Status", "Audit action")
	addKey(e, "admin.audit.title_order_status", "admin", "تحديث حالة الشحن/التوريد", "Update Shipping/Supply Status", "Audit title")
	addKey(e, "admin.audit.desc_order_status", "admin", "تغيير حالة الطلب الدوائي بين التجهيز والتوصيل والاستلام", "Order status changed between processing, shipping, and delivery", "Audit desc")

	addKey(e, "admin.audit.action_inst_created", "admin", "إضافة تصنيف هيكل مؤسسي", "Add Institutional Structure", "Audit action")
	addKey(e, "admin.audit.title_inst_created", "admin", "إضافة هيكل مؤسسي جديد", "Add New Institutional Structure", "Audit title")
	addKey(e, "admin.audit.desc_inst_created", "admin", "إنشاء تصنيف هيكلي جديد للمنشآت والمستودعات", "New institutional structure category created for organizations and warehouses", "Audit desc")

	addKey(e, "admin.audit.action_inst_updated", "admin", "تعديل تصنيف هيكل مؤسسي", "Edit Institutional Structure", "Audit action")
	addKey(e, "admin.audit.title_inst_updated", "admin", "تعديل هيكل مؤسسي", "Edit Institutional Structure", "Audit title")
	addKey(e, "admin.audit.desc_inst_updated", "admin", "تحديث بيانات تصنيف هيكلي أو باقة التسعير", "Institutional structure details or pricing tier updated", "Audit desc")

	addKey(e, "admin.audit.desc_default", "admin", "عملية إدارية مسجلة بالنظام", "Administrative action logged in system", "Default audit desc")

	addKey(e, "admin.audit.entity_org", "admin", "منشأة / شركة", "Organization / Company", "Audit entity organization")
	addKey(e, "admin.audit.entity_user", "admin", "مستخدم", "User", "Audit entity user")
	addKey(e, "admin.audit.entity_product", "admin", "صنف دوائي", "Medicine Product", "Audit entity product")
	addKey(e, "admin.audit.entity_variant", "admin", "عرض توريد", "Supply Offer", "Audit entity variant")
	addKey(e, "admin.audit.entity_order", "admin", "أمر توريد", "Supply Order", "Audit entity order")
	addKey(e, "admin.audit.entity_branch", "admin", "فرع مستودع / صيدلية", "Warehouse / Pharmacy Branch", "Audit entity branch")
	addKey(e, "admin.audit.entity_inst", "admin", "هيكل مؤسسي", "Institutional Structure", "Audit entity institutional")

	addKey(e, "admin.audit.default_actor", "admin", "النظام / System", "System", "Default audit actor")
	addKey(e, "admin.audit.default_org", "admin", "المنصة الرئيسية", "Main Platform", "Default audit organization")

	// --- Temporary Warehouses & Price Lists ---
	addKey(e, "admin.temp_wh.invalid_id", "admin", "معرف المستودع غير صحيح.", "Invalid temporary warehouse ID.", "Validation error")
	addKey(e, "admin.temp_wh.not_found", "admin", "المستودع غير موجود.", "Temporary warehouse not found.", "Not found error")
	addKey(e, "admin.temp_wh.mapping_updated_msg", "admin", "تم تحديث أعمدة المستودع [%s] بنجاح (إجمالي %d صنف).", "Warehouse columns [%s] updated successfully (total %d items).", "Success message")
	addKey(e, "admin.temp_wh.mapping_applied_msg", "admin", "تم تحديث وتأكيد أعمدة المستودع [%s] بنجاح.", "Warehouse columns [%s] updated and confirmed successfully.", "Success notice")
	addKey(e, "admin.temp_wh.deleted_success", "admin", "تم حذف المستودع وكافة أصنافه بنجاح.", "Temporary warehouse and all items deleted successfully.", "Success notice")
	addKey(e, "admin.temp_wh.unarchived_success", "admin", "تم تفعيل المستودع وإتاحته بالخصومات بنجاح.", "Warehouse unarchived and enabled for discounts.", "Success notice")
	addKey(e, "admin.temp_wh.manual_archive_reason", "admin", "أرشفة يدوية من لوحة المشرف", "Manual archive from admin portal", "Archive reason")
	addKey(e, "admin.temp_wh.archived_success", "admin", "تم أرشفة المستودع بنجاح.", "Warehouse archived successfully.", "Success notice")

	// --- Categories & Reference ---
	addKey(e, "admin.categories.service_unavailable", "catalog", "خدمة الكتالوج غير متاحة.", "Catalogue service is currently unavailable.", "Service error")
	addKey(e, "admin.categories.name_required", "catalog", "يرجى كتابة اسم فئة المنتج بالعربية أو الإنجليزية.", "Please enter category name in Arabic or English.", "Validation error")
	addKey(e, "admin.categories.created_success", "catalog", "تم إنشاء فئة المنتجات بنجاح.", "Product category created successfully.", "Success notice")
	addKey(e, "admin.categories.invalid_id", "catalog", "معرف الفئة غير صالح.", "Invalid category ID.", "Validation error")
	addKey(e, "admin.categories.not_found", "catalog", "فئة المنتجات غير موجودة.", "Product category not found.", "Not found error")
	addKey(e, "admin.categories.updated_success", "catalog", "تم تحديث بيانات فئة المنتجات بنجاح.", "Product category updated successfully.", "Success notice")
	addKey(e, "admin.categories.status_updated_success", "catalog", "تم تحديث حالة الفئة بنجاح.", "Category status updated successfully.", "Success notice")
	addKey(e, "admin.categories.cannot_delete_has_products_format", "catalog", "لا يمكن حذف هذه الفئة لوجود %d صنف معتمد مرتبط بها. يرجى نقل الأصناف لفئة أخرى أولاً.", "Cannot delete this category because %d certified products are linked to it. Please move items to another category first.", "Delete restriction error")
	addKey(e, "admin.categories.deleted_success", "catalog", "تم حذف فئة المنتجات بنجاح.", "Product category deleted successfully.", "Success notice")

	addKey(e, "admin.reference.social_media_title", "settings", "قنوات التواصل الاجتماعي للمنصة", "Platform Social Media Channels", "Page title")
	addKey(e, "admin.reference.social_channel", "settings", "قناة تواصل", "Social Channel", "Entity singular")
	addKey(e, "admin.reference.api_gateway_title", "developers", "بوابة الواجهات البرمجية (API Gateway)", "API Gateway", "Gateway title")
	addKey(e, "admin.reference.api_gateway_desc_format", "developers", "البيئة: %s | الرابط: %s", "Environment: %s | URL: %s", "Gateway desc")
	addKey(e, "admin.reference.api_gateway_timeout_format", "developers", "المهلة: %d ثوانٍ", "Timeout: %d seconds", "Gateway timeout")
	addKey(e, "admin.reference.ai_gateway_title", "developers", "بوابة الذكاء الاصطناعي (AI Gateway)", "AI Gateway", "Gateway title")
	addKey(e, "admin.reference.ai_gateway_desc_format", "developers", "الرابط: %s", "URL: %s", "Gateway desc")
	addKey(e, "admin.reference.ai_gateway_max_tokens_format", "developers", "الرموز القصوى: %d", "Max tokens: %d", "Gateway tokens")
	addKey(e, "admin.reference.api_integrations_title", "developers", "بوابات الربط والواجهات البرمجية (APIs)", "API Integrations & Gateways", "Page title")
	addKey(e, "admin.reference.api_integration", "developers", "واجهة ربط", "API Integration", "Entity singular")

	// --- Platform Roles & Permissions ---
	addKey(e, "admin.roles.title", "admin", "الأدوار والصلاحيات", "Roles & Permissions", "Page title")
	addKey(e, "admin.roles.subtitle", "admin", "أدوار مشرفي المنصة: ما الذي يراه كل مشرف وما الذي يستطيع تغييره.", "Platform moderator roles: what each staff member sees and can modify.", "Page subtitle")
	addKey(e, "admin.roles.badge_full_access", "admin", "كامل الصلاحيات", "Full Access", "Badge label")
	addKey(e, "admin.roles.badge_staff", "admin", "لوحة الإدارة", "Admin Portal", "Badge label")
	addKey(e, "admin.roles.badge_regular", "admin", "حساب عادي", "Standard Account", "Badge label")
	addKey(e, "admin.roles.id_service_unavailable", "identity", "خدمة الهوية غير متوفرة.", "Identity service is currently unavailable.", "Service error")
	addKey(e, "admin.roles.not_found", "identity", "الدور غير موجود.", "Role not found.", "Not found error")
	addKey(e, "admin.roles.created_success", "identity", "تم إنشاء الدور. حدّد صلاحياته الآن.", "Role created. Configure its permissions now.", "Success notice")
	addKey(e, "admin.roles.permissions_saved_success", "identity", "تم حفظ صلاحيات الدور.", "Role permissions saved successfully.", "Success notice")
	addKey(e, "admin.roles.deleted_success", "identity", "تم حذف الدور.", "Role deleted successfully.", "Success notice")
	addKey(e, "admin.roles.invalid_request", "identity", "طلب غير صالح.", "Invalid request.", "Validation error")
	addKey(e, "admin.roles.cannot_change_own_role", "identity", "لا يمكنك تغيير دور حسابك بنفسك.", "You cannot change your own account role.", "Permission error")
	addKey(e, "admin.roles.user_role_updated_success", "identity", "تم تحديث دور المستخدم وإنهاء جلساته.", "User role updated and active sessions terminated.", "Success notice")
	addKey(e, "admin.roles.invalid_form", "identity", "بيانات النموذج غير صالحة.", "Invalid form data.", "Validation error")
	addKey(e, "admin.roles.staff_created_success", "identity", "تم إنشاء حساب المشرف وإسناد دوره.", "Staff account created and role assigned successfully.", "Success notice")

	// --- Product Images Import ---
	addKey(e, "admin.image_import.file_too_large", "catalog", "حجم الملف كبير جداً أو تعذر قراءته.", "File size is too large or could not be read.", "Upload error")
	addKey(e, "admin.image_import.select_valid_file", "catalog", "يرجى اختيار ملف Excel أو CSV صالح.", "Please select a valid Excel or CSV file.", "Validation error")
	addKey(e, "admin.image_import.unsupported_format", "catalog", "صيغة الملف غير مدعومة. يرجى رفع ملف Excel (.xlsx أو .xls) أو ملف نصي (.csv).", "Unsupported file format. Please upload an Excel (.xlsx or .xls) or CSV (.csv) file.", "Format error")
	addKey(e, "admin.image_import.file_empty", "catalog", "الملف المرفوع فارغ أو تالف.", "The uploaded file is empty or corrupted.", "Validation error")
	addKey(e, "admin.image_import.header_only", "catalog", "الملف يحتوي على صف العناوين فقط ولا يوجد به أي بيانات.", "File contains headers only and has no data rows.", "Validation error")
	addKey(e, "admin.image_import.session_not_found", "catalog", "جلسة المعالجة منتهية الصلاحية أو غير موجودة.", "Processing session expired or not found.", "Not found error")
	addKey(e, "admin.image_import.catalog_service_unavailable", "catalog", "خدمة الكتالوج معطلة.", "Catalogue service is disabled.", "Service error")
	addKey(e, "admin.image_import.select_columns_required", "catalog", "يرجى تحديد عمود كود الصنف (SKU) وعمود رابط الصورة (Image URL) للبدء بربط الصور.", "Please select the SKU column and the Image URL column to begin linking images.", "Validation error")
	addKey(e, "admin.image_import.session_deleted_success", "catalog", "تم حذف جلسة استيراد الصور بنجاح.", "Image import session deleted successfully.", "Success notice")

	// --- Geography & Locations ---
	addKey(e, "admin.geo.city_name_ar_required", "admin", "اسم المدينة بالعربية مطلوب.", "City Arabic name is required.", "Validation error")
	addKey(e, "admin.geo.city_created_success", "admin", "تم حفظ وإضافة المدينة الفرعية بنجاح في قاعدة البيانات.", "City/district added and saved successfully.", "Success notice")
	addKey(e, "admin.geo.city_invalid_id", "admin", "معرف المدينة غير صالح.", "Invalid city ID.", "Validation error")
	addKey(e, "admin.geo.city_status_updated_success", "admin", "تم تحديث حالة تفعيل المدينة بنجاح.", "City status updated successfully.", "Success notice")
	addKey(e, "admin.geo.city_updated_success", "admin", "تم تحديث وحفظ بيانات المدينة بنجاح.", "City data updated and saved successfully.", "Success notice")
	addKey(e, "admin.geo.gov_name_ar_required", "admin", "اسم المحافظة بالعربية مطلوب.", "Governorate Arabic name is required.", "Validation error")
	addKey(e, "admin.geo.gov_created_success", "admin", "تم حفظ وإضافة المحافظة الرئيسية بنجاح.", "Main governorate added and saved successfully.", "Success notice")
	addKey(e, "admin.geo.gov_invalid_id", "admin", "معرف المحافظة غير صالح.", "Invalid governorate ID.", "Validation error")
	addKey(e, "admin.geo.gov_updated_success", "admin", "تم تحديث بيانات المحافظة بنجاح.", "Governorate data updated successfully.", "Success notice")
	addKey(e, "admin.geo.gov_status_updated_success", "admin", "تم تحديث حالة تفعيل المحافظة بنجاح.", "Governorate status updated successfully.", "Success notice")

	// --- Platform Settings ---
	addKey(e, "admin.settings.invalid_feature_key", "admin", "مفتاح الميزة غير صالح.", "Invalid feature key.", "Validation error")
	addKey(e, "admin.settings.feature_save_failed", "admin", "فشل حفظ حالة الميزة.", "Failed to save feature state.", "Save error")
	addKey(e, "admin.settings.feature_enabled_success", "admin", "تم تفعيل الميزة بنجاح.", "Feature enabled successfully.", "Success notice")
	addKey(e, "admin.settings.feature_disabled_success", "admin", "تم إيقاف الميزة بنجاح.", "Feature disabled successfully.", "Success notice")
	addKey(e, "admin.settings.service_unavailable", "admin", "خدمة الإعدادات غير متاحة حالياً.", "Settings service is currently unavailable.", "Service error")
	addKey(e, "admin.settings.invalid_email", "admin", "البريد الإلكتروني غير صالح.", "Invalid email address.", "Validation error")
	addKey(e, "admin.settings.invalid_commission", "admin", "نسبة العمولة يجب أن تكون بين 0 و 100.", "Commission rate must be between 0 and 100.", "Validation error")
	addKey(e, "admin.settings.saved_general_success", "admin", "تم حفظ البيانات العامة بنجاح.", "General settings saved successfully.", "Success notice")
	addKey(e, "admin.settings.saved_ai_success", "admin", "تم حفظ إعدادات الذكاء الاصطناعي بنجاح.", "AI settings saved successfully.", "Success notice")
	addKey(e, "admin.settings.saved_system_prompt_success", "admin", "تم حفظ وتطبيق التوجيه العام لذكاء كبسولة الاصطناعي بنجاح.", "AI system prompt saved and applied successfully.", "Success notice")

	// --- Platform Finance & Billing ---
	addKey(e, "admin.finance.invalid_deposit_id", "billing", "معرف طلب الإيداع غير صالح.", "Invalid deposit request ID.", "Validation error")
	addKey(e, "admin.finance.service_unavailable", "billing", "خدمة المدفوعات والمحفظة غير متاحة.", "Billing and wallet service is unavailable.", "Service error")
	addKey(e, "admin.finance.deposit_approved_success_format", "billing", "تم اعتماد طلب الإيداع بنجاح وإضافة %s ج.م إلى محفظة المستخدم (معاملة #TX-%d).", "Deposit request approved successfully and %s EGP added to user wallet (Tx #TX-%d).", "Success notice")
	addKey(e, "admin.finance.default_deposit_rejection_reason", "billing", "تم رفض طلب الإيداع لعدم تطابق بيانات أو إشعار التحويل البنكي.", "Deposit rejected due to unmatched bank transfer receipt details.", "Default rejection reason")
	addKey(e, "admin.finance.deposit_rejected_success", "billing", "تم رفض طلب الإيداع وإشعار صاحب الحساب بحيثيات الرفض.", "Deposit rejected and user notified with reason.", "Success notice")
	addKey(e, "admin.finance.invalid_wallet_id", "billing", "معرف المحفظة غير صحيح.", "Invalid wallet ID.", "Validation error")
	addKey(e, "admin.finance.invalid_amount", "billing", "يرجى تحديد مبلغ صالح أكبر من الصفر.", "Please specify a valid amount greater than zero.", "Validation error")
	addKey(e, "admin.finance.default_adjustment_reason", "billing", "تعديل إداري مباشر للرصيد", "Direct manual administrative adjustment", "Default adjustment reason")
	addKey(e, "admin.finance.wallet_adjusted_success", "billing", "تم قيد وتحديث رصيد المحفظة بنجاح وتسجيل المعاملة في السجل.", "Wallet balance updated and transaction logged successfully.", "Success notice")
	addKey(e, "admin.finance.plan_types_title", "billing", "أنواع وتصنيفات خطط الاشتراك", "Subscription Plan Types & Categories", "Page title")
	addKey(e, "admin.finance.plan_type", "billing", "نوع خطة", "Plan Type", "Entity singular")
	addKey(e, "admin.finance.plan_features_title", "billing", "ميزات ومحددات باقات الاشتراك", "Subscription Plan Features & Limits", "Page title")
	addKey(e, "admin.finance.plan_feature", "billing", "ميزة", "Feature", "Entity singular")

	// --- Platform Subscription Plans ---
	addKey(e, "admin.billing.gw_plan_dev_desc", "billing", "باقة التطوير والتشغيل المجانية", "Free development & operational tier", "Plan description")
	addKey(e, "admin.billing.gw_plan_yalla_desc", "billing", "باقة الأعمال والنمو المتوسطة", "Mid-tier business and growth plan", "Plan description")
	addKey(e, "admin.billing.gw_plan_max_desc", "billing", "باقة المؤسسات والشركات الكبرى", "Enterprise tier for large organizations", "Plan description")
	addKey(e, "admin.billing.service_unavailable", "billing", "الخدمة غير متاحة حالياً.", "Service is currently unavailable.", "Service error")
	addKey(e, "admin.billing.plan_created_success", "billing", "تمت إضافة وحفظ باقة الاشتراك الجديدة بنجاح.", "New subscription plan added successfully.", "Success notice")
	addKey(e, "admin.billing.invalid_plan_id", "billing", "معرف الخطة غير صالح.", "Invalid plan ID.", "Validation error")
	addKey(e, "admin.billing.plan_updated_success", "billing", "تم حفظ وتحديث بيانات باقة الاشتراك بنجاح.", "Subscription plan updated successfully.", "Success notice")
	addKey(e, "admin.billing.plan_status_toggled_success", "billing", "تم تحديث حالة تفعيل باقة الاشتراك بنجاح.", "Subscription plan status updated successfully.", "Success notice")
	addKey(e, "admin.billing.plan_sort_updated_success", "billing", "تم تعيين وتحديث الترتيب لعرض الباقة بالصفحة الرئيسية.", "Plan display order updated successfully.", "Success notice")
	addKey(e, "admin.billing.plan_deleted_success", "billing", "تم حذف باقة الاشتراك بنجاح.", "Subscription plan deleted successfully.", "Success notice")

	// --- Platform Commerce & Policies ---
	addKey(e, "admin.commerce.invalid_offer_id", "promo", "معرف العرض غير صالح.", "Invalid offer ID.", "Validation error")
	addKey(e, "admin.commerce.approved_by_admin", "promo", "تم الاعتماد من الإدارة", "Approved by administration", "Approval status comment")
	addKey(e, "admin.commerce.offer_approved_success", "promo", "تم اعتماد وتفعيل العرض الخاص وباقة الأدوية بنجاح.", "Special bundle offer approved and activated successfully.", "Success notice")
	addKey(e, "admin.commerce.rejected_by_admin", "promo", "تم رفض العرض من الإدارة", "Rejected by administration", "Rejection status comment")
	addKey(e, "admin.commerce.offer_rejected_success", "promo", "تم رفض العرض الخاص وحفظ الملاحظات.", "Special offer rejected and notes recorded.", "Success notice")
	addKey(e, "admin.commerce.offer_status_toggled_success", "promo", "تم تحديث حالة تفعيل العرض بنجاح.", "Offer status updated successfully.", "Success notice")
	addKey(e, "admin.commerce.policy_service_unavailable", "admin", "خدمة السياسات غير متاحة.", "Policy service is unavailable.", "Service error")
	addKey(e, "admin.commerce.policy_saved_success", "admin", "تم حفظ إصدار السياسة بنجاح.", "Policy version saved successfully.", "Success notice")
	addKey(e, "admin.commerce.invalid_policy_id", "admin", "معرف السياسة غير صالح.", "Invalid policy ID.", "Validation error")
	addKey(e, "admin.commerce.policy_published_success", "admin", "تم نشر الإصدار وتفعيله للجمهور.", "Policy version published and made active for public.", "Success notice")

	// --- Platform Translation Management ---
	addKey(e, "admin.translations.key_and_text_required", "admin", "يرجى ملء مفتاح الترجمة والنص العربي أو الإنجليزي.", "Please fill in the translation key and Arabic or English text.", "Validation error")
	addKey(e, "admin.translations.admin_service_unavailable", "admin", "خدمة الإدارة العامة غير متوفرة.", "Platform administration service is unavailable.", "Service error")
	addKey(e, "admin.translations.updated_success", "admin", "تم تحديث الترجمة وتطبيقها بنجاح في كامل المنصة.", "Translation updated and applied platform-wide.", "Success notice")
	addKey(e, "admin.translations.invalid_key", "admin", "مفتاح الترجمة غير صالح.", "Invalid translation key.", "Validation error")
	addKey(e, "admin.translations.reset_success", "admin", "تمت استعادة الترجمة الافتراضية بنجاح.", "Default translation restored successfully.", "Success notice")
	addKey(e, "admin.translations.synced_success", "admin", "تمت مزامنة كافة مفاتيح النظام الافتراضية مع قاعدة البيانات بنجاح.", "All default system translation keys synchronized successfully.", "Success notice")

	// --- Manufacturers & Brands ---
	addKey(e, "admin.brands.name_required", "catalog", "يرجى كتابة اسم الشركة المصنعة بالعربية أو الإنجليزية.", "Please enter manufacturer/brand name in Arabic or English.", "Validation error")
	addKey(e, "admin.brands.created_success", "catalog", "تمت إضافة الشركة المصنعة بنجاح.", "Manufacturer/brand added successfully.", "Success notice")
	addKey(e, "admin.brands.invalid_id", "catalog", "معرف الشركة غير صالح.", "Invalid manufacturer/brand ID.", "Validation error")
	addKey(e, "admin.brands.not_found", "catalog", "الشركة المصنعة غير موجودة.", "Manufacturer/brand not found.", "Not found error")
	addKey(e, "admin.brands.updated_success", "catalog", "تم تحديث بيانات الشركة المصنعة بنجاح.", "Manufacturer/brand updated successfully.", "Success notice")
	addKey(e, "admin.brands.deleted_success", "catalog", "تم حذف الشركة المصنعة بنجاح.", "Manufacturer/brand deleted successfully.", "Success notice")

	// --- Developer & Diagnostics ---
	addKey(e, "admin.dev.admin_service_unavailable", "developers", "خدمة إدارة المنظومة غير متاحة.", "Platform administration service is unavailable.", "Service error")
	addKey(e, "admin.dev.empty_sql_query", "developers", "استعلام SQL فارغ.", "Empty SQL query.", "Validation error")
	addKey(e, "admin.dev.saved_ai_settings_success", "developers", "تم حفظ إعدادات بوابة الذكاء الاصطناعي بنجاح.", "AI Gateway settings saved successfully.", "Success notice")
	addKey(e, "admin.dev.connection_failed_format", "developers", "تعذر الاتصال بـ %s (%v)", "Could not connect to %s (%v)", "Connection error")
	addKey(e, "admin.dev.unauthorized_format", "developers", "خطأ في المصادقة (401 Unauthorized): كلمة مرور أو بيانات اعتماد المدير غير صحيحة لبوابة %s. يرجى كتابة كلمة المرور المحددة في ADMIN_PASSWORD الخاصة بالبوابة والضغط على حفظ.", "Authentication error (401 Unauthorized): invalid admin credentials for %s. Please configure ADMIN_PASSWORD correctly.", "Auth error")
	addKey(e, "admin.dev.connection_healthy_format", "developers", "الاتصال بـ %s نشط بنجاح — تم جلب %d باقات ذكاء اصطناعي حية من البوابة", "Connection to %s is active — retrieved %d live AI plans from gateway", "Success notice")
	addKey(e, "admin.dev.invalid_log_id", "developers", "معرف السجل غير صالح.", "Invalid log ID.", "Validation error")
	addKey(e, "admin.dev.error_status_updated_success", "developers", "تم تحديث حالة الخطأ بنجاح.", "Error log status updated successfully.", "Success notice")

	// --- Document Management & Verification ---
	addKey(e, "admin.docs.select_valid_org", "attachments", "يرجى اختيار منشأة صالحة من القائمة.", "Please select a valid organization from the list.", "Validation error")
	addKey(e, "admin.docs.title_required", "attachments", "عنوان المستند المطلوب إلزامي.", "Requested document title is required.", "Validation error")
	addKey(e, "admin.docs.service_unavailable", "attachments", "خدمة المستندات غير متاحة.", "Document service is unavailable.", "Service error")
	addKey(e, "admin.docs.request_created_success", "attachments", "تم إصدار طلب المستند الرسمي للمنشأة مع التنبيه والمهلة المحددة بنجاح.", "Document request issued to organization with deadline successfully.", "Success notice")
	addKey(e, "admin.docs.invalid_request_id", "attachments", "معرف الطلب غير صالح.", "Invalid request ID.", "Validation error")
	addKey(e, "admin.docs.request_cancelled_success", "attachments", "تم إلغاء طلب المستند.", "Document request cancelled.", "Success notice")
	addKey(e, "admin.docs.invalid_doc_id", "attachments", "معرف المستند غير صالح.", "Invalid document ID.", "Validation error")
	addKey(e, "admin.docs.verified_success", "attachments", "تم اعتماد وتوثيق المستند وتحديث ملف المنشأة بنجاح.", "Document verified and organization profile updated successfully.", "Success notice")
	addKey(e, "admin.docs.rejected_success", "attachments", "تم رفض المستند وحفظ الملاحظات.", "Document rejected and notes recorded.", "Success notice")

	// --- Products Management ---
	addKey(e, "admin.products.service_unavailable", "catalog", "خدمة المنتجات غير متاحة حالياً.", "Products service is currently unavailable.", "Service error")
	addKey(e, "admin.products.name_required", "catalog", "يرجى كتابة اسم الصنف الدوائي بالعربية أو الإنجليزية.", "Please enter medicine product name in Arabic or English.", "Validation error")
	addKey(e, "admin.products.created_success", "catalog", "تمت إضافة الصنف الدوائي الأساسي بنجاح إلى الدليل المعتمد.", "Master pharmaceutical product added to certified catalog successfully.", "Success notice")
	addKey(e, "admin.products.invalid_id", "catalog", "معرف الدواء غير صالح.", "Invalid medicine product ID.", "Validation error")
	addKey(e, "admin.products.not_found", "catalog", "الصنف الدوائي غير موجود.", "Medicine product not found.", "Not found error")
	addKey(e, "admin.products.updated_success", "catalog", "تم تحديث بيانات الصنف الدوائي بنجاح.", "Medicine product updated successfully.", "Success notice")
	addKey(e, "admin.products.deleted_success", "catalog", "تم حذف الصنف الدوائي من الكتالوج المعتمد.", "Medicine product deleted from certified catalog.", "Success notice")

	// --- Platform Payment Methods ---
	addKey(e, "admin.pm.id_required", "billing", "المعرف الفريد لوسيلة الدفع مطلوب.", "Payment method ID is required.", "Validation error")
	addKey(e, "admin.pm.name_ar_required", "billing", "اسم وسيلة الدفع بالعربية مطلوب.", "Payment method Arabic name is required.", "Validation error")
	addKey(e, "admin.pm.saved_success", "billing", "تم حفظ وتحديث وسيلة وقناة الدفع بنجاح.", "Payment method and channel saved successfully.", "Success notice")
	addKey(e, "admin.pm.toggle_failed", "billing", "فشل تحديث حالة وسيلة الدفع.", "Failed to update payment method status.", "Error notice")
	addKey(e, "admin.pm.disabled_notice", "billing", "تم تعطيل وسيلة الدفع مؤقتاً.", "Payment method disabled temporarily.", "Notice")
	addKey(e, "admin.pm.enabled_notice", "billing", "تم تفعيل وسيلة الدفع بنجاح.", "Payment method enabled successfully.", "Success notice")
	addKey(e, "admin.pm.delete_failed", "billing", "فشل حذف وسيلة الدفع.", "Failed to delete payment method.", "Error notice")
	addKey(e, "admin.pm.deleted_success", "billing", "تم حذف وسيلة الدفع بنجاح.", "Payment method deleted successfully.", "Success notice")

	// --- Trash & Soft Delete Management ---
	addKey(e, "admin.trash.restored_success", "admin", "تم استرجاع السجل بنجاح.", "Record restored successfully.", "Success notice")
	addKey(e, "admin.trash.purged_success", "admin", "تم الحذف النهائي للسجل.", "Record permanently purged.", "Success notice")

	// --- Institutional Structure & Classification ---
	addKey(e, "admin.inst.service_unavailable", "org", "خدمة الهيكل المؤسسي غير متاحة.", "Institutional structure service is unavailable.", "Service error")
	addKey(e, "admin.inst.title_required", "org", "يرجى كتابة اسم التصنيف المؤسسي.", "Institutional category name is required.", "Validation error")
	addKey(e, "admin.inst.created_success", "org", "تمت إضافة تصنيف الهيكل المؤسسي والاتصالات المسموح بها بنجاح.", "Institutional work category and connections created successfully.", "Success notice")
	addKey(e, "admin.inst.invalid_id", "org", "معرف التصنيف غير صالح.", "Invalid category ID.", "Validation error")
	addKey(e, "admin.inst.updated_success", "org", "تم تحديث بيانات التصنيف المؤسسي والاتصالات بنجاح.", "Institutional category and connections updated successfully.", "Success notice")
	addKey(e, "admin.inst.deleted_success", "org", "تم حذف التصنيف المؤسسي.", "Institutional category deleted.", "Success notice")
	addKey(e, "admin.inst.status_updated_success", "org", "تم تحديث حالة تفعيل التصنيف.", "Category status updated.", "Success notice")

	// --- Platform User Management ---
	addKey(e, "admin.users.action_success", "identity", "تم تنفيذ الإجراء بنجاح.", "Action executed successfully.", "Success notice")
	addKey(e, "admin.users.deletion_approved_reason", "identity", "تمت الموافقة من إدارة المنصة", "Approved by platform administration", "Review notes")
	addKey(e, "admin.users.deletion_approved_success", "identity", "تمت الموافقة على حذف الحساب وتعطيله بنجاح.", "Account deletion approved and account disabled successfully.", "Success notice")
	addKey(e, "admin.users.deletion_rejected_reason", "identity", "تم رفض طلب الحذف من الإدارة", "Deletion request rejected by administration", "Review notes")
	addKey(e, "admin.users.deletion_rejected_success", "identity", "تم رفض طلب حذف الحساب بنجاح.", "Account deletion request rejected successfully.", "Success notice")

	// --- Platform Approvals & Organization Verification ---
	addKey(e, "admin.approvals.invalid_org_id", "org", "معرف المنشأة غير صالح.", "Invalid organization ID.", "Validation error")
	addKey(e, "admin.approvals.org_service_unavailable", "org", "خدمة المؤسسات غير متاحة.", "Organization service is unavailable.", "Service error")
	addKey(e, "admin.approvals.approved_and_verified_success", "org", "تم اعتماد وتفعيل ترخيص المنشأة وتوثيق المستندات المرفقة بنجاح.", "Organization license approved and attached documents verified successfully.", "Success notice")
	addKey(e, "admin.approvals.rejected_success", "org", "تم رفض طلب المنشأة وحفظ سبب الرفض.", "Organization application rejected and reason recorded.", "Success notice")
	addKey(e, "admin.approvals.suspended_notice", "org", "تم تعليق حساب المنشأة مؤقتاً.", "Organization account suspended temporarily.", "Notice")
	addKey(e, "admin.approvals.status_updated_success", "org", "تم تحديث حالة المنشأة وتفعيل صلاحياتها بنجاح.", "Organization status updated and permissions activated successfully.", "Success notice")
	addKey(e, "admin.approvals.account_activated_success", "org", "تم اعتماد وتفعيل حساب المنشأة بنجاح.", "Organization account approved and activated successfully.", "Success notice")

	// --- Master Catalog Imports ---
	addKey(e, "admin.import.forbidden", "catalog", "صلاحيات غير كافية لتنفيذ هذه العملية.", "Insufficient permissions to perform this operation.", "Permission error")
	addKey(e, "admin.import.service_unavailable", "catalog", "خدمة الكتالوج غير متاحة حالياً. يرجى المحاولة بعد قليل أو التواصل مع الدعم الفني.", "Catalogue service is currently unavailable. Please try again later or contact support.", "Service error")
	addKey(e, "admin.import.session_expired", "catalog", "لم يتم العثور على جلسة الاستيراد المطلوبة أو انتهت صلاحيتها. يرجى رفع الملف من جديد.", "Requested import session was not found or expired. Please upload file again.", "Not found error")
	addKey(e, "admin.import.confirm_destructive_required", "catalog", "يجب تأكيد أرشفة الكتالوج الحالي قبل تنفيذ هذه الطريقة.", "You must confirm archiving current catalogue before executing this method.", "Validation error")
	addKey(e, "admin.import.committed_success_format", "catalog", "تم حفظ %d صنف في الكتالوج المعتمد (%d جديد، %d محدَّث).", "Saved %d products in certified catalogue (%d new, %d updated).", "Success notice")
	addKey(e, "admin.import.cancelled_success", "catalog", "تم إلغاء عملية الاستيراد ولم يتم حفظ أي صنف.", "Import cancelled; no products were saved.", "Success notice")
	addKey(e, "admin.import.upload_max_bytes_format", "catalog", "تعذرت قراءة الملف المرفوع. الحد الأقصى لحجم الملف هو %d ميجابايت.", "Could not read uploaded file. Maximum file size is %d MB.", "Upload error")
	addKey(e, "admin.import.no_file_selected", "catalog", "لم يتم اختيار أي ملف. يرجى اختيار ملف Excel (.xlsx) أو CSV ثم الضغط على «قراءة الملف».", "No file selected. Please select an Excel (.xlsx) or CSV file and click 'Read File'.", "Upload validation")
	addKey(e, "admin.import.read_content_failed", "catalog", "تعذرت قراءة محتوى الملف المرفوع. يرجى إعادة المحاولة.", "Could not read uploaded file content. Please try again.", "Upload error")
	addKey(e, "admin.import.file_too_large_format", "catalog", "حجم الملف يتجاوز الحد الأقصى المسموح به (%d ميجابايت).", "File size exceeds maximum allowed size (%d MB).", "Upload validation")
	addKey(e, "admin.import.parse_settings_failed", "catalog", "تعذرت قراءة الإعدادات المرسلة.", "Could not parse submitted settings.", "Validation error")
	addKey(e, "admin.import.preview_updated_success", "catalog", "تم تحديث المعاينة بالإعدادات الجديدة.", "Preview updated with new settings.", "Success notice")

	// --- Catalog & Variants ---
	addKey(e, "admin.catalog.variant_status_updated_success", "catalog", "تم تحديث حالة صنف المورد بنجاح.", "Supplier variant status updated successfully.", "Success notice")
	addKey(e, "admin.catalog.delete_all_failed_format", "catalog", "حدث خطأ أثناء حذف منتجات الكتالوج: %s", "Failed to delete catalogue products: %s", "Error notice")
	addKey(e, "admin.catalog.deleted_all_success_format", "catalog", "تم حذف %d صنفاً من الكتالوج المركزي الأساسي بنجاح.", "Successfully deleted %d products from primary central catalogue.", "Success notice")

	// --- Admin Dashboard ---
	addKey(e, "admin.dashboard.commission_5pct", "admin", "5% عمولة", "5% Commission", "Commission label")

	// --- Site Settings & Branding ---
	addKey(e, "admin.branding.site_saved_success", "admin", "تم حفظ وتحديث إعدادات الموقع بنجاح.", "Site settings saved and updated successfully.", "Success notice")
	addKey(e, "admin.branding.branding_saved_success", "admin", "تم حفظ وتطبيق الهوية البصرية بنجاح.", "Branding and visual identity saved successfully.", "Success notice")

	// --- Admin Organizations & Weekly Coverage ---
	addKey(e, "admin.org.not_found", "org", "المنشأة غير موجودة.", "Organization not found.", "Not found error")
	addKey(e, "admin.org.coverage_created_success", "org", "تم إضافة جدول التغطية الأسبوعية بنجاح.", "Weekly coverage schedule added successfully.", "Success notice")
	addKey(e, "admin.org.coverage_status_updated_success", "org", "تم تحديث حالة جدول التغطية بنجاح.", "Coverage schedule status updated successfully.", "Success notice")
	addKey(e, "admin.org.coverage_deleted_success", "org", "تم حذف جدول التغطية بنجاح.", "Coverage schedule deleted successfully.", "Success notice")

	// --- Vendor Storefront ---
	addKey(e, "vendor.storefront.title_ar_required", "vendor", "عنوان القسم بالعربية مطلوب.", "Arabic section title is required.", "Validation error")
	addKey(e, "vendor.storefront.section_created_success", "vendor", "تم إضافة القسم المميز بنجاح.", "Featured section added successfully.", "Success notice")
	addKey(e, "vendor.storefront.section_edit_failed", "vendor", "تعذر تعديل القسم.", "Failed to edit section.", "Error notice")
	addKey(e, "vendor.storefront.section_updated_success", "vendor", "تم تحديث بيانات القسم بنجاح.", "Section updated successfully.", "Success notice")
	addKey(e, "vendor.storefront.section_delete_failed", "vendor", "تعذر حذف القسم.", "Failed to delete section.", "Error notice")
	addKey(e, "vendor.storefront.section_deleted_success", "vendor", "تم حذف القسم بنجاح.", "Section deleted successfully.", "Success notice")
	addKey(e, "vendor.storefront.section_toggle_failed", "vendor", "تعذر تغيير حالة القسم.", "Failed to change section status.", "Error notice")
	addKey(e, "vendor.storefront.section_not_found_or_unauthorized", "vendor", "القسم غير موجود أو غير مصرح بتعديله.", "Section not found or unauthorized to edit.", "Error notice")
	addKey(e, "vendor.storefront.section_status_updated", "vendor", "تم تحديث حالة القسم.", "Section status updated.", "Success notice")

	// --- Subscriptions ---
	addKey(e, "sub.default_plan_name", "billing", "الباقة الأساسية الافتراضية", "Default Basic Plan", "Plan name")
	addKey(e, "sub.status_active", "billing", "ساري وفعال", "Active & Valid", "Subscription status")
	addKey(e, "sub.auto_renews", "billing", "تجديد تلقائي مستمر", "Continuous auto-renewal", "Subscription expiry")

	// --- Issue Reporting ---
	addKey(e, "issues.service_unavailable", "workflow", "خدمة البلاغات غير متوفرة.", "Issues reporting service is unavailable.", "Service error")
	addKey(e, "issues.submitted_success", "workflow", "تم إرسال البلاغ بنجاح، سيقوم فريق الدعم بمتابعته.", "Issue reported successfully; support team will follow up.", "Success notice")

	// --- Contact Messages & Requests ---
	addKey(e, "admin.messages.status_updated_success", "admin", "تم تحديث حالة الرسالة بنجاح.", "Message status updated successfully.", "Success notice")
	addKey(e, "admin.messages.deleted_success", "admin", "تم حذف الرسالة بنجاح.", "Message deleted successfully.", "Success notice")
	addKey(e, "requests.sent_success", "workflow", "تم إرسال الطلب.", "Request sent successfully.", "Success notice")

	// --- Admin Temp Warehouse Upload ---
	addKey(e, "admin.temp_warehouse.default_name_prefix", "admin", "مستودع ", "Warehouse ", "Default warehouse prefix")
	addKey(e, "admin.temp_warehouse.open_failed_format", "admin", "فشل فتح الملف: %s", "Failed to open file: %s", "Error format")
	addKey(e, "admin.temp_warehouse.read_failed_format", "admin", "فشل قراءة محتوى الملف: %s", "Failed to read file content: %s", "Error format")
	addKey(e, "admin.temp_warehouse.insufficient_rows", "admin", "الملف لا يحتوي على صفوف بيانات كافية أو تعذر تحليله", "File does not contain sufficient data rows or could not be parsed", "Error notice")
	addKey(e, "admin.temp_warehouse.create_record_failed_format", "admin", "فشل إنشاء سجل المستودع: %s", "Failed to create warehouse record: %s", "Error format")
	addKey(e, "admin.temp_warehouse.upload_limit_exceeded", "admin", "حجم الملفات المرفوعة يتجاوز الحد الأقصى المسموح (500 ميجابايت).", "Uploaded files size exceeds maximum limit (500 MB).", "Error notice")
	addKey(e, "admin.temp_warehouse.upload_too_large", "admin", "حجم الملفات المرفوعة كبير جداً.", "Uploaded files are too large.", "Error notice")
	addKey(e, "admin.temp_warehouse.select_files", "admin", "يرجى اختيار ملف أو أكثر للرفع.", "Please select one or more files to upload.", "Error notice")
	addKey(e, "admin.temp_warehouse.select_files_notice", "admin", "يرجى اختيار ملف أو مجموعة ملفات للرفع.", "Please select a file or group of files to upload.", "Error notice")
	addKey(e, "admin.temp_warehouse.upload_success_message", "admin", "تم بنجاح رفع ومعالجة %d من أصل %d ملف مستودع بإجمالي %d صنف متاح في خصومات ومقارنات السوق.", "Successfully uploaded and processed %d of %d warehouse files with %d items available in market comparison and discounts.", "Upload success message")
	addKey(e, "admin.temp_warehouse.all_files_failed_prefix", "admin", "فشل معالجة كافة الملفات المرفوعة: ", "Failed to process all uploaded files: ", "Error prefix")
	addKey(e, "admin.temp_warehouse.success_summary", "admin", "تم بنجاح رفع ومعالجة %d من أصل %d ملف مستودع بإجمالي %d صنف.", "Successfully uploaded and processed %d of %d warehouse files with %d items.", "Success summary")
	addKey(e, "admin.temp_warehouse.fail_count_suffix", "admin", " (فشل %d ملف)", " (%d files failed)", "Fail suffix")

	// --- AI Gateway Dashboard Texts ---
	addKey(e, "ai.unspecified", "ai", "غير محدد", "Unspecified", "Unspecified value")
	addKey(e, "ai.no_operations", "ai", "لا توجد عمليات في هذه الفترة", "No operations in this period", "No AI operations")
	addKey(e, "ai.at_least_cost_format", "ai", "%.4f$ على الأقل", "At least $%.4f", "At least cost format")
	addKey(e, "ai.no_ceiling_published", "ai", "لا يوجد سقف منشور", "No ceiling published", "No ceiling published")
	addKey(e, "ai.periodic_renewal", "ai", "تجديد شهري دوري", "Periodic monthly renewal", "Periodic monthly renewal")
	addKey(e, "ai.today_renewing", "ai", "اليوم (جاري التجديد)", "Today (renewing)", "Today renewing")
	addKey(e, "ai.remaining_days_format", "ai", "متبقي %d يوماً", "%d days remaining", "Days remaining")
	addKey(e, "ai.remaining_one_day", "ai", "متبقي يوم واحد", "1 day remaining", "One day remaining")
	addKey(e, "ai.remaining_hours_format", "ai", "متبقي %d ساعة", "%d hours remaining", "Hours remaining")
	addKey(e, "ai.remaining_one_hour", "ai", "متبقي ساعة واحدة", "1 hour remaining", "One hour remaining")
	addKey(e, "ai.remaining_mins_format", "ai", "متبقي %d دقيقة", "%d minutes remaining", "Mins remaining")
	addKey(e, "ai.remaining_less_min", "ai", "أقل من دقيقة", "Less than a minute", "Less than a minute")
	addKey(e, "ai.no_usage_data", "ai", "لا تتوفر بيانات استهلاك من البوابة حالياً", "No consumption data available from gateway currently", "No usage data")
	addKey(e, "ai.consumed_no_ceiling_format", "ai", "%.2f$ مستهلك — لا يوجد سقف منشور لهذه الباقة", "$%.2f consumed — no ceiling published for this plan", "Consumed no ceiling format")
	addKey(e, "ai.consumed_of_total_format", "ai", "%.2f$ من %.2f$", "$%.2f of $%.2f", "Consumed of total format")
}
