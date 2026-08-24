-- 118_performance_composite_indexes (up)
--
-- High-performance composite and GIN trigram indexes for high-scale catalog searches,
-- commerce order filtering, file comparison processing, and notifications.

BEGIN;

-- 1. Catalog Products Composite & Trigram Expressions
CREATE INDEX IF NOT EXISTS idx_products_org_status
    ON catalog.products (organization_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_products_category_brand
    ON catalog.products (category_id, brand_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_products_dosage
    ON catalog.products (dosage_form)
    WHERE deleted_at IS NULL AND dosage_form <> '';

CREATE INDEX IF NOT EXISTS idx_products_price_sort
    ON catalog.products (price ASC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_products_name_ar_trgm
    ON catalog.products USING GIN ((platform.normalize_arabic(COALESCE(name->>'ar', ''))) gin_trgm_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_products_name_en_trgm
    ON catalog.products USING GIN ((COALESCE(name->>'en', '')) gin_trgm_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_products_scientific_trgm
    ON catalog.products USING GIN ((platform.normalize_arabic(COALESCE(scientific_name, ''))) gin_trgm_ops)
    WHERE deleted_at IS NULL AND scientific_name <> '';

CREATE INDEX IF NOT EXISTS idx_products_active_trgm
    ON catalog.products USING GIN ((platform.normalize_arabic(COALESCE(active, ''))) gin_trgm_ops)
    WHERE deleted_at IS NULL AND active <> '';

-- 2. Catalog Saving Products
CREATE INDEX IF NOT EXISTS idx_saving_products_org_prod
    ON catalog.saving_products (organization_id, product_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_saving_products_user
    ON catalog.saving_products (user_id)
    WHERE deleted_at IS NULL AND user_id IS NOT NULL;

-- 3. Commerce Orders & Carts
CREATE INDEX IF NOT EXISTS idx_orders_buyer_status
    ON commerce.orders (buyer_org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_seller_status
    ON commerce.orders (seller_org_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_carts_user_org
    ON commerce.carts (user_id, organization_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_wishlists_user_prod
    ON commerce.wishlists (user_id, product_id);

-- 4. Compare File Rows & Sessions
CREATE INDEX IF NOT EXISTS idx_compare_file_rows_file_id
    ON compare.file_rows (file_id);

CREATE INDEX IF NOT EXISTS idx_compare_files_user_status
    ON compare.files (user_id, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_compare_sessions_user_active
    ON compare.user_sessions (user_id, is_active, last_activity_at DESC)
    WHERE deleted_at IS NULL;

-- 5. Notifications
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications.notifications (user_id, is_read, created_at DESC);

COMMIT;
