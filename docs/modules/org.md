# Module: Organizations & Tenant Management (`org`)

## Overview

The `org` bounded context manages organization tenants (suppliers, pharmacies, pharmacy chains), physical branch networks, staff membership roles, vendor customer ratings/reviews, following relationships, and organizational policies.

## Schema Mapping

- **PostgreSQL Schema:** `org`
- **Migrations:** `003_organizations.up.sql`, `014_enhancements.up.sql`, `015_org_extensions.up.sql`
- **Tables Owned:**
  - `org.organizations` — Core tenant entity with approval status and credit terms.
  - `org.branches` — Physical branch and warehouse locations.
  - `org.members` — Staff association and RBAC role assignment.
  - `org.organization_reviews` — Customer ratings (1-5) and review texts.
  - `org.organization_followers` — User following relationships for promotional alerts.
  - `org.organization_social_media` — Verified social media channels.
  - `org.organization_policies` — Public terms, refund policies, and shipping rules.
  - `org.user_organization_numbers` — Official contact registries.

## Invariants & Rules

1. **Tenant Isolation:** All tenant tables enforce PostgreSQL `FORCE ROW LEVEL SECURITY` with `platform.tenant_visible(organization_id)`.
2. **Single Main Branch:** Each organization has at most one branch with `is_main = true`. Creating a new main branch automatically unsets the flag on existing branches.
3. **Approval Lifecycle:** Organizations transition strictly through `pending` $\rightarrow$ `approved` / `rejected` / `suspended`.
