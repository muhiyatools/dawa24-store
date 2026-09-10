package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func adminStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrganizations, key: "organizations_list",
			description: "دليل منشآت المنصة: استعراض وتصفية الصيدليات والموردين حسب النوع وحالة النشاط والمدينة.",
			scopes:      adminScope, permissions: []string{"org.organization.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("اسم المنشأة أو رقمها التعريفي.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrganizationDetails, key: "organization_details",
			description: "تفاصيل منشأة محددة: الفروع، المستودعات، بيانات الاتصال، التقييم، وحالة الحساب.",
			scopes:      adminScope, permissions: []string{"org.organization.view"}, handleKind: handles.KindOrgUnit, handleField: "organization", detail: true,
		}, projectionDetailSchema("organization")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionApprovals, key: "approvals_pending",
			description: "طلبات اعتماد المنشآت والوثائق المعلقة: المنشآت بانتظار المراجعة والتدقيق الإداري والترخيص.",
			scopes:      adminScope, permissions: []string{"org.approval.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم المنشأة المطلوب فحصها.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDeletionRequests, key: "deletion_requests",
			description: "طلبات حذف وتعطيل الحسابات: مراجعة طلبات المنشآت الراغبة في حذف حساباتها وأسبابها.",
			scopes:      adminScope, permissions: []string{"org.organization.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionUsers, key: "users_search",
			description: "بحث مستخدمي المنصة: الاسم، البريد الإلكتروني، رقم الهاتف، الدور الوظيفي، وحالة الحساب.",
			scopes:      adminScope, permissions: []string{"identity.user.view"}, handleKind: handles.KindUser, handleField: "member",
		}, projectionListSchema(map[string]any{"search": strProp("الاسم أو البريد الإلكتروني أو الهاتف.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionUserDetails, key: "user_details",
			description: "تفاصيل مستخدم محدد: المنشآت التابع لها، الأدوار والصلاحيات، وتاريخ آخر دخول.",
			scopes:      adminScope, permissions: []string{"identity.user.view"}, handleKind: handles.KindUser, handleField: "member", detail: true,
		}, projectionDetailSchema("member")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionErrorLogs, key: "error_logs_search",
			description: "سجل أخطاء النظام والتطبيقات: مستوى الخطأ، مسار الرابط، فئة الاستثناء، والتاريخ.",
			scopes:      adminScope, permissions: []string{"platform.error_log.view"},
		}, projectionListSchema(map[string]any{"search": strProp("رسالة الخطأ، مسار الرابط، أو فئة الاستثناء.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionAuditLog, key: "audit_log_search",
			description: "سجل الأنشطة والعمليات الإدارية: الإجراءات المنفذة، هوية المنفذ، المنشأة، والكيان المتأثر.",
			scopes:      adminScope, permissions: []string{"platform.activity_log.view"},
		}, projectionListSchema(map[string]any{"search": strProp("نوع الإجراء، نوع الكيان، أو المرجع.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFinance, key: "finance_overview",
			description: "المؤشرات المالية العامة للمنصة: إجمالي أرصدة المحافظ، الفواتير، المدفوعات، وطلبات السحب المعلقة.",
			scopes:      adminScope, permissions: []string{"billing.finance.view", "billing.wallet.read", "billing.invoice.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionWalletTransactions, key: "wallet_transactions_search",
			description: "حركات المحافظ المالية للمنصة: نوع المعاملة، المبلغ، الرصيد بعدها، واسم المنشأة.",
			scopes:      adminScope, permissions: []string{"billing.wallet.read"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم المنشأة، نوع المعاملة، أو المرجع.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSubscriptions, key: "subscriptions_report",
			description: "تقرير الاشتراكات وباقات المنصة: أعداد الاشتراكات، الخطط، وتواريخ التجديد والانتهاء.",
			scopes:      adminScope, permissions: []string{"billing.subscription_plan.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionVisitors, key: "visitors_report",
			description: "تقرير الزيارات والتحليلات: إجمالي الزيارات، المدن الأكثر نشاطاً، والزوار الفريدين.",
			scopes:      adminScope, permissions: []string{"platform.analytics.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionHealth, key: "platform_health",
			description: "مؤشرات الجاهزية وصحة المنظومة: معدل الأخطاء في آخر 24 ساعة، طوابير المهام الخلفية، وحالة التشغيل.",
			scopes:      adminScope, permissions: []string{"platform.dashboard.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionMatchDecisions, key: "match_decisions_search",
			description: "ذاكرة مطابقة الكتالوج: قرارات المطابقة الآلية للأدوية ومستوى الثقة وسجل الاستخدام.",
			scopes:      adminScope, permissions: []string{"catalog.match_decision.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم الصنف أو مفتاح القرار.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInstitutionalGraph, key: "institutional_graph",
			description: "شبكة ربط الأعمال المؤسسية: ارتباطات الفروع بين المشترين والموردين وشبكات التوريد.",
			scopes:      adminScope, permissions: []string{"org.institutional_work.view"},
		}, projectionListSchema(map[string]any{"search": strProp("اسم العمل أو المنشأة.")})),
	}
}
