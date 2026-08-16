# Module: Platform Admin

## Overview

The `platform_admin` bounded context provides global administrative configurations, geographical reference data (countries, cities), and public tenant settings.

## Schema Mapping

- **PostgreSQL Schemas:** `platform_admin`
- **Migrations:** `012_platform_admin.up.sql`
- **Tables Owned:**
  - `platform_admin.system_settings` — Key-value system configurations with public visibility flags.
  - `platform_admin.countries` & `platform_admin.cities` — Normalized geography master data.

## Invariants & Rules

1. **Global Configuration:** System settings can be marked `is_public: true` to be safely exposed to frontend clients without authentication.
2. **Normalized Geography:** Countries and cities are structured with standardized ISO-3166 alpha codes, international dialing prefixes, and local currencies.
