-- Migration 048: Seed Realistic Egyptian Pharmaceutical Mock Data
-- Populates real-world pharma suppliers, licensed pharmacies, comprehensive categories,
-- registered drugs, variants, warehouses, stocks, and professional job postings.

BEGIN;

-- 1. Fix HR ID length constraints for UUID public IDs
ALTER TABLE hr.job_offers ALTER COLUMN public_id TYPE VARCHAR(64);
ALTER TABLE hr.job_offers ALTER COLUMN status TYPE VARCHAR(64);
ALTER TABLE hr.job_applications ALTER COLUMN public_id TYPE VARCHAR(64);
ALTER TABLE hr.job_applications ALTER COLUMN status TYPE VARCHAR(64);

-- 2. Insert HR Job Categories if not already present
INSERT INTO hr.job_categories (name, slug, is_active)
VALUES
    ('{"ar":"صيادلة وإدارة فروع","en":"Pharmacists & Branch Management"}'::jsonb, 'pharmacists', true),
    ('{"ar":"دعاية وتسويق طبي","en":"Medical Reps & Marketing"}'::jsonb, 'medical-reps', true),
    ('{"ar":"سلاسل إمداد ومخازن","en":"Supply Chain & Warehouses"}'::jsonb, 'supply-chain', true),
    ('{"ar":"مساعدو صيادلة","en":"Pharmacy Assistants"}'::jsonb, 'pharmacy-assistants', true)
ON CONFLICT (slug) DO UPDATE SET is_active = true;

-- 3. Insert Official Egyptian Pharma Suppliers & Distributors
INSERT INTO org.organizations (
    organization_number, name, description, type, status,
    email, phone, address, tax_number, license_document_url, verification_notes,
    min_order_price, max_order_price, is_sponsored, rating, rank, approved_at
)
VALUES
(
    'ORG-EGY-1001',
    '{"ar":"الشركة المتحدة لتوزيع الأدوية (UCP)","en":"United Company of Pharmacists"}'::jsonb,
    '{"ar":"كبار موزعي الأدوية ومستحضرات التجميل في مصر، أسطول شحن مبرد يغطي 27 محافظة.","en":"Leading pharmaceutical distributor in Egypt."}'::jsonb,
    'supplier',
    'approved',
    'supply@ucp-egypt.com',
    '0224158000',
    'المنطقة الصناعية، العبور، القاهرة',
    '100-249-582',
    '/uploads/licenses/ucp_license.pdf',
    'تم تدقيق السجل التجاري وترخيص هيئة الدواء بنجاح',
    1000.00,
    500000.00,
    true,
    5,
    99,
    NOW()
),
(
    'ORG-EGY-1002',
    '{"ar":"ابن سينا فارما للتوزيع الدوائي","en":"Ibnsina Pharma"}'::jsonb,
    '{"ar":"شبكة توزيع دوائي رائدة، توريد يومي لآلاف الصيدليات والمراكز الطبية المعتمدة.","en":"Leading pharmaceutical supply chain network."}'::jsonb,
    'supplier',
    'approved',
    'sales@ibnsina-pharma.com',
    '0227984000',
    'شارع التسعين الشمالي، التجمع الخامس، القاهرة',
    '200-481-913',
    '/uploads/licenses/ibnsina_license.pdf',
    'منشأة معتمدة ومطابقة لكافة اشتراطات الفاتورة الإلكترونية',
    500.00,
    300000.00,
    true,
    5,
    98,
    NOW()
),
(
    'ORG-EGY-1003',
    '{"ar":"شركة فارما أوفرسيز للأدوية","en":"Pharma Overseas"}'::jsonb,
    '{"ar":"موزع رئيسي لكبرى شركات الأدوية العالمية والمحلية مع مخازن تبريد مركزية.","en":"Major distributor for global and local pharma companies."}'::jsonb,
    'supplier',
    'approved',
    'orders@pharmaoverseas.com',
    '0233056000',
    'المنطقة الاستثمارية، مدينة 6 أكتوبر، الجيزة',
    '300-842-105',
    '/uploads/licenses/pharma_overseas.pdf',
    'معتمد لدى منظومة هيئة الدواء والضرائب المصرية',
    750.00,
    400000.00,
    false,
    4,
    95,
    NOW()
),
(
    'ORG-EGY-1004',
    '{"ar":"مستودع أدوية النيل للتوزيع","en":"Nile Pharma Distribution Depot"}'::jsonb,
    '{"ar":"مستودع مركزي للأدوية والمستلزمات الطبية وسلاسل التبريد المباشر للإسكندرية والدلتا.","en":"Nile Pharma Depot for Delta and Alexandria."}'::jsonb,
    'supplier',
    'approved',
    'info@nilepharma-depot.com',
    '034298000',
    'طريق الإسكندرية الزراعي، سموحة، الإسكندرية',
    '400-192-384',
    '/uploads/licenses/nile_license.pdf',
    'فحص روتيني مكتمل واعتماد كامل',
    300.00,
    150000.00,
    false,
    4,
    92,
    NOW()
)
ON CONFLICT DO NOTHING;

-- 4. Insert Licensed Community & Chain Pharmacies
INSERT INTO org.organizations (
    organization_number, name, description, type, status,
    email, phone, address, tax_number, license_document_url, verification_notes,
    approved_at
)
VALUES
(
    'ORG-PHARM-2001',
    '{"ar":"مجموعة صيدليات العزبي","en":"El-Ezaby Pharmacies Group"}'::jsonb,
    '{"ar":"سلسلة صيدليات كبرى معتمدة في كافة أنحاء جمهورية مصر العربية.","en":"Leading pharmacy chain in Egypt."}'::jsonb,
    'company',
    'approved',
    'procurement@elezaby.com',
    '19600',
    'المعادي، القاهرة',
    '101-992-481',
    '/uploads/licenses/elezaby_pharm.pdf',
    'سلسلة صيدليات مرخصة - ترخيص نقابة وصحة سارٍ',
    NOW()
),
(
    'ORG-PHARM-2002',
    '{"ar":"صيدلية النصر المركزية","en":"Al-Nasr Central Pharmacy"}'::jsonb,
    '{"ar":"صيدلية مجتمعية معتمدة تقدم خدمات صرف وتوريد دوائي متكامل.","en":"Licensed community pharmacy."}'::jsonb,
    'company',
    'approved',
    'dr.hossam@alnasr-pharm.com',
    '0222718900',
    'شارع مصطفى النحاس، مدينة نصر، القاهرة',
    '102-482-194',
    '/uploads/licenses/alnasr_pharm.pdf',
    'صيدلية مرخصة ومطابقة للشروط',
    NOW()
),
(
    'ORG-PHARM-2003',
    '{"ar":"صيدليات سيف (Seif Pharmacies)","en":"Seif Pharmacies"}'::jsonb,
    '{"ar":"سلسلة صيدليات متكاملة تقدم خدمات الرعاية الصيدلانية.","en":"Seif Pharmacies Chain."}'::jsonb,
    'company',
    'approved',
    'orders@seif-pharmacies.com',
    '19199',
    'الدقي، الجيزة',
    '103-752-910',
    '/uploads/licenses/seif_pharm.pdf',
    'ترخيص سارٍ ومعتمد',
    NOW()
)
ON CONFLICT DO NOTHING;

-- 5. Insert Standard Pharmaceutical Categories
INSERT INTO catalog.categories (name, description, icon, status, sort_order)
VALUES
    ('{"ar":"المضادات الحيوية","en":"Antibiotics"}'::jsonb, '{"ar":"أدوية علاج العدوى البكتيرية والالتهابات","en":"Antibiotics and antimicrobials"}'::jsonb, 'pill', 'active', 1),
    ('{"ar":"مسكنات وخافضات الحرارة","en":"Analgesics & Antipyretics"}'::jsonb, '{"ar":"مسكنات الألم ومضادات الالتهاب غير الستيرويدية","en":"Pain relief and antipyretics"}'::jsonb, 'pill', 'active', 2),
    ('{"ar":"أدوية القلب والضغط","en":"Cardiovascular"}'::jsonb, '{"ar":"علاجات ضغط الدم وأمراض القلب والشرايين","en":"Heart and blood pressure treatments"}'::jsonb, 'heart', 'active', 3),
    ('{"ar":"أدوية السكري والغدد","en":"Diabetes & Endocrine"}'::jsonb, '{"ar":"أدوية السكر والإنسولينات ومنظمات الهرمونات","en":"Diabetes and endocrine medications"}'::jsonb, 'pill', 'active', 4),
    ('{"ar":"الجهاز الهضمي والقولون","en":"Gastrointestinal"}'::jsonb, '{"ar":"علاجات الحموضة، قرحة المعدة والقولون","en":"Digestive and stomach care"}'::jsonb, 'pill', 'active', 5),
    ('{"ar":"الفيتامينات والمكملات الغذائية","en":"Vitamins & Supplements"}'::jsonb, '{"ar":"فيتامينات، معادن، ومقويات المناعة","en":"Vitamins and dietary supplements"}'::jsonb, 'package', 'active', 6),
    ('{"ar":"أدوية الجهاز التنفسي والحساسية","en":"Respiratory & Allergy"}'::jsonb, '{"ar":"بخاخات الصدر، مضادات الهيستامين وموسعات الشعب","en":"Asthma, cough and allergy relief"}'::jsonb, 'pill', 'active', 7),
    ('{"ar":"مستلزمات وأجهزة طبية","en":"Medical Supplies"}'::jsonb, '{"ar":"شاش، قطن، محاقن، وأجهزة قياس السكر والضغط","en":"Medical disposables and diagnostic devices"}'::jsonb, 'package', 'active', 8)
ON CONFLICT DO NOTHING;

-- 6. Insert Core Pharmaceutical Brands
INSERT INTO catalog.brands (name, description, status)
VALUES
    ('{"ar":"جلاكسو سميث كلاين (GSK)","en":"GlaxoSmithKline"}'::jsonb, '{"ar":"شركة أدوية عالمية رائدة","en":"Global pharma leader"}'::jsonb, 'active'),
    ('{"ar":"نوفارتس (Novartis)","en":"Novartis"}'::jsonb, '{"ar":"أدوية علاجية متخصصة ومبتكرة","en":"Innovative medicines"}'::jsonb, 'active'),
    ('{"ar":"سانوفي (Sanofi)","en":"Sanofi"}'::jsonb, '{"ar":"علاجات السكري واللقاحات والرعاية الصحية","en":"Healthcare & diabetes solutions"}'::jsonb, 'active'),
    ('{"ar":"إيفا فارما (Eva Pharma)","en":"Eva Pharma"}'::jsonb, '{"ar":"كبرى شركات الصناعات الدوائية في مصر والشرق الأوسط","en":"Leading Egyptian pharma manufacturer"}'::jsonb, 'active'),
    ('{"ar":"آمون للأدوية (Amoun)","en":"Amoun Pharmaceutical"}'::jsonb, '{"ar":"صرح دوائي مصري رائد بجودة عالمية","en":"Egyptian pharmaceutical market leader"}'::jsonb, 'active'),
    ('{"ar":"ميرك (Merck)","en":"Merck Healthcare"}'::jsonb, '{"ar":"ريادة في أدوية القلب والأورام والسكري","en":"Global science and tech company"}'::jsonb, 'active')
ON CONFLICT DO NOTHING;

-- 7. Insert Top Essential Registered Products & Stocks
DO $$
DECLARE
    v_supplier_id BIGINT;
    v_warehouse_id BIGINT;
    v_cat_antibiotics BIGINT;
    v_cat_analgesics BIGINT;
    v_cat_cardio BIGINT;
    v_cat_diabetes BIGINT;
    v_cat_gi BIGINT;
    v_cat_vitamins BIGINT;
    v_cat_respiratory BIGINT;
    v_brand_gsk BIGINT;
    v_brand_novartis BIGINT;
    v_brand_sanofi BIGINT;
    v_brand_eva BIGINT;
    v_brand_amoun BIGINT;
    v_brand_merck BIGINT;
    v_prod_id BIGINT;
    v_var_id BIGINT;
BEGIN
    SELECT id INTO v_supplier_id FROM org.organizations WHERE organization_number = 'ORG-EGY-1001' LIMIT 1;
    IF v_supplier_id IS NULL THEN
        SELECT id INTO v_supplier_id FROM org.organizations LIMIT 1;
    END IF;

    -- Ensure a warehouse exists for inventory stocks
    SELECT id INTO v_warehouse_id FROM inventory.warehouses WHERE organization_id = v_supplier_id LIMIT 1;
    IF v_warehouse_id IS NULL THEN
        INSERT INTO inventory.warehouses (organization_id, name, code, address, is_active)
        VALUES (v_supplier_id, 'المستودع الرئيسي - العبور', 'WH-MAIN-01', 'المنطقة الصناعية، العبور، القاهرة', true)
        RETURNING id INTO v_warehouse_id;
    END IF;

    SELECT id INTO v_cat_antibiotics FROM catalog.categories WHERE name->>'ar' = 'المضادات الحيوية' LIMIT 1;
    SELECT id INTO v_cat_analgesics FROM catalog.categories WHERE name->>'ar' = 'مسكنات وخافضات الحرارة' LIMIT 1;
    SELECT id INTO v_cat_cardio FROM catalog.categories WHERE name->>'ar' = 'أدوية القلب والضغط' LIMIT 1;
    SELECT id INTO v_cat_diabetes FROM catalog.categories WHERE name->>'ar' = 'أدوية السكري والغدد' LIMIT 1;
    SELECT id INTO v_cat_gi FROM catalog.categories WHERE name->>'ar' = 'الجهاز الهضمي والقولون' LIMIT 1;
    SELECT id INTO v_cat_vitamins FROM catalog.categories WHERE name->>'ar' = 'الفيتامينات والمكملات الغذائية' LIMIT 1;
    SELECT id INTO v_cat_respiratory FROM catalog.categories WHERE name->>'ar' = 'أدوية الجهاز التنفسي والحساسية' LIMIT 1;

    SELECT id INTO v_brand_gsk FROM catalog.brands WHERE name->>'ar' LIKE '%GSK%' LIMIT 1;
    SELECT id INTO v_brand_novartis FROM catalog.brands WHERE name->>'ar' LIKE '%نوفارتس%' LIMIT 1;
    SELECT id INTO v_brand_sanofi FROM catalog.brands WHERE name->>'ar' LIKE '%سانوفي%' LIMIT 1;
    SELECT id INTO v_brand_eva FROM catalog.brands WHERE name->>'ar' LIKE '%إيفا%' LIMIT 1;
    SELECT id INTO v_brand_amoun FROM catalog.brands WHERE name->>'ar' LIKE '%آمون%' LIMIT 1;
    SELECT id INTO v_brand_merck FROM catalog.brands WHERE name->>'ar' LIKE '%ميرك%' LIMIT 1;

    -- Product 1: Augmentin 1g
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_antibiotics, v_brand_gsk,
        '{"ar":"أوجمنتين 1 جم أقراص","en":"Augmentin 1g Tablets"}'::jsonb,
        '{"ar":"مضاد حيوي واسع المجال يحتوي على أموكسيسيلين وحمض الكلافولانيك لعلاج الالتهابات البكتيرية.","en":"Broad-spectrum antibiotic."}'::jsonb,
        'AUG-1G-14T', '6221008291048', 135.00, true, 'أقراص', 'Amoxicillin + Clavulanic Acid', 'GlaxoSmithKline (GSK)', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"علبة 14 قرص","en":"Box of 14 Tablets"}'::jsonb,
        'AUG-1G-14T-V1', '6221008291048', 135.00, 115.00, 'AUG-2849', '2028-06-30'::date, 5, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 850, 10);
    END IF;

    -- Product 2: Panadol Extra
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_analgesics, v_brand_gsk,
        '{"ar":"بانادول إكسترا أقراص مسكنة","en":"Panadol Extra Tablets"}'::jsonb,
        '{"ar":"مسكن فعال للآلام وخافض للحرارة مع الكافيين لتعزيز المفعول والتسكين السريع.","en":"Paracetamol + Caffeine for fast pain relief."}'::jsonb,
        'PAN-EXT-24T', '6221004928103', 45.00, true, 'أقراص', 'Paracetamol 500mg + Caffeine 65mg', 'Haleon / GSK', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"شريطين 24 قرص","en":"2 Strips 24 Tablets"}'::jsonb,
        'PAN-EXT-24T-V1', '6221004928103', 45.00, 38.00, 'PAN-9921', '2028-11-30'::date, 10, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 2400, 20);
    END IF;

    -- Product 3: Concor 5mg
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_cardio, v_brand_merck,
        '{"ar":"كونكور 5 مجم أقراص","en":"Concor 5mg Tablets"}'::jsonb,
        '{"ar":"علاج ارتفاع ضغط الدم وتنظيم ضربات القلب وتخفيف العبء على عضلة القلب.","en":"Bisoprolol Fumarate for hypertension."}'::jsonb,
        'CON-5MG-30T', '6221003819201', 62.50, true, 'أقراص', 'Bisoprolol Fumarate 5mg', 'Merck Healthcare', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"علبة 30 قرص","en":"Box of 30 Tablets"}'::jsonb,
        'CON-5MG-30T-V1', '6221003819201', 62.50, 53.00, 'CON-5820', '2028-09-30'::date, 3, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 1100, 10);
    END IF;

    -- Product 4: Glucophage 1000mg
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_diabetes, v_brand_merck,
        '{"ar":"جلوكوفاج 1000 مجم أقراص","en":"Glucophage 1000mg Tablets"}'::jsonb,
        '{"ar":"منظم مستويات السكر في الدم لمرضى السكري من النوع الثاني.","en":"Metformin Hydrochloride 1000mg."}'::jsonb,
        'GLU-1000-30T', '6221007419482', 70.00, true, 'أقراص', 'Metformin HCl 1000mg', 'Merck Healthcare', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"علبة 30 قرص","en":"Box of 30 Tablets"}'::jsonb,
        'GLU-1000-30T-V1', '6221007419482', 70.00, 59.50, 'GLU-4819', '2028-04-30'::date, 5, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 950, 10);
    END IF;

    -- Product 5: Cataflam 50mg
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_analgesics, v_brand_novartis,
        '{"ar":"كتافلام 50 مجم أقراص مسكنة ومضادة للالتهاب","en":"Cataflam 50mg Tablets"}'::jsonb,
        '{"ar":"مسكن سريع المفعول لآلام الأسنان والعظام والصداع والالتهابات الحادة.","en":"Diclofenac Potassium 50mg."}'::jsonb,
        'CAT-50MG-20T', '6221005829104', 58.00, true, 'أقراص', 'Diclofenac Potassium 50mg', 'Novartis', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"شريطين 20 قرص","en":"20 Tablets"}'::jsonb,
        'CAT-50MG-20T-V1', '6221005829104', 58.00, 49.00, 'CAT-3819', '2027-12-31'::date, 5, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 1400, 15);
    END IF;

    -- Product 6: Ventolin Inhaler
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_respiratory, v_brand_gsk,
        '{"ar":"فنتولين بخاخ استنشاق 100 ميكروجرام","en":"Ventolin Evohaler 100mcg"}'::jsonb,
        '{"ar":"موسع للشعب الهوائية لعلاج أزمات الربو وضيق التنفس الحاد.","en":"Salbutamol 100mcg Inhaler."}'::jsonb,
        'VEN-INH-200D', '6221009182736', 68.00, true, 'بخاخ', 'Salbutamol 100mcg', 'GlaxoSmithKline (GSK)', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"عبوة 200 جرعة","en":"200 Doses Inhaler"}'::jsonb,
        'VEN-INH-200D-V1', '6221009182736', 68.00, 57.00, 'VEN-7410', '2028-08-31'::date, 3, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 620, 5);
    END IF;

    -- Product 7: Lantus SoloStar
    INSERT INTO catalog.products (
        organization_id, category_id, brand_id, name, description, sku, barcode, price, is_featured, dosage_form, scientific_name, manufacturing_companies, status
    ) VALUES (
        v_supplier_id, v_cat_diabetes, v_brand_sanofi,
        '{"ar":"لانتوس سولوستار أقلام إنسولين (سلسلة تبريد)","en":"Lantus SoloStar Insulin Pens"}'::jsonb,
        '{"ar":"إنسولين ممتد المفعول للحقن تحت الجلد - يتطلب سلسلة تبريد 2 إلى 8 درجات مئوية.","en":"Insulin Glargine 100 units/ml."}'::jsonb,
        'LAN-SOLO-5P', '6221001928374', 680.00, true, 'حقن مبردة', 'Insulin Glargine 100 U/ml', 'Sanofi', 'active'
    ) RETURNING id INTO v_prod_id;

    INSERT INTO catalog.product_variants (
        organization_id, product_id, name, sku, barcode, price, cost_price, batch_number, expiry_date, min_order_qty, status, is_featured
    ) VALUES (
        v_supplier_id, v_prod_id,
        '{"ar":"علبة 5 أقلام معبأة","en":"Box of 5 Prefilled Pens"}'::jsonb,
        'LAN-SOLO-5P-V1', '6221001928374', 680.00, 595.00, 'LAN-9182', '2027-10-31'::date, 2, 'active', true
    ) RETURNING id INTO v_var_id;

    IF v_warehouse_id IS NOT NULL THEN
        INSERT INTO inventory.stocks (organization_id, warehouse_id, product_id, product_variant_id, quantity, min_threshold)
        VALUES (v_supplier_id, v_warehouse_id, v_prod_id, v_var_id, 280, 5);
    END IF;

END $$;

-- 8. Insert Professional Job Postings with clean public IDs
DO $$
DECLARE
    v_org_ezaby BIGINT;
    v_org_ucp BIGINT;
    v_org_ibnsina BIGINT;
    v_org_alnasr BIGINT;
    v_cat_pharm BIGINT;
    v_cat_reps BIGINT;
    v_cat_supply BIGINT;
    v_cat_asst BIGINT;
BEGIN
    SELECT id INTO v_org_ezaby FROM org.organizations WHERE organization_number = 'ORG-PHARM-2001' LIMIT 1;
    SELECT id INTO v_org_alnasr FROM org.organizations WHERE organization_number = 'ORG-PHARM-2002' LIMIT 1;
    SELECT id INTO v_org_ucp FROM org.organizations WHERE organization_number = 'ORG-EGY-1001' LIMIT 1;
    SELECT id INTO v_org_ibnsina FROM org.organizations WHERE organization_number = 'ORG-EGY-1002' LIMIT 1;

    -- Fallbacks
    IF v_org_ezaby IS NULL THEN SELECT id INTO v_org_ezaby FROM org.organizations LIMIT 1; END IF;
    IF v_org_alnasr IS NULL THEN v_org_alnasr := v_org_ezaby; END IF;
    IF v_org_ucp IS NULL THEN v_org_ucp := v_org_ezaby; END IF;
    IF v_org_ibnsina IS NULL THEN v_org_ibnsina := v_org_ezaby; END IF;

    SELECT id INTO v_cat_pharm FROM hr.job_categories WHERE slug = 'pharmacists' LIMIT 1;
    SELECT id INTO v_cat_reps FROM hr.job_categories WHERE slug = 'medical-reps' LIMIT 1;
    SELECT id INTO v_cat_supply FROM hr.job_categories WHERE slug = 'supply-chain' LIMIT 1;
    SELECT id INTO v_cat_asst FROM hr.job_categories WHERE slug = 'pharmacy-assistants' LIMIT 1;

    INSERT INTO hr.job_offers (
        public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status
    ) VALUES
    (
        'job_pharm_sr_01', v_org_alnasr, v_cat_pharm,
        '{"ar":"صيدلي مسائي / صيدلي أول","en":"Senior Evening Pharmacist"}'::jsonb,
        'إدارة صرف الوصفات الطبية، تقديم الاستشارات الدوائية للعملاء، متابعة المخزون وتسجيل النواقص.',
        'بكالوريوس صيدلة، ترخيص مزاولة المهنة سارٍ، خبرة من 1-3 سنوات في الصيدليات الأهلية.',
        9000.00, 12000.00, 'القاهرة - مدينة نصر', 'published'
    ),
    (
        'job_mgr_ezaby_02', v_org_ezaby, v_cat_pharm,
        '{"ar":"مدير فرع صيدلية (Pharmacy Branch Manager)","en":"Pharmacy Branch Manager"}'::jsonb,
        'الإشراف الكامل على إدارة الفرع، فريق الصيادلة، تحقيق أهداف المبيعات، ومراجعة مطابقة الجودة.',
        'بكالوريوس صيدلة، خبرة لا تقل عن 4 سنوات في إدارة الصيدليات الكبرى أو السلاسل.',
        14000.00, 18000.00, 'الجيزة - الدقي', 'published'
    ),
    (
        'job_medrep_ucp_03', v_org_ucp, v_cat_reps,
        '{"ar":"مندوب دعاية طبية وتسويق صيدلاني (Medical Representative)","en":"Medical Representative"}'::jsonb,
        'بناء علاقات مع الصيدليات والمراكز الطبية، الترويج للأصناف الدوائية الجديدة، وفتح حسابات توريد جديدة.',
        'خريج صيدلة أو علوم طبية، مهارات تواصل وتفاوض ممتازة، يفضل وجود سيارة.',
        11000.00, 16000.00, 'الإسكندرية والبحيرة', 'published'
    ),
    (
        'job_whmgr_ibnsina_04', v_org_ibnsina, v_cat_supply,
        '{"ar":"مدير مخزن وتوزيع دوائي وسلسلة تبريد","en":"Warehouse & Cold Chain Manager"}'::jsonb,
        'إدارة عمليات استلام وفحص الشحنات، الرقابة على درجات حرارة التبريد 2-8 مئوية، وجدولة سيارات التوزيع.',
        'مؤهل عالٍ مناسب، خبرة 3 سنوات على الأقل في مخازن الأدوية والتوزيع وسلاسل التبريد.',
        13000.00, 17500.00, 'القاهرة - العبور', 'published'
    ),
    (
        'job_asst_alnasr_05', v_org_alnasr, v_cat_asst,
        '{"ar":"مساعد صيدلي خبرة (Pharmacy Assistant)","en":"Pharmacy Assistant"}'::jsonb,
        'ترتيب الرفوف، مساعدة الصيدلي في تجهيز طلبيات الأدوية، واستقبال مستحضرات العناية والتجميل.',
        'مؤهل متوسط أو فوق متوسط، خبرة سنة على الأقل في صيدليات المجتمع.',
        6000.00, 8000.00, 'القاهرة - مصر الجديدة', 'published'
    )
    ON CONFLICT DO NOTHING;
END $$;

COMMIT;
