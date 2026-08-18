-- Dawa24 Platform Database Reset Script
-- Wipes all application tables and data rows while preserving reference tables (cities, roles) and super_admin account.

BEGIN;

-- 1. Truncate all application and tenant data
TRUNCATE TABLE
    commerce.order_status_history,
    commerce.order_items,
    commerce.invoices,
    commerce.payments,
    commerce.orders,
    commerce.cart_items,
    commerce.carts,
    inventory.lots,
    inventory.adjustments,
    inventory.transfers,
    catalog.product_images,
    catalog.products,
    promo.coupon_redemptions,
    promo.coupons,
    promo.offer_products,
    promo.offers,
    promo.special_offers,
    hr.job_applications,
    hr.resumes,
    hr.jobs,
    platform_admin.documents,
    platform_admin.service_requests,
    platform_admin.services,
    platform_admin.job_applications,
    platform_admin.jobs,
    platform_admin.activity_logs,
    chat.messages,
    chat.conversations,
    workflow.approvals,
    workflow.audit_logs,
    notifications.notifications,
    billing.transactions,
    billing.wallets,
    org.review_ratings,
    org.reviews,
    org.policies,
    org.coverage_areas,
    org.members,
    org.branches,
    org.organizations,
    identity.sessions,
    identity.user_addresses,
    identity.user_address_history,
    identity.user_favorites,
    identity.user_preferences,
    identity.audit_logs
CASCADE;

-- 2. Delete non-admin users
DELETE FROM identity.user_security WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
DELETE FROM identity.user_mfa WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
DELETE FROM identity.users WHERE role != 'super_admin';

-- 3. Upsert Super Admin account (Password: Dawa24!Test)
-- $2a$10$O4b9xM3p1z2b0V7n7v0oIu7b3wY6l5w1v9y4k6q2s8t0u1v2w3x4y -> bcrypt hash for Dawa24!Test
INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
VALUES (
    'admin@dawa24.com',
    '$2a$10$tZ8I2vTzMhyZqE3rR95nneC65w9jS2l52o35vFf.36X149fK4bUaK',
    '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
    'super_admin',
    'active',
    'ar',
    'Africa/Cairo'
)
ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    name          = EXCLUDED.name,
    role          = 'super_admin',
    status        = 'active',
    deleted_at    = NULL,
    updated_at    = now();

INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
VALUES (
    'admin@dawa24.test',
    '$2a$10$tZ8I2vTzMhyZqE3rR95nneC65w9jS2l52o35vFf.36X149fK4bUaK',
    '{"ar":"مدير المنصة العام","en":"Platform Super Admin"}'::jsonb,
    'super_admin',
    'active',
    'ar',
    'Africa/Cairo'
)
ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    name          = EXCLUDED.name,
    role          = 'super_admin',
    status        = 'active',
    deleted_at    = NULL,
    updated_at    = now();

INSERT INTO identity.user_security (user_id, login_attempts)
SELECT id, 0 FROM identity.users WHERE email IN ('admin@dawa24.com', 'admin@dawa24.test')
ON CONFLICT (user_id) DO UPDATE SET login_attempts = 0, locked_until = NULL;

COMMIT;