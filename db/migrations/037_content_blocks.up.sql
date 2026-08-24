-- 037_content_blocks
--
-- CMS content blocks (legacy what_in_contents). These back the /about,
-- /how-it-works, /faq and /help pages so their copy is editable without a
-- redeploy. platform_admin.privacy_policies already exists (migration 021) and
-- backs /privacy and /terms.

BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.content_blocks (
    id         BIGSERIAL PRIMARY KEY,
    key        VARCHAR(64) NOT NULL,
    title      JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    body       JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    position   VARCHAR(64) NOT NULL DEFAULT 'page',
    sort_order INT NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_content_blocks_key UNIQUE (key)
);

COMMENT ON COLUMN platform_admin.content_blocks.key IS 'مفتاح الكتلة — what_in_contents key';
COMMENT ON COLUMN platform_admin.content_blocks.body IS 'محتوى الكتلة // {"ar":"","en":""}';

-- Seed the four public pages so they render something sensible out of the box.
INSERT INTO platform_admin.content_blocks (key, title, body, position, sort_order) VALUES
    ('about',   '{"ar":"من نحن","en":"About us"}',       '{"ar":"دواء 24 هي المنظومة الرقمية الرائدة في جمهورية مصر العربية المصممة لربط الصيدليات، المستشفيات، والمراكز الطبية المرخصة مباشرة بمصانع وشركات ومستودعات توزيع الأدوية المعتمدة، لخلق سوق دوائي موثوق، شفاف، وفائق الكفاءة.","en":"Dawa24 is the leading digital pharmaceutical supply and marketplace platform in Egypt connecting licensed pharmacies and hospitals directly with verified manufacturers and distributors."}', 'page', 10),
    ('about-vision', '{"ar":"رؤيتنا","en":"Our Vision"}', '{"ar":"أن نكون البنية التحتية الرقمية الأكثر أماناً وموثوقية لسلاسل الإمداد والتوريد الدوائي في الشرق الأوسط وإفريقيا، وضمان توافر الدواء بجودة قياسية وأسعار عادلة لكل مريض عبر تمكين الصيدلي والمورد.","en":"To be the most trusted and secure digital infrastructure for pharmaceutical supply chains across the Middle East and Africa."}', 'page', 12),
    ('about-mission', '{"ar":"رسالتنا","en":"Our Mission"}', '{"ar":"القضاء على تعقيدات ونواقص التوريد التقليدي عبر توفير منصة إلكترونية ذكية تتيح مقارنة الأسعار، الشراء المباشر، الفواتير الإلكترونية المعتمدة، وسلاسل التبريد المتطورة بدعم تقني وصيدلي متواصل 24/7.","en":"To eliminate traditional supply chain friction through intelligent price comparison, direct ordering, verified e-invoicing, and cold-chain logistics."}', 'page', 14),
    ('how-it-works', '{"ar":"كيف يعمل","en":"How it works"}', '{"ar":"سجّل مؤسستك، تصفح المنتجات، اطلب مباشرة من الموردين، وتابع شحناتك.","en":"Register your organization, browse products, order directly from suppliers, and track your shipments."}', 'page', 20),
    ('faq',     '{"ar":"الأسئلة الشائعة وإرشادات الاستخدام","en":"Frequently Asked Questions"}', '{"ar":"كل ما تحتاج لمعرفته حول طلب وتوريد الأدوية، شروط التعامل، وسياسات الدفع والشحن عبر منصة دواء 24.","en":"Everything you need to know about pharmaceutical procurement, terms of service, payment methods, and cold-chain shipping on Dawa24."}', 'page', 30),
    ('help',    '{"ar":"مركز المساعدة والدعم","en":"Help & Support Center"}',     '{"ar":"فريق الدعم الفني والخدمات الصيدلانية متاح للإجابة على استفسارات الصيدليات والموردين على مدار الساعة عبر صفحة تواصل معنا أو الخط الساخن.","en":"Our pharmacy customer support team is available 24/7 to assist pharmacies and suppliers via the contact page or hotline."}', 'page', 40),
    ('home-hero', '{"ar":"سوق التوريد الدوائي المباشر للصيدليات والمستودعات","en":"Direct Pharmaceutical Supply Marketplace"}', '{"ar":"اطلب طلبيتك الدوائية مباشرة من كبرى شركات التوزيع ومصانع الأدوية المرخصة، مع ضمان فواتير إلكترونية معتمدة وسلاسل شحن مبردة (Cold-Chain).","en":"Procure medicines directly from licensed pharma manufacturers and top distributors with certified e-invoicing and temperature-controlled shipping."}', 'section', 50),
    ('highlight-coldchain', '{"ar":"سلسلة تبريد معتمدة (Cold-Chain)","en":"Certified Cold-Chain Logistics"}', '{"ar":"أسطول مجهز بحاويات مبردة ومجسات حرارية لضمان وصول الأنسولين والأمصال بأعلى معايير الأمان الحيوي.","en":"Climate-controlled delivery vehicles with real-time temperature sensors for sensitive biologics and vaccines."}', 'section', 60),
    ('highlight-fastdelivery', '{"ar":"توصيل سريع لكافة المحافظات","en":"Fast Nationwide Delivery"}', '{"ar":"شبكة لوجستية تغطي محافظات ومدن الجمهورية لضمان وصول طلبيات الصيدليات في المواعيد المحددة.","en":"Nationwide logistics network covering all Egyptian governorates for on-time pharmaceutical deliveries."}', 'section', 70),
    ('highlight-einvoice', '{"ar":"تكامل الفاتورة الإلكترونية","en":"Certified E-Invoicing"}', '{"ar":"ربط مباشر مع مصلحة الضرائب المصرية لإصدار الفواتير الضريبية المعتمدة فور تسليم أمر التوريد.","en":"Direct integration with the Egyptian Tax Authority for instant compliant electronic tax invoices."}', 'section', 80)
ON CONFLICT (key) DO NOTHING;

COMMIT;
