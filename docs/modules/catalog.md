# Module: Product Catalog

## Overview

The `catalog` bounded context manages pharmaceutical products, packaging variants, hierarchical categories, brands, and Arabic trigram search.

## Schema Mapping

- **PostgreSQL Schemas:** `catalog`
- **Migrations:** `004_catalog.up.sql`
- **Tables Owned:**
  - `catalog.categories` — Hierarchical product taxonomy with bilingual names.
  - `catalog.brands` — Pharmaceutical manufacturers and brands.
  - `catalog.products` — Tenant-owned master product records with full 34-column legacy parity (`dosage_form`, `scientific_name`, `pharmacology`, `active`, `concentration`, `unit`, `price`, `discount`).
  - `catalog.product_variants` (renamed from legacy `product_childerns`) — Specific packaging, concentration, or SKU variations.
  - `catalog.product_infos` & `catalog.product_clients` — Metadata extensions and custom vendor mappings.

## Invariants & Rules

1. **Row Level Security:** Enforced with `FORCE ROW LEVEL SECURITY` on `catalog.products`, `catalog.product_variants`, `catalog.product_infos`, and `catalog.product_clients` using `platform.tenant_visible(organization_id)`.
2. **Arabic Trigram Search:** Indexed with `pg_trgm` GIN indexes over `platform.normalize_arabic(name->>'ar')`. Matches Arabic search terms regardless of alef, teh marbuta, or yah variations.
3. **Monetary Precision:** All prices and discounts are stored and computed via `money.Amount` (minor unit integer arithmetic).

## Endpoints

- `GET /api/v1/catalog/search?q={query}&category_id={id}&brand_id={id}` — Fuzzy Arabic product search.
- `GET /api/v1/catalog/products/{id}` — Retrieve product details with all active variants.
- `POST /api/v1/catalog/products` — Create new product for current tenant.
- `POST /api/v1/catalog/products/{id}/variants` — Create new variant under product.
- `GET /api/v1/catalog/categories` — List taxonomy categories.
- `GET /api/v1/catalog/brands` — List manufacturers and brands.
