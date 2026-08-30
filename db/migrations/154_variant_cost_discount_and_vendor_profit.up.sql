-- Migration 154: Add cost discount percentage and make cost_price nullable/optional
-- Schema: catalog, commerce

BEGIN;

-- 1. In catalog.product_variants: Ensure cost_price is nullable and add cost_discount_percentage
ALTER TABLE catalog.product_variants
    ALTER COLUMN cost_price DROP NOT NULL,
    ALTER COLUMN cost_price SET DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS cost_discount_percentage NUMERIC(5,2) NOT NULL DEFAULT 0.00;

COMMENT ON COLUMN catalog.product_variants.cost_price IS 'سعر التكلفة للوحدة (اختياري) - Optional unit cost price';
COMMENT ON COLUMN catalog.product_variants.cost_discount_percentage IS 'نسبة خصم التكلفة (اختياري) - Cost discount percentage applied to cost_price';

-- 2. In commerce.order_lines: Add cost_price snapshot and cost_discount_percentage snapshot
ALTER TABLE commerce.order_lines
    ADD COLUMN IF NOT EXISTS cost_price NUMERIC(12,2) DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS cost_discount_percentage NUMERIC(5,2) NOT NULL DEFAULT 0.00;

COMMENT ON COLUMN commerce.order_lines.cost_price IS 'سعر التكلفة المسجل للصنف وقت الشراء - Cost price snapshot at purchase';
COMMENT ON COLUMN commerce.order_lines.cost_discount_percentage IS 'نسبة خصم التكلفة المسجلة وقت الشراء - Cost discount percentage snapshot';

COMMIT;
