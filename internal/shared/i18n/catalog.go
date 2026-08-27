package i18n

func loadCatalogDefaults(e *engine) {
	loadCommonAndAuthKeys(e)
	loadNavKeys(e)
	loadModulesKeys(e)
	loadAdminKeys(e)
	loadCommerceAndIngestKeys(e)
	loadFrontendUIKeys(e)
	loadBillingAndHRKeys(e)
	loadCompareAndPromoKeys(e)
	loadDevelopersAndSystemKeys(e)
	loadOrgBranchesGeoKeys(e)
	loadChatAndAttachmentsKeys(e)
	loadErrorsAndValidationKeys(e)
}

func addKey(e *engine, key, namespace, textAR, textEN, desc string) {
	e.defaults[key] = KeyEntry{
		Key:         key,
		Namespace:   namespace,
		TextAR:      textAR,
		TextEN:      textEN,
		Description: desc,
		IsCustom:    false,
	}
}

func loadCommonAndAuthKeys(e *engine) {
	// Common actions
	addKey(e, "common.save", "common", "حفظ", "Save", "General save button")
	addKey(e, "common.save_changes", "common", "حفظ التعديلات", "Save Changes", "Save changes button")
	addKey(e, "common.cancel", "common", "إلغاء", "Cancel", "Cancel button")
	addKey(e, "common.delete", "common", "حذف", "Delete", "Delete action")
	addKey(e, "common.edit", "common", "تعديل", "Edit", "Edit action")
	addKey(e, "common.create", "common", "إنشاء", "Create", "Create action")
	addKey(e, "common.add", "common", "إضافة", "Add", "Add action")
	addKey(e, "common.search", "common", "بحث", "Search", "Search button")
	addKey(e, "common.search_placeholder", "common", "بحث بالاسم أو الكود أو الرقم...", "Search by name, code or number...", "Search input placeholder")
	addKey(e, "common.filter", "common", "تصفية", "Filter", "Filter action")
	addKey(e, "common.all", "common", "الكل", "All", "All tab or option")
	addKey(e, "common.actions", "common", "الإجراءات", "Actions", "Table actions column")
	addKey(e, "common.status", "common", "الحالة", "Status", "Status column or field")
	addKey(e, "common.active", "common", "نشط", "Active", "Active status")
	addKey(e, "common.inactive", "common", "غير نشط", "Inactive", "Inactive status")
	addKey(e, "common.pending", "common", "قيد المراجعة", "Pending", "Pending status")
	addKey(e, "common.approved", "common", "معتمد", "Approved", "Approved status")
	addKey(e, "common.rejected", "common", "مرفوض", "Rejected", "Rejected status")
	addKey(e, "common.completed", "common", "مكتمل", "Completed", "Completed status")
	addKey(e, "common.failed", "common", "فشل", "Failed", "Failed status")
	addKey(e, "common.loading", "common", "جاري التحميل...", "Loading...", "Loading state")
	addKey(e, "common.success", "common", "تم بنجاح", "Success", "Success notice title")
	addKey(e, "common.error", "common", "حدث خطأ", "Error", "Error notice title")
	addKey(e, "common.warning", "common", "تنبيه", "Warning", "Warning notice title")
	addKey(e, "common.no_data", "common", "لا توجد بيانات متاحة", "No data available", "Empty table message")
	addKey(e, "common.no_results", "common", "لم يتم العثور على أي نتائج", "No results found", "Empty search results")
	addKey(e, "common.back", "common", "رجوع", "Back", "Back button")
	addKey(e, "common.next", "common", "التالي", "Next", "Next button")
	addKey(e, "common.previous", "common", "السابق", "Previous", "Previous button")
	addKey(e, "common.confirm", "common", "تأكيد", "Confirm", "Confirm action")
	addKey(e, "common.close", "common", "إغلاق", "Close", "Close button")
	addKey(e, "common.export", "common", "تصدير", "Export", "Export action")
	addKey(e, "common.import", "common", "استيراد", "Import", "Import action")
	addKey(e, "common.download", "common", "تنزيل", "Download", "Download action")
	addKey(e, "common.upload", "common", "رفع ملف", "Upload", "Upload action")
	addKey(e, "common.refresh", "common", "تحديث", "Refresh", "Refresh action")
	addKey(e, "common.details", "common", "التفاصيل", "Details", "Details action")
	addKey(e, "common.currency_egp", "common", "ج.م", "EGP", "Egyptian Pound abbreviation")
	addKey(e, "common.rows_per_page", "common", "%d صفوف", "%d rows", "Pagination limit selector")
	addKey(e, "common.showing_results", "common", "عرض %d من أصل %d نتيجة", "Showing %d of %d results", "Pagination summary")
	addKey(e, "common.theme_dark", "common", "الوضع الليلي", "Dark Mode", "Theme toggle dark")
	addKey(e, "common.theme_light", "common", "الوضع الفاتح", "Light Mode", "Theme toggle light")
	addKey(e, "common.lang_ar", "common", "العربية", "Arabic", "Language Arabic")
	addKey(e, "common.lang_en", "common", "الإنجليزية", "English", "Language English")

	// Authentication
	addKey(e, "auth.login", "auth", "تسجيل الدخول", "Sign In", "Login heading/button")
	addKey(e, "auth.logout", "auth", "تسجيل الخروج", "Sign Out", "Logout button")
	addKey(e, "auth.register", "auth", "إنشاء حساب جديد", "Create Account", "Register link/button")
	addKey(e, "auth.email", "auth", "البريد الإلكتروني", "Email Address", "Email input label")
	addKey(e, "auth.password", "auth", "كلمة المرور", "Password", "Password input label")
	addKey(e, "auth.forgot_password", "auth", "نسيت كلمة المرور؟", "Forgot Password?", "Forgot password link")
	addKey(e, "auth.remember_me", "auth", "تذكرني على هذا الجهاز", "Remember Me", "Remember me checkbox")
	addKey(e, "auth.invalid_credentials", "auth", "بيانات الدخول غير صحيحة. يرجى التحقق والمحاولة مجدداً.", "Invalid credentials. Please check and try again.", "Auth failure message")
	addKey(e, "auth.session_expired", "auth", "انتهت صلاحية الجلسة، يرجى تسجيل الدخول مجدداً.", "Session expired, please sign in again.", "Session expired message")
	addKey(e, "auth.access_denied", "auth", "ليس لديك الصلاحية الكافية للوصول إلى هذه الصفحة.", "You do not have permission to access this page.", "Access denied message")

	// Validations & Errors
	addKey(e, "validation.required", "validation", "هذا الحقل مطلوب.", "This field is required.", "Required field error")
	addKey(e, "validation.invalid_email", "validation", "يرجى إدخال عنوان بريد إلكتروني صالح.", "Please enter a valid email address.", "Invalid email error")
	addKey(e, "validation.invalid_number", "validation", "يرجى إدخال رقم صحيح صالح.", "Please enter a valid number.", "Invalid number error")
	addKey(e, "validation.file_too_large", "validation", "حجم الملف المرفوع أكبر من الحد المسموح به.", "Uploaded file is larger than the allowed size.", "File size error")
	addKey(e, "validation.invalid_file_format", "validation", "صيغة الملف غير مدعومة.", "Unsupported file format.", "Invalid file format error")
	addKey(e, "errors.not_found", "errors", "الصفحة أو العنصر المطلوب غير موجود.", "The requested page or item was not found.", "404 not found")
	addKey(e, "errors.server_error", "errors", "حدث خطأ غير متوقع في الخادم. يرجى المحاولة لاحقاً.", "An unexpected server error occurred. Please try again later.", "500 server error")
	addKey(e, "errors.unauthorized", "errors", "يرجى تسجيل الدخول أولاً للمتابعة.", "Please sign in first to continue.", "401 unauthorized")
}
