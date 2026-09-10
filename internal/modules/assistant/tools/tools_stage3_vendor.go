package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func vendorStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSalesInsights, key: "sales_insights",
			description: "تحليلات المبيعات ونمو الإيرادات: نمو المبيعات مقارنة بالـ 30 يوماً السابقة، نسبة إنجاز وتسليم الشحنات، متوسط قيمة الشحنة، وعدد الصيدليات النشطة.",
			scopes:      vendorScope, permissions: []string{permVendorOrder},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInventoryHealth, key: "inventory_health",
			description: "تقييم وتحليل صحة المخزون: القيمة المالية التقديرية للمخزون بالجنيه، عدد الأصناف النافذة، الأصناف الحرجة دون حد الأمان، وإجمالي الوحدات بالمستودعات.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionIncomingQuotes, key: "incoming_quotes_list",
			description: "طلبات عروض الأسعار الواردة من الصيدليات (RFQ): مراجعة طلبات التسعير الجديدة والمعلقة بانتظار رد المورد بالأسعار.",
			scopes:      vendorScope, permissions: []string{permVendorOrder}, handleKind: handles.KindRequest, handleField: "request",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصيدلية أو رقم طلب التسعير.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionIncomingQuoteDetails, key: "incoming_quote_details",
			description: "تفاصيل طلب تسعير وارد محدد: استعراض أصناف الأدوية والكميات المطلوبة والأسعار المستهدفة وملاحظات الصيدلية للرد عليها.",
			scopes:      vendorScope, permissions: []string{permVendorOrder}, handleKind: handles.KindRequest, handleField: "request", detail: true,
		}, projectionDetailSchema("request")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionVariantDetails, key: "variant_details",
			description: "تفاصيل صنف وعبوة دوائية محددة: السعر، الخصم، حالة التفعيل، سقف الكوتة، وإجمالي الرصيد بالمستودعات.",
			scopes:      vendorScope, permissions: []string{"vendor.product.view"}, handleKind: handles.KindVariant, handleField: "variant", detail: true,
		}, projectionDetailSchema("variant")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionStockByWarehouse, key: "stock_by_warehouse",
			description: "أرصدة المخزون موزعة حسب المستودع والصنف: الكمية الحالية وحد التنبيه الأدنى لكل مستودع.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"}, handleKind: handles.KindWarehouse, handleField: "warehouse",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف، الكود، أو اسم المستودع.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionWarehouseTransfers, key: "warehouse_transfers",
			description: "حركة التحويلات المخزنية بين المستودعات: الصنف المنقول، الكمية، المستودع المحول منه وإليه، وحالة النقل.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"}, handleKind: handles.KindTransfer, handleField: "transfer",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionQuotaReport, key: "quota_report",
			description: "تقرير كوتة الأصناف: الحصص المخصصة للفروع، الكميات المستهلكة في الطلبات، والكميات المتبقية.",
			scopes:      vendorScope, permissions: []string{"vendor.quota.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCoverageReport, key: "coverage_report",
			description: "نطاقات تغطية التوصيل الأسبوعية للمورد: المحافظات، المدن، أيام الأسبوع، ونوافذ مواعيد الشحن والتسليم.",
			scopes:      vendorScope, permissions: []string{"vendor.pharmacy_coverage.view", "vendor.coverage.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم المحافظة أو المدينة أو الفرع.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrdersByStatus, key: "orders_by_status",
			description: "توزيع الشحنات وأوامر التوريد حسب الحالة: إجمالي عدد ومبالغ الشحنات (قيد التجهيز، مشحونة، مسلمة، ملغاة).",
			scopes:      vendorScope, permissions: []string{"vendor.order.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionRevenueByPeriod, key: "revenue_by_period",
			description: "إيرادات المبيعات مقسمة زمنياً: الإيراد وعدد الشحنات مجمعة شهرياً لاستكشاف اتجاهات النمو المالي.",
			scopes:      vendorScope, permissions: []string{"vendor.earnings.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionRevenueByProduct, key: "revenue_by_product",
			description: "إيرادات المبيعات حسب الصنف: الكمية المباعة، الإيراد الإجمالي بالجنيه، وعدد الشحنات لكل دواء.",
			scopes:      vendorScope, permissions: []string{"vendor.earnings.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف أو الكود.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCustomers, key: "customers_list",
			description: "قائمة الصيدليات والعملاء المشترين: إجمالي الطلبات، حجم المبيعات بالجنيه، وتاريخ آخر طلب.",
			scopes:      vendorScope, permissions: []string{"vendor.order.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصيدلية أو المنشأة المشترية.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRuns, key: "import_runs_list",
			description: "سجل عمليات استيراد الكتالوج وقوائم الأسعار: الملف، عدد السطور، السطور المدخلة، والمحدثة، والأخطاء.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRunDetails, key: "import_run_details",
			description: "تفاصيل عملية استيراد كتالوج محددة: مراجعة السطور الناجحة، المعلقة للمراجعة، أو التي بها أخطاء.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOffersPerformance, key: "offers_performance",
			description: "أداء وتفاعل العروض الترويجية: المشاهدات، النقرات، ومعدل التحويل إلى طلبات شراء.",
			scopes:      vendorScope, permissions: []string{"vendor.offer.view"}, handleKind: handles.KindOffer, handleField: "offer",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSponsorshipStatus, key: "sponsorship_status",
			description: "باقات الرعاية والإعلانات المميزة: رصيد الرعاية المستخدم والمتبقي وفترة الفعالية.",
			scopes:      vendorScope, permissions: []string{"vendor.offer_package.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionTeam, key: "team_list",
			description: "فريق عمل المنشأة: أعضاء فريق التوريد، الأدوار والصلاحيات، البريد وحالة الحساب.",
			scopes:      vendorScope, permissions: []string{"vendor.team.view"}, handleKind: handles.KindUser, handleField: "member",
		}, projectionListSchema(map[string]any{"search": strProp("اسم العضو أو البريد الإلكتروني.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionReviews, key: "reviews_list",
			description: "تقييمات وآراء الصيدليات في أداء التوريد وجودة الخدمة والالتزام بالمواعيد.",
			scopes:      vendorScope, permissions: []string{"vendor.review.view"}, handleKind: handles.KindReview, handleField: "review",
		}, projectionListSchema(nil)),
	}
}
