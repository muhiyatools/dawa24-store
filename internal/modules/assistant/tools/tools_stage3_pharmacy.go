package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func pharmacyStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSpendingInsights, key: "spending_insights",
			description: "تحليلات الإنفاق ومؤشرات الأداء المالي للصيدلية: معدل نمو المشتريات مقارنة بالشهر السابق، متوسط قيمة الطلب، المورد الأكبر، وعدد الشحنات الجارية.",
			scopes:      pharmacyScope, permissions: []string{permOrderView},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionReorderSuggestions, key: "reorder_suggestions",
			description: "اقتراحات إعادة الطلب: الأدوية والأصناف المعتاد شراؤها بانتظام والتي قد تحتاج الصيدلية لإعادة تموينها.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف أو كود الدواء.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCatalogSearch, key: "catalog_search",
			description: "بحث متقدم في كتالوج المنتجات المتاحة للشراء: مقارنة الأسعار، الخصومات، الموردين، والوحدات.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view", permOrderView}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف، المادة الفعالة، الكود أو الباركود.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOfferDetails, key: "offer_details",
			description: "تفاصيل عرض ترويجي محدد: شروط الخصم، قائمة المنتجات المشمولة، وفترة السريان.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.offer.view"}, handleKind: handles.KindOffer, handleField: "offer", detail: true,
		}, projectionDetailSchema("offer")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCartSummary, key: "cart_summary",
			description: "محتويات سلة المشتريات الحالية: الأصناف المضافة، الكميات، وإجمالي قيمة السلة.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.cart.use"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionPurchaseRequests, key: "purchase_requests_list",
			description: "قائمة طلبات التسعير (Purchase Requests) المرسلة للموردين وحالتها ومبالغها التقديرية.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view"}, handleKind: handles.KindRequest, handleField: "request",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionPurchaseRequestDetails, key: "purchase_request_details",
			description: "تفاصيل طلب تسعير محدد: الأصناف المطلوبة، الكميات، الأسعار المستهدفة، وعروض وأسعار الموردين والردود.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view"}, handleKind: handles.KindRequest, handleField: "request", detail: true,
		}, projectionDetailSchema("request")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInvoices, key: "invoices_list",
			description: "فواتير المشتريات: أرقام الفواتير، ميعاد الاستحقاق، الإجماليات، وحالة السداد.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindInvoice, handleField: "invoice",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInvoiceDetails, key: "invoice_details",
			description: "تفاصيل فاتورة محددة: البنود التفصيلية، الضريبة، الخصم، والإجمالي النهائي.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindInvoice, handleField: "invoice", detail: true,
		}, projectionDetailSchema("invoice")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionPayments, key: "payments_list",
			description: "سجل المدفوعات والمعاملات المالية المسددة لصالح الفواتير والموردين.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.wallet.view"}, handleKind: handles.KindPayment, handleField: "payment",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSavingProducts, key: "saving_products_list",
			description: "منتجات التوفير والبدائل الاقتصادية المتاحة للأصناف مع مقارنة الأسعار والكميات.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.saving_product.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSmartOrderRuns, key: "smart_order_runs_list",
			description: "سجل عمليات الطلب الذكي (Smart Order) السابقة ونتائج مطابقة الملفات.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.smart_order.view"}, handleKind: handles.KindSmartRun, handleField: "run",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSmartOrderDetails, key: "smart_order_run_details",
			description: "تفاصيل عملية طلب ذكي محددة: حالة كل سطر، أسباب الاستبعاد أو التعثر، والإجمالي التقديري.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.smart_order.view"}, handleKind: handles.KindSmartRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDecisionMemory, key: "decision_memory_search",
			description: "ذاكرة قرارات المطابقة المحفوظة: البحث في تطابق أسماء الأدوية مع الأصناف المعتمدة.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.decision_memory.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف أو مفتاح المطابقة.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionBranchQuota, key: "branch_quota_status",
			description: "حالة كوتة وتخصيص الأصناف المقيدة للفروع: الكمية المستهلكة والرصيد المتبقي.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view", "pharmacy.smart_order.view"},
		}, projectionListSchema(map[string]any{
			"branch":  strProp("مرجع الفرع من branches_list."),
			"product": strProp("مرجع الصنف من الكتالوج."),
		})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSupplierProfile, key: "supplier_profile",
			description: "الملف التعريفي للمورد: تقييم المورد، بيانات الاتصال، عدد الفروع، ونطاق الخدمة.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.supplier.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("اسم المورد أو رقمه التعريفي.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFavourites, key: "favourites_list",
			description: "قائمة المنتجات والأصناف المحفوظة في المفضلة للوصول السريع.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.favorite.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionNotifications, key: "notifications_list",
			description: "الإشعارات والتنبيهات الحديثة لحساب الصيدلية وحالة قراءتها.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.dashboard.view"},
		}, projectionListSchema(map[string]any{"status": enumProp("حالة الإشعار.", "read", "unread")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionBranchProductAvailability, key: "branch_product_availability",
			description: "فحص توافر المنتجات وإمكانية الشراء لفرع محدد: يربط بين الموردين الذين يغطون موقع الفرع، والأصناف المعروضة لديهم، والأرصدة المتاحة وسقف الكوتة والأسعار والخصومات. لسؤال «ما المنتجات المتاحة لفرعي» أو «هل أستطيع طلب صنف كذا لفرع كذا».",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.branch.view", permOrderView}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{
			"branch": strProp("مرجع الفرع كما ورد في نتيجة branches_list أو سياق الجلسة."),
			"search": strProp("اسم الصنف أو الكود أو المادة الفعالة للبحث عن منتج معين."),
		})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrderWorkflowRules, key: "order_workflow_rules",
			description: "شروط وقواعد التوريد والطلب لمورد معين بالنسبة لفرع الصيدلية: الحد الأدنى للطلب، مواعيد التوصيل، طرق الدفع المدعومة، وأوقات إغلاق الطلبات اليومية.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{
			"search": strProp("اسم المورد المراد الاستعلام عن شروطه."),
		})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFinancialObligations, key: "financial_obligations_summary",
			description: "تسوية الالتزامات المالية للصيدلية: ملخص شامل للفواتير المستحقة للدفع، ومبالغ الطلبات قيد التجهيز، ورصيد المحفظة المتاح لمقارنة الالتزامات مع الرصيد.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.wallet.view", permOrderView},
		}, projectionListSchema(nil)),
	}
}
