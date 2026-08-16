# Dawa24 Store — Engineering Handoff

**For:** the next agent or developer continuing this rebuild
**Repo state at handoff:** commit `b23fb42`, `main`, builds clean, tests pass
**Written:** 2026-08-16

Read Part 0 through Part 5 before writing any code. They are short, and skipping
them will cost you more time than reading them.

---

# PART 0 — What this project is

Dawa24 is an **Arabic-first B2B pharmaceutical marketplace** for the Egyptian
market. Pharmacies and clinics buy from suppliers and distributors.

There is an **existing, working Laravel 12 application** that implements the
whole thing. This repository is a **ground-up rewrite of that application in Go**.
It is the same product, the same business rules, and eventually the same data —
a different codebase, not a different system.

**Current reality: the rewrite is roughly 5% complete.** What exists is
infrastructure. There is no marketplace, no login, no products, no orders, no UI.
Deployed right now, the service answers `/health`, `/ready`, `/api/v1/status`
and `/` with JSON, and nothing else.

**The Laravel app remains the only functioning marketplace.** Do not treat this
repo as a replacement, and do not let anyone switch production traffic to it.

## Actors in the domain

From `users.role` in the legacy schema:
`admin` · `owner` · `manager` · `employee` · `vendor` · `customer` · `developer`

Suppliers are modelled as **organizations** with **branches** and **warehouses**.
Organizations are the tenant boundary.

---

# PART 1 — Reference map

Everything below is an absolute path on the machine this was built on.

## The three codebases

| What | Path | Role |
|---|---|---|
| **Legacy Laravel app** | `F:\Dawa 24\Laravel` | The source of truth for all business logic. **Read it constantly.** Not in git. Never deployed. |
| **This rewrite** | `F:\Dawa 24\dawa24-store` | → `github.com/muhiyatools/dawa24-store` |
| **AI Gateway** | `F:\MuhiyaWorkspace\MuhiyaWorkspace` | Existing Go service, 40+ migrations. **Do not fork it.** Deploy it a second time. |

## Planning and audit documents

| Document | Path | What it contains |
|---|---|---|
| **Master plan** | `F:\Dawa 24\docs\rebuild\REBUILD_MASTER_PLAN.md` | The full audit and target architecture. §2 legacy DB audit, §5 target schema, §11 data migration, §13 phase roadmap. |
| **Legacy schema inventory** | `F:\Dawa 24\docs\rebuild\SCHEMA_INVENTORY.md` | Every one of the 141 tables + 7 views: columns, types, indexes, FKs. **3,734 lines. Use this instead of re-parsing the SQL dump.** |
| **Raw SQL dump** | `F:\Dawa 24\u924222867_Testv5.sql` | MariaDB 11.8 schema-only dump, 6,804 lines. Source for the inventory above. |
| **ADRs** | `dawa24-store/docs/adr/decisions.md` | Five decisions with their reasoning. Read before proposing an alternative. |
| **Agent conventions** | `dawa24-store/AGENTS.md` | The working rules. Non-negotiable section is short. |
| **Deployment** | `dawa24-store/DEPLOY.md` | Elest.io steps. |
| **Ops / infra** | `F:\Dawa 24\dawa24-ops\` | Compose files and pgAdmin bootstrap SQL per pipeline. |

## Where to look in the Laravel app

Business logic lives in **Livewire components**, not in services. This is the
single most important fact about reading the legacy code.

| Concern | Legacy path |
|---|---|
| Business logic (353 components) | `Laravel\app\Livewire\{Admin,Customer,Employee,Front,Pages}` |
| Views (514 templates) | `Laravel\resources\views` |
| Models (114) | `Laravel\app\Models` |
| Services (~32, partially used) | `Laravel\app\Services` |
| Routes | `Laravel\routes\{web,admin,vendor,customer,api}.php` |
| **Globally autoloaded helpers (13 files)** | `Laravel\app\Helpers` — loaded via composer `files[]`, so these are global functions used everywhere |
| Queued jobs (8) | `Laravel\app\Jobs` |
| Middleware (18) | `Laravel\app\Http\Middleware` |
| **The one well-built module** | anything under an `Offers` subdirectory — use as the quality reference |

---

# PART 2 — What is DONE

All of this is committed, compiles, is `go vet` clean, and is formatted.
**Verified at commit `b23fb42`.**

## Shared primitives (`internal/shared/`) — dependency-free leaves

| Package | File | State |
|---|---|---|
| `money` | `money.go` + `money_test.go` | **Complete. 10 tests.** Exact int64 minor units. `Parse`, `Add`, `Sub`, `MulInt`, `ApplyPercent` (basis points, half-up away from zero), `Allocate` (splits without losing a unit), SQL `Valuer`/`Scanner`, JSON as string. |
| `arabic` | `arabic.go` + `arabic_test.go` | **Complete. 11 tests.** Faithful port of `Laravel\app\Services\ArabicNormalizer.php`, including the exact similarity curve. |
| `i18n` | `i18n.go` | **Complete.** `Text` = `map[Lang]string` for the `{"ar","en"}` JSONB columns. RTL helpers. No tests yet. |
| `apperr` | `apperr.go` | **Complete.** Closed error-kind set mapped to HTTP by `httpx`. No tests yet. |

## Platform (`internal/platform/`) — infrastructure

| Package | File | State |
|---|---|---|
| `config` | `config.go` | **Complete.** Typed, fail-fast, aggregates every problem. Prod-only strictness. No tests. |
| `database` | `database.go` | **Complete.** pgx pool + **the tenant isolation contract**: `InTx`/`InReadTx` issue `SET LOCAL app.current_org_id`. `AsSystem` for explicit cross-tenant. Error classifiers. |
| `database` | `migrate.go` | **Complete.** Embedded, checksummed, advisory-locked migration runner. Refuses edited migrations. |
| `cache` | `cache.go` | **Complete.** Redis. **All keys tenant-namespaced.** `Remember[T]` generic. `InvalidateTenant` via SCAN. |
| `gateway` | `gateway.go`, `errors.go`, `disabled.go` | **Complete.** Capability-based client, per-capability budgets, circuit breaker, closed error set, `ShouldFallback`, `Disabled` no-op implementation. **No tests.** |
| `httpx` | `middleware.go`, `respond.go` | **Complete.** RequestID, Recover, Logger, SecurityHeaders+CSP, Locale. JSON error envelope, strict `DecodeJSON`. |
| `observability` | `log.go` | **Complete.** slog JSON, context-aware, **redacts sensitive keys**. |

## Entrypoints (`cmd/`)

| Binary | Files | State |
|---|---|---|
| `server` | `main.go`, `health.go`, `deps.go` | **Complete for what exists.** Starts HTTP *before* dependencies connect; dials PostgreSQL and Redis in background with capped backoff. Routes: `/`, `/health`, `/ready`, `/api/v1/status`. |
| `worker` | `main.go` | **Skeleton.** River client, queue config, River self-migration, graceful shutdown, one heartbeat job. **Nothing enqueues jobs yet.** |
| `cli` | `main.go` | **Complete for now.** `migrate`, `migrate-status`, `health`. |

## Database (`db/migrations/`)

| Migration | State |
|---|---|
| `001_foundation` | Extensions, 13 schemas, **`platform.tenant_visible()` RLS helper**, `platform.normalize_arabic()` (SQL mirror of the Go normaliser), `touch_updated_at()`, `audit_log`, `settings`. |
| `002_identity` | `users` decomposed into `users`/`user_security`/`user_mfa`/`user_identities`/`kyc_records`/`account_deletion_requests` + `profile.user_profiles`/`user_preferences`. Unified RBAC (`roles`/`permissions`/`role_permissions`/`user_roles`). Seeds 8 roles, 12 permissions. |
| `003_organizations` | `org.organizations`, `org.branches`, `org.members`. **Establishes the RLS pattern** — copy it verbatim for every tenant-owned table. |

**⚠ Never verified against a live PostgreSQL.** Docker was unavailable during the
build session. The SQL is written carefully but is **unproven**. The first run of
`cli migrate` is a real test — expect to fix syntax there.

## Infrastructure and docs

`Dockerfile` (multi-stage, non-root, 3 binaries) · `docker-compose.yml`
(migrate→server→worker) · `docker-compose.dev.yml` · `.dockerignore` ·
`.gitattributes` (LF-pinned, protects migration checksums) · `.golangci.yml`
(depguard boundary rules) · `Makefile` · `.github/workflows/ci.yml` ·
`AGENTS.md` · `README.md` · `DEPLOY.md` · `docs/adr/decisions.md`

---

# PART 3 — What is NOT done

## Referenced in docs but does not exist — create when needed

- `sqlc.yaml` — README describes it; never written. Phase A task.
- `internal/platform/storage/` — S3/MinIO. `config.Storage` exists, no code.
- `internal/platform/queue/` — River is wired directly in `cmd/worker/main.go`.
- `internal/modules/` — **the entire directory does not exist.**
- `docs/modules/` — one page per module. Empty.
- `.github/workflows` is committed but `make` was unavailable locally, so the
  `check-provider-isolation` and `check-file-size` targets have **only been run
  manually**, never through CI.

## Not started

Everything that makes it a marketplace:

- **All business modules**: catalog, inventory, commerce, promo, billing,
  ingest, workflow, hr, platform_admin
- **Authentication handlers** — the schema exists; login/logout/session/MFA
  flows do not
- **The entire UI** — no templ, no HTMX, no Tailwind, no layouts, no components
- **The legacy data ETL** — MariaDB → PostgreSQL
- **Migrations 004 onwards** — 138 of 141 legacy tables have no target schema yet
- **Tests** beyond `money` and `arabic`
- **The RLS cross-tenant test suite** — mandated by ADR 0003, not written

## Honest estimate

~26–29 weeks of remaining work for one developer. The audit's total was ~30
weeks; three are done.

---

# PART 4 — Rules you must not break

Full version in `AGENTS.md`. The ones that cause real damage if ignored:

1. **Money never touches `float64`.** Use `money.Amount`. Database columns are
   `NUMERIC(p,2)`.

2. **No AI provider name outside `internal/platform/gateway/`.** Not `openai`,
   `anthropic`, `deepseek`, `gemini`, `groq`, `openrouter`. The Store asks for a
   *capability*. `make check-provider-isolation` enforces it.

3. **Every AI capability ships with a deterministic fallback in the same change.**
   A pharmacy must be able to order when the Gateway is down.

4. **Tenant-scoped queries go through `db.InTx` / `db.InReadTx`.** Never
   `db.Pool()` in a module. Cross-tenant work uses `database.AsSystem(ctx)`.

5. **Module boundaries** — `modules/A` must not import `modules/B`. `platform/`
   must not import `modules/`. `shared/` imports nothing internal. depguard
   enforces this.

6. **400 lines per Go file.**

7. **Never edit an applied migration.** The runner checksums them and refuses.
   Add a new one.

8. **Preserve legacy behaviour exactly during migration.** Business rules,
   primary key values, `order_number` formats, money semantics. Port bugs as-is
   and record them in `docs/modules/<module>.md`. Parity against the old system
   is how correctness is proven; "improving" a rule destroys that.

---

# PART 5 — Audit findings you must know before touching the schema

These came out of the legacy database audit. Each one is a trap.

| ID | Finding | Consequence for you |
|---|---|---|
| **D1** | `users` has **137 columns** spanning 11 concerns, including `base_salary` next to `password` | Already fixed in `002_identity`. Do not reintroduce. |
| **D2** | **Two complete, parallel order systems**: `orders`+`order_items` (5 statuses) and `main_orders`+`adv_orders` (13 statuses) | Phase D must unify into `orders`→`order_shipments`→`order_lines`. **Determine which is authoritative first** by row counts and recent `created_at`. |
| **D3** | **Two 2FA implementations** on one row: `two_factor_*` and `google2fa_*` | `002_identity` keeps one (`google2fa_*`, the one with a live service). ETL must reconcile rows where they disagree. |
| **D4** | **`stocks` unique constraint is wrong**: `UNIQUE(product_id, warehouse_id)` while `product_childern_id` is `NOT NULL` | Two variants of one product **cannot coexist in a warehouse**. Silent import failures. Target must be `UNIQUE(warehouse_id, product_variant_id)`. |
| **D5** | `temporary_file_uploads.file_content` is a **BLOB in the OLTP database** | Move to object storage in Phase F. |
| **D6** | **7 views used as tables**, no PK, doing runtime `json_extract` over joins: `all_products`, `products_view`, `product_importants`, `organizations_overview`, `basic_products`, `offer_importants`, `product_infos` | Cannot be indexed. Replace with materialised views or worker-refreshed projections. |
| **D7** | **Four overlapping subscription systems** + `session_plans` | Collapse into one entitlement model. Keep `source_system` on each row for auditability. |
| **D8** | **Two RBAC systems** plus `users.he_can_do_anything` and `full_powers` | Unified in `002_identity`. ETL must produce a **reconciliation report** of every case where the two disagreed. |
| **D11** | Denormalized counters with no transactional guard: `users.total_orders`, `total_spent`, `products.sold_times`, `organizations.rating`/`rank` | Rebuild as recomputable projections. |
| **D12** | **36 `*_id` columns with no FK constraint** | Orphans already exist. ETL needs a pre-migration orphan sweep. |
| **D13** | Misspellings in the schema: `product_childerns`, `user_temparte_warehouses`, `father_user_temparte_warehouses`, `removed_from_new_clinet_at` | Renamed in target (see master plan §5.6). Legacy names reachable via `compat` views during migration only. |

## What the legacy system got RIGHT — preserve it

- **Money is `DECIMAL` everywhere** (106 columns). Only geo coordinates are float.
- **Bilingual JSON** `{"ar","en"}` with `json_valid` CHECK — ~173 columns. Maps to JSONB.
- **Price snapshotting on order lines** — `order_items.price`, `adv_orders.old_product_childern_price`.
- **Arabic column comments** — often the only documentation. Carry them into `COMMENT ON COLUMN`.
- **The `Offers` module** — the one place with FormRequests, Resources, Policies, Enums, Events, Observers.

## Most-referenced tables (migration ordering)

```
users (91 inbound FKs) → organizations (41) → products (13) → organization_branches (10)
→ offers (8) → product_childerns (5) → cities (5) → plans (4)
```

---

# PART 6 — Remaining phases

Phases are lettered to avoid confusion with the master plan's numbering. Each is
several sessions. **Do one at a time and finish it.**

---

## PHASE A — Close the foundation gaps
**Prereq:** none. **Do this first.**

### Tasks
1. **Verify migrations against a real PostgreSQL.** `docker compose -f docker-compose.dev.yml up -d`, then `go run ./cmd/cli migrate`. Fix whatever breaks. This has never been run.
2. Write `sqlc.yaml` — engine postgres, pgx/v5, query dir `db/queries`, output per module into `internal/modules/<name>/postgres`. Override `NUMERIC` → `money.Amount`, `JSONB` name columns → `i18n.Text`.
3. Create `internal/platform/storage/storage.go` — S3 SDK v2, path-style for MinIO, `Put`/`Get`/`Presign`/`Delete`. Keys namespaced by `org_id`.
4. Create `internal/platform/queue/queue.go` — extract River setup out of `cmd/worker/main.go` so modules can enqueue inside a transaction.
5. Write the **RLS cross-tenant test suite** (`test/integration/rls_test.go`): for every tenant-owned table, insert as org A, read as org B, assert zero rows. **This is a CI gate per ADR 0003.**
6. Add tests for `i18n` and `apperr`.
7. Confirm CI actually runs green on GitHub.

### Done when
`make check` passes on a machine with `make`; CI green; migrations apply and re-apply idempotently; RLS suite passes.

---

## PHASE B — Authentication and identity
**Prereq:** A. **Schema already exists in `002_identity`.**

### Tasks
1. `internal/modules/identity/` — `domain.go`, `service.go`, `repository.go`, `postgres/`, `http/`, `views/`.
2. **Verify bcrypt compatibility before anything else.** Laravel hashes are `$2y$`; Go's `golang.org/x/crypto/bcrypt` verifies them (`$2y$` and `$2a$` are the same algorithm). **Test against 100 real hashes from the legacy DB.** If this fails, every user needs a password reset and the migration plan changes.
3. Session store in Redis; cookie `Secure`/`HttpOnly`/`SameSite=Lax`.
4. Login, logout, password reset, email verification.
5. TOTP MFA against `identity.user_mfa`; encrypt secrets with pgcrypto.
6. Login rate limiting + account lockout using `identity.user_security`.
7. Authorization middleware resolving effective permissions from `roles`/`role_permissions`/`user_roles`/`org.members`.
8. Tenant-resolution middleware → `database.WithTenant(ctx, orgID)`.

### Read in legacy
`Laravel\app\Http\Middleware\` (all 18), `Laravel\app\Services\Google2FAService.php`, `Laravel\app\Traits\HasPermissions.php`, `Laravel\app\Helpers\role_permission.php`, `Laravel\app\Helpers\admin_role_permission.php`, `Laravel\app\Livewire\Auth\`

### Done when
A real legacy user logs in with their existing password. Permission checks match the legacy matrix for every (role, resource, action) triple.

---

## PHASE C — Catalog and inventory
**Prereq:** B. **Migrations 004–005.**

### Tasks
1. Migration `004_catalog`: `categories`, `brands`, `products`, `product_variants` (was `product_childerns`), `product_infos`, `product_clients`, `product_alerts`, `customer_product_mappings`. RLS on all. Trigram GIN indexes over `platform.normalize_arabic(name->>'ar')`.
2. Migration `005_inventory`: `warehouses`, `stocks`, `stock_movements`, `warehouse_transfers`, `temp_warehouses`, `temp_warehouse_lines`. **Fix D4**: `UNIQUE(warehouse_id, product_variant_id)`.
3. `internal/modules/catalog/` and `internal/modules/inventory/`.
4. Arabic search using `platform.normalize_arabic` + `pg_trgm`. Keep it in Postgres; no external search engine until the ADR 0002 trigger is hit.
5. Stock movement ledger — every quantity change writes a movement row in the same transaction.
6. Vendor product CRUD + admin catalog screens.

### Read in legacy
`Laravel\app\Models\{Product,ProductChildern,Stock,StockMovement,Warehouses}.php`, `Laravel\app\Services\{FastSearchService,ProductMatcher,WarehouseLifecycleService}.php`, `Laravel\app\Livewire\Admin\{AdminProductShow,AdminProductChildern,BranchProducts}.php`

### Watch for
- Legacy `products` has **34 columns** mixing catalog and pharma-specific fields (`dosage_form`, `pharmacology`, `scientific_name`, `active`, `concentration`, `barcode`). Keep them — this is a regulated domain.
- `products.price` and `product_childerns.price` both exist. Determine precedence from the Livewire components before designing the target.

### Done when
Catalog search parity on 500 real queries vs the legacy app; search P95 < 300 ms; a variant can exist in a warehouse alongside its siblings (D4 proven fixed).

---

## PHASE D — Commerce
**Prereq:** C. **Migration 006. Highest-risk phase.**

### Tasks
1. **Resolve D2 first.** Query both legacy order systems for row counts and recent activity. Document the answer in `docs/modules/commerce.md` before writing any schema.
2. Migration `006_commerce`: `carts`, `orders` (was `main_orders`), `order_shipments` (was `orders`), `order_lines` (was `adv_orders` + `order_items`), `order_status_history`, `invoices`.
3. One canonical status enum, superset of both legacy sets, plus a legacy→target mapping table used by the ETL.
4. `internal/modules/commerce/` — cart, checkout, order placement, status machine, vendor fulfilment.
5. **Price snapshotting** on every line. Use `money.Allocate` to split totals across shipments — never percentage rounding per part.
6. Order status transitions validated by an explicit state machine, with history written in the same transaction.

### Read in legacy
`Laravel\app\Models\{Order,OrderItem,MainOrder,AdvOrder,CartWishlist,Invoice}.php`, `Laravel\app\Services\OrderService.php`, `Laravel\app\Http\Controllers\OrderController.php`, `Laravel\app\Observers\MainOrderObserver.php`, `Laravel\app\Http\Requests\ConfirmOrderRequest.php`

### Done when
1,000 historical orders reprice **to the cent**. Every order's `SUM(lines.line_total) = order.final_price`.

---

## PHASE E — Billing, payments, entitlements
**Prereq:** D. **Migration 007.**

> **⚠ BLOCKING UNKNOWN.** `payments`, `payment_histories`, `user_payment_methods` and a **47-column `payment_integrations`** table exist, but **`composer.json` contains no payment SDK**. Determine whether payments are manual, custom HTTP to a local processor (Paymob/Fawry?), or unimplemented. **This changes the phase from 1 week to 4.** Resolve by inspecting `payment_integrations` rows and the vendor payment Livewire components.

### Tasks
1. Migration `007_billing`: `wallets`, `wallet_transactions`, `payments`, `payment_histories`, `payment_integrations`, `user_payment_methods`, `plans`, `plan_features`, `subscriptions`, `entitlements`.
2. **Collapse the four subscription systems (D7)** into one model with `source_system`/`source_id` provenance.
3. `entitlements.Check(subject, feature_key)` — the single question application code asks.
4. Wallet ledger: append-only, balance is a projection, never a mutable column.

### Done when
Wallet balances match legacy exactly. Every legacy subscription maps to exactly one target subscription with recorded provenance.

---

## PHASE F — Ingest pipeline
**Prereq:** C. **Migration 008.** Can run parallel to E.

### Tasks
1. Migration `008_ingest`: `import_batches`, `import_sessions`, `import_rows`, `import_progress`, `file_uploads`. **Files go to object storage (D5), not a BLOB column.**
2. River jobs on the `imports` queue, chunked.
3. **Column detection**: heuristic matcher first, `import.detect_columns` capability only for what the heuristic cannot resolve.
4. **Product matching**: `arabic.Similarity` + Postgres trigram **first**; escalate only the non-matches to `product.match`. This is the largest AI cost saving available — expect it to remove 60–80% of calls.
5. Honour `import_sessions.min_similarity_score` (legacy default **0.85**, tuned against the exact curve in `internal/shared/arabic`).
6. SSE progress endpoint.

### Read in legacy
`Laravel\app\Jobs\` (all 8), `Laravel\app\Services\{ColumnDetector,ProductMatcher,ArabicNormalizer}.php`, `Laravel\app\Traits\HandlesExcelImport.php`

### Done when
A real vendor file imports with the same matches as the legacy importer, **and** the whole flow works with `GATEWAY_ENABLED=false`.

---

## PHASE G — Promo / offers / ads
**Prereq:** D. **Migration 009.**

The legacy `Offers` module is the best-built part of the old system. **Port its structure, do not redesign it.**

Tables: `offers`, `offer_products`, `offer_packages`, `offer_package_features`, `offer_promotions`, `offer_sponsorships`, `offer_views`, `offer_clicks`, `offer_location_covers`, `ads`, `ad_clicks`, `ad_plans`, `highlight_sections`.

Port `OfferRotationEngine`, `OfferSponsorshipService`, `OfferViewTrackingService`, `OfferClickTrackingService`, `OfferAnalyticsService`, and `ProcessExpiredSponsorshipsCommand` (→ River periodic job).

**Done when** offer pricing matches legacy for every active offer.

---

## PHASE H — The UI
**Prereq:** B onwards; build incrementally alongside each module.

1. templ + HTMX + Alpine + Tailwind 4. **Design RTL first**, verify LTR second.
2. Design tokens once: 6 spacing steps, 5 type sizes, 3 weights, one accent, one radius. No arbitrary values in templates.
3. Three shells: Admin, Vendor, Customer.
4. Tables are the product — density, sticky headers, sorting, saved filters, keyboard nav.
5. Money right-aligned, `tabular-nums`, always 2 decimals and a currency.
6. Every state designed: loading (skeleton), empty (with the action that fills it), error (with recovery), partial.
7. Budget: < 40 KB JS total, FCP < 1.2 s on 3G.
8. **No gradients, no glassmorphism, no decorative icons, no emoji.**

---

## PHASE I — Remaining domains
`workflow` (automation requests, `PurchasePriorityEngine`, `WeeklyCoverage`, `InstitutionalWork`), `hr` (employees, `work_times`, job board), `platform_admin` (settings, translations, countries/cities/currencies, system resources).

---

## PHASE J — The legacy data ETL
**Prereq:** all schema phases. **Master plan §11 is the detailed spec.**

A Go program under `cmd/cli` (`migrate-data`) — testable, resumable, idempotent. Six stages: Extract → Validate → Transform → Load → Verify → Reconcile.

**Preserve legacy primary key values.** User 4417 stays user 4417.

Verification gates (all must pass per table): row counts exact · checksums · money sums **to the cent** · zero FK orphans · 100% JSON parses · timestamps match after explicit UTC→TIMESTAMPTZ conversion · business invariants · 100 orders spot-checked by a human.

---

## PHASE K — AI capabilities
**Prereq:** F, and a Gateway virtual key.

Wire the six capabilities in `internal/modules/aicapabilities/`. Prompts versioned in-repo. **Write the black-hole test suite**: run the full order and import flows with `GATEWAY_BASE_URL` pointed at an unroutable address and assert nothing user-facing breaks.

---

## PHASE L — Cutover
**Prereq:** everything. **Master plan §11.5 and §13.**

Write and **test** the reverse-ETL (PostgreSQL → MariaDB) *before* cutover. Rehearse twice on staging. Point of no return is the first write accepted by the Go app.

---

# PART 7 — How to work in this repo

## Every session

1. Read `AGENTS.md` and this file's Part 4 and Part 5.
2. Read `docs/modules/<module>.md` if it exists for what you are touching.
3. **Read the legacy code before writing the Go code.** Business rules exist only there.
4. Work on **one module**. Do not touch two.
5. Run `make check` (or the raw `go` commands) before committing.
6. **Update this file's Part 2 and Part 3** so the next session starts accurate.
7. Write `docs/modules/<module>.md` as you go: entities, invariants, events, and **every legacy quirk you decided to preserve**.

## Verification commands

```bash
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .
go run ./cmd/cli migrate-status
```

`make` was not installed on the original machine. On Windows use the raw
commands above; CI runs on Ubuntu where `make` works.

## Provider-isolation check (run manually if `make` is unavailable)

```bash
grep -riE '\b(openai|anthropic|deepseek|gemini|groq|openrouter)\b' --include='*.go' --include='*.sql' ./cmd ./internal ./db | grep -v '^./internal/platform/gateway/' | grep -v '_test.go'
```

Any output is a build-breaking violation.

---

# PART 8 — Open questions blocking later phases

| ID | Question | Blocks | How to resolve |
|---|---|---|---|
| **U1** | Is the Laravel app live in production with real vendors and money? | Phase L strategy — convert-and-cut vs strangler. **~8 weeks difference.** | Ask the owner; check MariaDB row counts |
| **U2** | How are payments actually settled? No SDK in `composer.json`. | Phase E scope (1 week vs 4) | Inspect `payment_integrations` rows + vendor payment Livewire |
| **U3** | Does any code path execute **LLM-generated SQL**? | Phase K, and a possible **critical vulnerability in the live system** | Read `SqlQueryHistory`, `ChatTree`, `AiTest` Livewire components |
| **U4** | Which of the 4 subscription systems are actually in use? | Phase E mapping | Row counts per family |
| **U5** | Which of the 2 order systems is authoritative? | **Phase D — blocking** | Row counts + recent `created_at` in both |
| **U6** | Are `routes/old_admin.php` / `old__vendor.php` registered? | Feature inventory | Read `Laravel\bootstrap\app.php` |
| **U10** | Is the `Testv5` dump representative of production schema? | All schema phases | Diff against production `SHOW CREATE TABLE` |

---

# PART 9 — Deployment status

- **Store:** never successfully deployed. Build previously failed on a missing
  `internal/ui/static` (fixed, `d5c46ea`). Needs PostgreSQL provisioned via
  `dawa24-ops/postgres/01-bootstrap.sql` and `02-extensions.sql`.
- **Gateway:** an instance is running at the `dawa24-store-u74003.vm.elestio.app`
  domain — **this is the Gateway, not the Store**, deployed against production
  Gateway data. Its `/admin` panel is publicly reachable and should be locked
  down or removed.
- **Ports** (chosen to avoid an existing project): Postgres `5442`, Redis `6389`,
  MinIO `9010`/`9011`, Gateway `8091`, Store `8081` (the working compose
  currently uses `8070` — match it to the reverse proxy target).
- **Redis** is deployed at `redis2-u74003.vm.elestio.app:26381`. Its password was
  exposed in a chat transcript and **should be rotated**.
