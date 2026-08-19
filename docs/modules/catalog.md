# Module: Product Catalog

## Overview

The `catalog` bounded context manages pharmaceutical products, packaging variants, hierarchical categories, brands, and Arabic trigram search.

## Schema Mapping

- **PostgreSQL Schemas:** `catalog`
- **Migrations:** `004_catalog.up.sql`, `083_saving_products.up.sql`, `084_employee_institutional_works.up.sql`, `085_product_index.up.sql`
- **Tables Owned:**
  - `catalog.categories` — Hierarchical product taxonomy with bilingual names.
  - `catalog.brands` — Pharmaceutical manufacturers and brands.
  - `catalog.products` — Tenant-owned master product records with full 34-column legacy parity (`dosage_form`, `scientific_name`, `pharmacology`, `active`, `concentration`, `unit`, `price`, `discount`, `institutional_work_ids`).
  - `catalog.product_variants` (renamed from legacy `product_childerns`) — Specific packaging, concentration, or SKU variations.
  - `catalog.product_infos` — Legacy 5-column key-value attribute bag (`id`, `product_id`, `info_key`, `info_value`, `created_at`).
  - `catalog.product_index` — High-performance 25-column denormalized read model for faceted search and instant product discovery.
  - `catalog.customer_product_mappings` — One per-customer pricing/mapping table (071): vendor-set pricing (`price`, `discount` percent, `customer_org_id`) and import rows (`raw_name`, `branch_id`, `source` `excel|csv|link|manual`, `status`).
  - `catalog.saving_products` — Customer/vendor savings tracking lists (reinstated in 083 per Plan V5 Phase 0 Task 0.6.1).

## Invariants & Rules

1. **Row Level Security & Marketplace Cross-Tenant Read:** Enforced with `FORCE ROW LEVEL SECURITY` on all tenant tables using `organization_id = app.current_tenant`. `catalog.product_index` has RLS enabled, and customer-facing discovery queries explicitly execute under `database.AsSystem` with code-level comments documenting that cross-tenant catalogue read is the explicit invariant of a multi-vendor marketplace.
2. **Name Collision Invariant (Plan V5 Phase 1 Task 1.2):**
   - Laravel's `product_infos` is a 25-column denormalized read model (also backing `all_products` view and `product_search_index`).
   - In Go, this read model is named `catalog.product_index` to prevent collision with Go's pre-existing 5-column `catalog.product_infos` key-value attribute bag.
3. **Deterministic Fallback (Rule R3 Spirit):** If the `catalog.product_index` read model is empty or stale, `catalog.Service.FastSearch` automatically falls back to querying `catalog.products` directly rather than returning an empty result.
4. **Asynchronous Read Model Synchronization:**
   - Background updates are dispatched as River worker jobs (`catalog.reindex` via `internal/modules/catalog/jobs/reindex.go`) on product/variant/stock/branch mutations.
   - Batch full-rebuilds are executed via `dawa24 cli reindex` or `RebuildProductIndex`.
5. **Arabic Trigram & Fulltext Search:** Indexed with `pg_trgm` GIN indexes over `search_simple` (generated via `internal/shared/arabic.Normalize`) and Postgres `tsvector` over `(search_text, search_simple)`.
6. **Monetary Precision:** All prices and discounts are stored and computed via `money.Amount` (minor unit integer arithmetic).
7. **Saving Products Decision (Task 0.6.1):** `catalog.saving_products` is reinstated as a distinct customer/vendor-owned list of products with quantity and price used for savings tracking and procurement optimization, distinct from time-boxed promotional discount offers.

## Endpoints

- `GET /api/v1/catalog/search?q={query}&category_id={id}&brand_id={id}` — Fuzzy Arabic product search.
- `GET /api/v1/catalog/products/{id}` — Retrieve product details with all active variants.
- `POST /api/v1/catalog/products` — Create new product for current tenant.
- `POST /api/v1/catalog/products/{id}/variants` — Create new variant under product.
- `GET /api/v1/catalog/categories` — List taxonomy categories.
- `GET /api/v1/catalog/brands` — List manufacturers and brands.
