-- name: CreateUser :one
INSERT INTO identity.users (
    email, password_hash, name, role, status, language, timezone, phone
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING id, public_id, email, password_hash, name, role, status, language, timezone, phone, email_verified_at, phone_verified_at, created_at, updated_at, deleted_at;

-- name: GetUserByID :one
SELECT id, public_id, email, password_hash, name, role, status, language, timezone, phone, email_verified_at, phone_verified_at, created_at, updated_at, deleted_at
FROM identity.users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT id, public_id, email, password_hash, name, role, status, language, timezone, phone, email_verified_at, phone_verified_at, created_at, updated_at, deleted_at
FROM identity.users
WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUser :exec
UPDATE identity.users
SET email = $2,
    password_hash = $3,
    name = $4,
    role = $5,
    status = $6,
    language = $7,
    timezone = $8,
    phone = $9,
    email_verified_at = $10,
    phone_verified_at = $11,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserSecurity :one
SELECT user_id, login_attempts, locked_until, last_login_at, last_login_ip::text, last_user_agent, last_password_change, password_change_count, max_login_sessions
FROM identity.user_security
WHERE user_id = $1;

-- name: UpsertUserSecurity :exec
INSERT INTO identity.user_security (
    user_id, login_attempts, locked_until, last_login_at, last_login_ip, last_user_agent, last_password_change, password_change_count, max_login_sessions
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) ON CONFLICT (user_id) DO UPDATE SET
    login_attempts = EXCLUDED.login_attempts,
    locked_until = EXCLUDED.locked_until,
    last_login_at = EXCLUDED.last_login_at,
    last_login_ip = EXCLUDED.last_login_ip,
    last_user_agent = EXCLUDED.last_user_agent,
    last_password_change = EXCLUDED.last_password_change,
    password_change_count = EXCLUDED.password_change_count,
    max_login_sessions = EXCLUDED.max_login_sessions;

-- name: GetUserMFA :one
SELECT user_id, totp_secret, recovery_codes, enabled, confirmed_at
FROM identity.user_mfa
WHERE user_id = $1;

-- name: UpsertUserMFA :exec
INSERT INTO identity.user_mfa (
    user_id, totp_secret, recovery_codes, enabled, confirmed_at
) VALUES (
    $1, $2, $3, $4, $5
) ON CONFLICT (user_id) DO UPDATE SET
    totp_secret = EXCLUDED.totp_secret,
    recovery_codes = EXCLUDED.recovery_codes,
    enabled = EXCLUDED.enabled,
    confirmed_at = EXCLUDED.confirmed_at;

-- name: GetPlatformRolesForUser :many
SELECT role_key
FROM identity.user_roles
WHERE user_id = $1;

-- name: GetPermissionsForUserAndOrg :many
SELECT DISTINCT p.key
FROM identity.permissions p
JOIN identity.role_permissions rp ON rp.permission_key = p.key
WHERE rp.role_key IN (
    -- Platform-level roles
    SELECT role_key FROM identity.user_roles WHERE user_id = $1
    UNION
    -- System default role (e.g. users.role)
    SELECT role FROM identity.users WHERE id = $1
    UNION
    -- Organization membership role
    SELECT role_key FROM org.members WHERE user_id = $1 AND organization_id = $2 AND status = 'active'
);
