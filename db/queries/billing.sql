-- name: GetOrCreateWallet :one
INSERT INTO billing.wallets (user_id, currency)
VALUES ($1, $2)
ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
RETURNING id, public_id, user_id, organization_id, currency, created_at, updated_at;

-- name: GetLatestWalletTransaction :one
SELECT id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at
FROM billing.wallet_transactions
WHERE wallet_id = $1
ORDER BY id DESC
LIMIT 1;

-- name: ListPlans :many
SELECT id, public_id, slug, name, description, price_month, price_year, duration_days, max_users, is_active, created_at, updated_at
FROM billing.plans
WHERE is_active = true
ORDER BY id ASC;
