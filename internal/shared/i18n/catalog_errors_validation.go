package i18n

func loadErrorsAndValidationKeys(e *engine) {
	// --- Validation Messages ---
	addKey(e, "validation.field_required", "validation", "هذا الحقل إلزامي ولا يمكن تركه فارغاً.", "This field is required and cannot be empty.", "Required validation")
	addKey(e, "validation.invalid_phone", "validation", "يرجى إدخال رقم هاتف صحيح (مثال: 01012345678).", "Please enter a valid mobile phone number.", "Invalid phone validation")
	addKey(e, "validation.invalid_national_id", "validation", "الرقم القومي المصري يجب أن يتكون من 14 رقماً صحيحاً.", "National ID must be exactly 14 digits.", "Invalid national ID")
	addKey(e, "validation.password_too_short", "validation", "كلمة المرور يجب ألا تقل عن 8 أحرف وأرقام.", "Password must be at least 8 characters long.", "Password short validation")
	addKey(e, "validation.password_mismatch", "validation", "كلمتا المرور غير متطابقتين.", "Passwords do not match.", "Password mismatch")
	addKey(e, "validation.invalid_coords", "validation", "يرجى تحديد إحداثيات جغرافية صحيحة (خط الطول وخط العرض).", "Please provide valid GPS coordinates.", "Invalid GPS coordinates")
	addKey(e, "validation.invalid_url", "validation", "يرجى إدخال رابط إنترنت صحيح يبدأ بـ https://", "Please enter a valid URL starting with https://", "Invalid URL")
	addKey(e, "validation.min_order_exceeded", "validation", "قيمة الطلب أقل من الحد الأدنى للشراء المحدد من المورد (%s ج.م).", "Order total is below the minimum required by supplier (%s EGP).", "Min order validation")
	addKey(e, "validation.stock_insufficient", "validation", "الكمية المطلوبة من الصنف (%s) تتجاوز الرصيد المتاح حالياً بالمخزن (%d).", "Requested quantity for (%s) exceeds available stock (%d).", "Insufficient stock error")
	addKey(e, "validation.out_of_coverage", "validation", "عنوان التوصيل المحدد يقع خارج نطاق تغطية هذا المورد.", "The selected delivery address is outside supplier's coverage zone.", "Out of coverage validation")

	// --- HTTP & System Errors ---
	addKey(e, "errors.400_bad_request", "errors", "طلب غير صالح، يرجى التحقق من صحة المدخلات.", "Bad request, please verify input values.", "400 error")
	addKey(e, "errors.401_unauthorized", "errors", "يرجى تسجيل الدخول للوصول إلى هذه الصفحة.", "Unauthorized, please sign in first.", "401 error")
	addKey(e, "errors.403_forbidden", "errors", "غير مصرح لك بتنفيذ هذا الإجراء أو الوصول لهذه الصفحة.", "Forbidden, you lack permission for this action.", "403 error")
	addKey(e, "errors.404_not_found", "errors", "العنصر أو المسار المطلوب غير موجود.", "Not found, the requested resource does not exist.", "404 error")
	addKey(e, "errors.409_conflict", "errors", "تعارض في البيانات، قد يكون السجل مسجلاً مسبقاً في النظام.", "Conflict, this record might already exist.", "409 error")
	addKey(e, "errors.429_rate_limited", "errors", "تم تجاوز الحد المسموح به من الطلبات، يرجى الانتظار قليلاً.", "Too many requests, please slow down.", "429 error")
	addKey(e, "errors.500_internal", "errors", "حدث خطأ غير متوقع في الخادم. تم تسجيل الخطأ للفحص.", "Internal server error. The incident has been logged.", "500 error")
	addKey(e, "errors.database_error", "errors", "تعذر الاتصال بقاعدة البيانات حالياً.", "Database connection error.", "Database error")
	addKey(e, "errors.ai_gateway_error", "errors", "تعذر استجابة خادم الذكاء الاصطناعي حالياً، يرجى المحاولة لاحقاً.", "AI Gateway is currently unavailable, please try again later.", "AI Gateway error")
}
