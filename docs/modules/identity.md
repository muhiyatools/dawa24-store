# Module: Identity & Authentication

## Overview

The `identity` bounded context manages user accounts, authentication, password verification, session lifecycles, and unified Role-Based Access Control (RBAC).

## Schema Mapping

- **PostgreSQL Schemas:** `identity`, `profile`
- **Migrations:** `002_identity.up.sql`
- **Tables Owned:**
  - `identity.users` — Core identity only (name, email, password hash, role, status).
  - `identity.user_security` — Login attempts, lockout timestamps, IP tracking.
  - `identity.user_mfa` — TOTP 2FA secret and confirmation status.
  - `identity.user_identities` — Federated third-party provider logins.
  - `identity.roles` & `identity.permissions` — Unified RBAC.
  - `identity.role_permissions` & `identity.user_roles` — Role assignments.
  - `identity.kyc_records` — Regulated compliance verification.
  - `identity.account_deletion_requests` — Account deletion audit workflow.
  - `profile.user_profiles` & `profile.user_preferences` — Extended profile metadata.

## Invariants & Rules

1. **Bcrypt Compatibility:** PHP/Laravel `$2y$` hashes and Go `$2a$` hashes are verified seamlessly. Existing user passwords require no forced resets.
2. **Account Lockout:** 5 consecutive failed login attempts lock the account for 15 minutes.
3. **Session Store:** Stored in Redis under `session:<token>` and `user_sessions:<userID>` sets.
4. **Tenant Resolution:** The middleware extracts tenant ID from `X-Dawa-Org-ID` header or session `ActiveOrgID`, and sets `database.WithTenant(ctx, orgID)`.
5. **Unified RBAC:** A user's effective permissions are the union of their platform roles (`identity.user_roles`), default role (`users.role`), and organization membership roles (`org.members`).

## Endpoints

- `POST /api/v1/auth/register` — Create new user account.
- `POST /api/v1/auth/login` — Authenticate and receive session cookie/token.
- `POST /api/v1/auth/logout` — Invalidate session.
- `GET /api/v1/auth/me` — Inspect current session and permissions.
