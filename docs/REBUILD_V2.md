# Dawa24 — Rebuild V2

**Measured:** 2026-08-17 against the live database (125 tables) and the 141-table
Laravel schema. Every claim below came from a query or a grep.

This plan does three things, in this order:

1. **Fixes the authorization hole** that makes the product behave randomly.
2. **Consolidates the database** from 125 tables, removing the ~22 that model the
   same concept twice.
3. **Rebuilds commerce on Laravel's actual design**, which is offer-centric and
   coverage-gated — not the product-centric model that was built here.

---

# PART 0 — Root cause

## 0.1 — There is no authorization on the web UI. At all.

```go
// cmd/server/routes.go:139
r.Group(func(uiRouter chi.Router) {
    uiRouter.Use(identityHttp.OptionalAuth(...))   // <-- Optional
    uiHandler.RegisterPageRoutes(uiRouter)
})
```

Measured:

| | Count |
|---|---|
| Middlewares on the UI router | **1** (visitor analytics) |
| Admin UI handlers | **39** |
| Role checks inside those handlers | **0** |
| `/admin/*` UI routes | **44** |
| `/vendor/*` UI routes | **31** |
| `/pharmacy/*` UI routes | **1** |

Consequences, worst first:

1. **Any signed-in user can open `/admin/users`.** It calls `AdminListUsers`
   under `database.AsSystem`, which deliberately bypasses tenant scoping — so a
   pharmacy account sees every user on the platform. The same holds for
   `/admin/approvals`, `/admin/organizations` and 41 more.
2. **A pharmacy account can open all 31 `/vendor/*` pages.** This is the bug you
   reported: "branches" lands on a vendor screen because there is no pharmacy
   equivalent and nothing stops the vendor one rendering.
3. **The pharmacy experience barely exists** — 31 vendor routes against 1
   pharmacy route.

The API side is properly guarded (`RequireAuth` + `RequirePermission`). **The
HTML side is not.** Everything else in this document is downstream of that.

## 0.2 — The database models the same concept several times

| Concept | Tables now | Should be |
|---|---|---|
| Roles | `identity.roles`, `identity.role_permissions`, `identity.user_roles`, `org.roles`, `org.role_permissions`, `org.custom_roles`, `org.members.role_key` | one org-role table + one permission map |
| Highlight sections | `promo.highlight_sections/_items` **and** `org.highlight_sections/_items` | one, with an owner column |
| Offers | `promo.offers/_products/_views/_clicks/_promotions/_sponsorships/_packages/_location_covers` **and** `promo.special_offers/_products/_locations` | one offer family |
| Policies | `platform_admin.policies`, `platform_admin.privacy_policies`, `org.organization_policies` | one platform + one org |
| Settings | `platform.settings`, `platform_admin.system_settings` | one |
| Employees | `hr.employees`, `org.members` | `org.members` |
| Attachments | `platform_admin.documents`, `ingest.file_uploads` | one documents table |
| Customer↔product | `catalog.product_clients`, `catalog.customer_product_mappings`, `catalog.saving_products` | one mapping table |

Roughly **22 tables of pure duplication**.

## 0.3 — Commerce does not match Laravel

Laravel sells **offers**, not products:

```
organizations (vendor)
  └── branches
        └── offers                 title, discount_percentage, discount_amount,
              │                    min_order_amount, start_date, end_date,
              │                    admin_status, branch_id
              └── offer_products   product_id, product_childern_id,
                                   custom_price, custom_discount_percentage,
                                   custom_discount_amount, custom_qty

main_orders (one per checkout)  ── offer_id, total_price, total_discount,
  └── adv_orders (line items)      final_price, user_address_id
        offer_id, offer_product_id, price, discount,
        original_price, original_discount, quantity
```

**A pharmacy browses offers published by vendor branches whose weekly coverage
reaches the pharmacy's branch.** An order is placed against **one offer** — which
is why `main_orders.offer_id` exists and why an order never spans two vendors.

The Go system built a product-centric catalogue with a generic cart, which is why
"products available based on coverage and branch location" has nowhere to live.

## 0.4 — Which legacy order system is authoritative — answered

**`main_orders` + `adv_orders`.** They carry `offer_id`, `offer_product_id`, the
full 13-value status enum, and the price-history columns (`original_price`,
`original_discount`, `old_product_childern_price`) a real order needs.
`orders` + `order_items` (20/11 columns) reference no offer and no branch. **This
unblocks the ETL question that has been open since the first plan.**

---

# PART 1 — Rules

1. **Two account types only.** `customer` (صيدلية) and `vendor` (مورّد). Platform
   `admin` is staff, not an account type. Every other value — `pharmacy`,
   `chain_pharmacy`, `supplier`, `individual`, `company`, `agency`,
   `job_seeker` — is mapped onto these two or removed.
2. **Every page route declares its audience.** A route without one does not mount.
3. **No navbar.** Every authenticated screen renders inside a sidebar shell.
4. **No mock, placeholder or hardcoded data in any template.** A screen with no
   data source does not ship.
5. Money is `money.Amount`; bilingual text is `i18n.Text`; identity comes from
   `authctx`; tenant scope lives in the `WHERE` clause.
6. Every admin mutation writes `platform.audit_log` in the same transaction.

**Gate:** `templ generate && go build ./... && go vet ./...`

---

# PHASE 1 — Authorization and the shell

**Nothing else matters until this lands. Do it first and completely.**

## 1.1 — Account type as a first-class concept

**Migration `060_two_account_types.up.sql`**

```sql
UPDATE org.organizations SET type = 'vendor'
  WHERE type IN ('supplier','company','agency');
UPDATE org.organizations SET type = 'customer'
  WHERE type IN ('pharmacy','chain_pharmacy','individual');

ALTER TABLE org.organizations DROP CONSTRAINT IF EXISTS organizations_type_check;
ALTER TABLE org.organizations ADD CONSTRAINT organizations_type_check
  CHECK (type IN ('customer','vendor'));

-- A chain is a customer with several branches, not a third type.
ALTER TABLE org.organizations
  ADD COLUMN IF NOT EXISTS is_chain BOOLEAN NOT NULL DEFAULT false;
UPDATE org.organizations SET is_chain = true WHERE branch_count > 1;

-- identity.users.role becomes platform-level only.
UPDATE identity.users SET role = 'user'
  WHERE role NOT IN ('super_admin','admin','support','developer');
ALTER TABLE identity.users ADD CONSTRAINT users_role_check
  CHECK (role IN ('user','support','admin','super_admin','developer'));
```

**Customer or vendor is a property of the organization, never of the user.** The
user's platform role only says whether they are staff.

## 1.2 — `authctx.Actor` carries the account type

```go
type Actor struct {
    UserID         int64
    OrganizationID int64
    OrgType        string   // "customer" | "vendor" | "" for staff-only
    OrgStatus      string   // pending | approved | rejected | suspended
    BranchID       *int64   // non-nil when the member is bound to one branch
    Role           string   // platform role
    Permissions    []string
    IsStaff        bool
}
```

Populated by `ResolveTenant` from the session, read at login from `org.members`
joined to `org.organizations`. **Never from a header or query parameter.**

## 1.3 — Route audiences, enforced by middleware

New: `internal/platform/authctx/audience.go`

```go
func RequireCustomer(log *slog.Logger) func(http.Handler) http.Handler
func RequireVendor(log *slog.Logger)   func(http.Handler) http.Handler
func RequireStaff(log *slog.Logger)    func(http.Handler) http.Handler
func RequireApproved(log *slog.Logger) func(http.Handler) http.Handler
```

| Situation | Response |
|---|---|
| Not signed in | 302 → `/auth/login?redirect=<path>` |
| Signed in, wrong account type | **404** — a vendor should not learn `/customer/*` exists |
| Organization `pending` | 302 → `/onboarding/pending` |
| `rejected` / `suspended` | 302 → `/onboarding/pending?state=…` |
| Staff route, non-staff | **404** |

**`cmd/server/routes.go` becomes five groups:**

```go
// Public
r.Group(func(pub chi.Router) {
    pub.Use(identityHttp.OptionalAuth(...))
    ui.RegisterPublicRoutes(pub)        // /, /auth/*, /privacy, /terms, /contact
})

// Customer
r.Group(func(c chi.Router) {
    c.Use(identityHttp.RequireAuth(...), identityHttp.ResolveTenant(...))
    c.Use(authctx.RequireCustomer(log), authctx.RequireApproved(log))
    ui.RegisterCustomerRoutes(c)        // /customer/*
})

// Vendor
r.Group(func(v chi.Router) {
    v.Use(identityHttp.RequireAuth(...), identityHttp.ResolveTenant(...))
    v.Use(authctx.RequireVendor(log), authctx.RequireApproved(log))
    ui.RegisterVendorRoutes(v)          // /vendor/*
})

// Staff
r.Group(func(a chi.Router) {
    a.Use(identityHttp.RequireAuth(...), authctx.RequireStaff(log))
    ui.RegisterAdminRoutes(a)           // /admin/*
})

// Shared authenticated: /settings/*, /documents, /notifications, /messages
```

Shared screens render inside the caller's own shell, chosen from `actor.OrgType`.

## 1.4 — A guard test that cannot be bypassed

`test/route_audience_test.go` walks every route registration in `internal/ui` and
fails when a non-public path is registered outside an audience group. **A new
route with no audience fails the build.** This is the same shape as the existing
`admin_guard_test.go`, which has already caught this class once.

## 1.5 — Remove the navbar entirely

Delete the public navbar from every authenticated layout. `layouts.Base` keeps
`<head>` and nothing else. Three shells:

- `layouts.CustomerShell(title, activeNav, actor, lang, dir)`
- `layouts.VendorShell(...)`
- `layouts.AdminShell(...)`

Each renders sidebar (right in RTL) + top strip (breadcrumb, branch selector,
search, language, notifications bell, avatar menu) + `<main>`. **Every
authenticated page renders inside one of the three. No page renders `Base`
directly.** Sidebar items come from a declared menu per audience, filtered by
`actor.Can(...)`, with `activeNav` highlighting. Everything opens in `<main>`.

**Customer sidebar (صيدلية):** لوحة المعلومات · تصفح العروض · السلة · طلباتي ·
المفضلة · الفروع · الموظفون · المستندات · المحفظة والفواتير · التقييمات ·
الرسائل · الإعدادات

**Vendor sidebar (مورّد):** لوحة المعلومات · الأصناف والعروض · نطاق التغطية ·
الطلبات الواردة · المخزون · التحويلات · استيراد ملف · الفروع · الموظفون ·
المستندات · المحفظة والفواتير · التقييمات · الرسائل · الإعدادات

**Done when:** signed in as a pharmacy, `/vendor/anything` and `/admin/anything`
return 404, every sidebar item opens inside the shell, and no navbar appears
anywhere after login.

---

# PHASE 2 — Database consolidation

One migration per family: create the target, migrate the data, drop the source,
update the repository. **Never drop before the data has moved.**

## 2.1 — Roles: seven tables to two

Keep `identity.permissions` (the vocabulary), `identity.roles` for platform roles
only, and one `org.roles` + `org.role_permissions` for organization roles.

**Migration `061_roles_consolidation.up.sql`**
- Merge `org.custom_roles` into `org.roles` with `is_system = false`.
- Backfill `org.members.org_role_id` from `role_key`.
- Drop `org.custom_roles` and `identity.user_roles` — membership carries the role.
- Keep `org.members.role_key` one release, marked deprecated in a column comment.

Permission keys are `<module>.<entity>.<action>`: `catalog.offer.create`,
`commerce.order.dispatch`, `org.branch.manage`, `org.employee.manage`,
`billing.invoice.read`, `inventory.transfer.approve`, `documents.upload`,
`documents.view_all`.

**Role templates seeded per organization at registration:**

| Role | Customer | Vendor |
|---|---|---|
| `org_owner` | everything | everything |
| `org_manager` | orders, branches, employees, documents | offers, orders, inventory, branches, employees |
| `org_pharmacist` | browse, order, receive | — |
| `org_accountant` | invoices, wallet | invoices, wallet |
| `org_warehouse` | receive, stock | stock, transfers, dispatch |
| `org_sales_rep` | — | offers, customer orders |

## 2.2 — Offers: two families to one

`promo.special_offers`/`_products`/`_locations` duplicate
`promo.offers`/`_products`/`_location_covers`. Migrate the `special_*` rows into
`offers`, keep the richer column set, drop the three tables.

## 2.3 — Highlight sections

One `promo.highlight_sections` with `owner_type` (`platform` | `organization`)
and `organization_id`. Migrate `org.highlight_sections`; drop it and its items.

## 2.4 — Settings, policies, employees, attachments

- `platform.settings` → `platform_admin.system_settings`; drop the former.
- `platform_admin.privacy_policies` → `platform_admin.policies` with a
  `policy_type` column; drop the former.
- `hr.employees` columns fold onto `org.members` (`employee_code`, `job_title`,
  `base_salary`, `variable_salary`, `hired_at`); drop `hr.employees`.
- `ingest.file_uploads` → `platform_admin.documents` with
  `document_type = 'import_file'`; drop the former. **One attachment table.**

## 2.5 — Customer↔product mappings

`product_clients`, `customer_product_mappings` and `saving_products` all map a
customer's own naming and pricing onto a catalogue product. Merge into
`catalog.customer_product_mappings`: `raw_name`, `product_id`, `variant_id`,
`organization_id`, `branch_id`, `price`, `discount`,
`source` (`excel`|`csv`|`link`|`manual`), `status`.

## 2.6 — Column hygiene

After each merge:

```bash
go run ./cmd/dbcheck -nullscan "$DATABASE_URL"
go run ./cmd/dbcheck -verify   "$DATABASE_URL"
```

Then drop columns nothing reads — visible candidates: `org.organizations.rank`,
`.main`, `.first_time_upload_file`, and the social-login and API-token columns on
`identity.users`. **Grep `internal/` for the column name before dropping it.** A
column no query names is a lie about the domain.

**Target: ~95 tables, none duplicated.**

---

# PHASE 3 — The Laravel commerce model

The heart of the rebuild, and the part currently missing.

## 3.1 — Offers as the merchandising unit

**Migration `062_offer_commerce.up.sql`**

```sql
ALTER TABLE promo.offers
  ADD COLUMN IF NOT EXISTS branch_id        BIGINT REFERENCES org.branches(id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS admin_status     TEXT NOT NULL DEFAULT 'pending'
      CHECK (admin_status IN ('pending','approved','rejected')),
  ADD COLUMN IF NOT EXISTS admin_notes      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_at      TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS approved_by      BIGINT REFERENCES identity.users(id),
  ADD COLUMN IF NOT EXISTS rejected_at      TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS rejected_by      BIGINT REFERENCES identity.users(id),
  ADD COLUMN IF NOT EXISTS min_order_amount BIGINT NOT NULL DEFAULT 0;   -- minor units

ALTER TABLE promo.offer_products
  ADD COLUMN IF NOT EXISTS variant_id                 BIGINT REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS custom_price               BIGINT,            -- minor units
  ADD COLUMN IF NOT EXISTS custom_discount_percentage NUMERIC(5,2),
  ADD COLUMN IF NOT EXISTS custom_discount_amount     BIGINT,            -- minor units
  ADD COLUMN IF NOT EXISTS custom_qty                 INT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS max_qty_per_order          INT;
```

**One price resolver, used everywhere:**

```go
// EffectivePrice resolves what a pharmacy actually pays for one unit.
//
// Precedence matches Laravel: an explicit custom_price wins outright; else a
// fixed custom_discount_amount is subtracted; else custom_discount_percentage
// is applied; else the variant list price stands. The offer-level discount
// applies only when the line carries none of its own.
func EffectivePrice(v *catalog.ProductVariant, op *promo.OfferProduct, o *promo.Offer) (money.Amount, Breakdown)
```

`Breakdown` carries `ListPrice`, `DiscountAmount`, `DiscountPercent` and
`FinalPrice` so the UI can strike through "قبل الخصم" exactly as Laravel does.
**Never recompute a price in a template.**

## 3.2 — Coverage decides what a pharmacy can see

**The rule:** a pharmacy browsing for one of its branches sees offers from vendor
branches whose weekly coverage circle contains that pharmacy branch's
coordinates, on the requested day.

`workflow.weekly_coverages` still lacks the coordinates Laravel has — add
`latitude`, `longitude`, `city_id` alongside `distance_meters`, plus the
`cube`/`earthdistance` extensions and `platform.distance_meters()` (migration
`050` in the parity plan).

**The canonical visibility query — one place only:**

```sql
SELECT o.*, vb.id AS vendor_branch_id,
       platform.distance_meters(wc.latitude, wc.longitude, $2, $3) AS metres
FROM promo.offers o
JOIN org.branches      vb ON vb.id = o.branch_id
JOIN org.organizations vo ON vo.id = o.organization_id
JOIN workflow.weekly_coverages wc
       ON wc.branch_id   = vb.id
      AND wc.day_of_week = $4
      AND wc.is_active
WHERE o.status = 'active'
  AND o.admin_status = 'approved'
  AND vo.status = 'approved'
  AND vo.type   = 'vendor'
  AND (o.start_date IS NULL OR o.start_date <= CURRENT_DATE)
  AND (o.end_date   IS NULL OR o.end_date   >= CURRENT_DATE)
  AND wc.latitude IS NOT NULL
  AND earth_box(ll_to_earth(wc.latitude::float8, wc.longitude::float8), wc.distance_meters)
      @> ll_to_earth($2::float8, $3::float8)
  AND platform.distance_meters(wc.latitude, wc.longitude, $2, $3) <= wc.distance_meters
ORDER BY vo.is_sponsored DESC, metres ASC, o.created_at DESC;
```

Lives in `internal/modules/promo/postgres/visibility.go`. Every customer-facing
listing calls it. `$2`/`$3` are the **pharmacy branch's** coordinates, from
`actor.BranchID` or the pharmacy's main branch — **never from the request**.

**The branch selector belongs in the customer shell**, not on a page: a dropdown
in the top strip naming which branch you are buying for. Changing it changes what
the whole catalogue shows. Persist the choice in the session.

## 3.3 — Orders match `main_orders` / `adv_orders`

**Migration `063_order_model.up.sql`** — `commerce.orders` gains `offer_id`
(NOT NULL: an order belongs to one offer), `branch_id` (customer branch),
`vendor_branch_id` (fulfilling branch), `total_discount`, `final_price`,
`user_address_id`. `commerce.order_lines` gains `offer_product_id`,
`original_price`, `original_discount`, `list_price` — the price history Laravel
keeps so an invoice reproduces after the offer changes.

**Status enum matches Laravel exactly:** `pending, processing, confirmed,
on_hold, shipped, in_transit, out_for_delivery, delivered, completed, cancelled,
failed, returned, refunded`. Transitions are compare-and-swap and write
`commerce.order_status_history`.

**Cart:** one cart per (customer branch, offer). Adding from a different offer
starts a second cart. Checkout below the offer's `min_order_amount` is refused
with the shortfall named.

## 3.4 — Discount presentation

Match Laravel: list price struck through, discounted price prominent, a
percentage badge (خصم ١٥٪) and the saved amount (توفير ٤٥٫٠٠ ج.م). One
component, `components.PriceTag(breakdown)`, fed by `EffectivePrice`, used on
offer cards, product rows, cart lines, checkout and invoices.

---

# PHASE 4 — Documents, both sides

## 4.1 — Registration documents survive into the account

Registration uploads write `platform_admin.documents` rows carrying the
organization id, `document_type` and `status = 'pending'`. **On approval they
stay — they are not consumed.** Backfill `organizations.license_document_url`
into a documents row, then drop the column.

## 4.2 — `/customer/documents` and `/vendor/documents`

The same screen in both shells. Shows every document on the organization —
those uploaded at registration with their verification status and reviewer note,
plus anything added since. Actions: upload (type picker + presigned PUT),
preview in a modal, replace, delete a `pending` one (never a `verified` one).

Grouped by requirement so a missing mandatory document is obvious:

| Document | Customer | Vendor |
|---|---|---|
| السجل التجاري | required | required |
| البطاقة الضريبية | required | required |
| ترخيص الصيدلية | required | — |
| ترخيص هيئة الدواء | — | required |
| بطاقة الصيدلي | required | — |
| خطاب تفويض | optional | optional |

An organization missing a required document shows a persistent banner and cannot
publish offers (vendor) or check out (customer).

## 4.3 — `/admin/documents`

Filter by organization, type and status. Preview, verify, reject with a note.
Every decision writes `reviewed_by`, `reviewed_at`, `review_notes` and an audit
row in the same transaction. `/admin/approvals` shows an organization's documents
inline.

---

# PHASE 5 — Screens, per audience

Every screen is backed by a real query. **No screen ships with placeholder data.**

**Customer (صيدلية)** — `/customer/*`: `dashboard` · `offers` (coverage-filtered) ·
`offers/{id}` · `cart` · `checkout` · `orders` · `orders/{id}` · `favorites` ·
`branches` · `branches/{id}` · `employees` · `documents` · `wallet` · `invoices` ·
`reviews` · `messages` · `settings/*`

**Vendor (مورّد)** — `/vendor/*`: `dashboard` · `offers` · `offers/new` ·
`offers/{id}` · `coverage` · `orders` · `orders/{id}` · `products` · `inventory` ·
`transfers` · `ingest` · `branches` · `employees` · `documents` · `wallet` ·
`invoices` · `reviews` · `messages` · `storefront` · `settings/*`

**Admin** — `/admin/*`: `dashboard` · `organizations` · `approvals` · `documents` ·
`users` · `offers` (approve/reject) · `orders` · `products` · `categories` ·
`cities` (with coordinates) · `content` · `translations` · `messages` ·
`analytics` · `audit` · `settings` · `feature-flags`

**On the screen you reported:** `/customer/branches` lists the pharmacy's own
branches with address, coordinates, main flag, employee count and coverage
status. It renders in the **customer** shell and is unreachable from a vendor
account. That one screen is what sent you to a vendor dashboard.

---

# PHASE 6 — Data integrity sweep

For every screen, confirm and record:

1. **Every field maps to a column.** Grep the template for literals; a number or
   name that is not from a variable is a defect.
2. **Every list has a real query** with limit/offset, and totals come from
   `COUNT`, never `len()` of a capped page — that bug has now recurred three
   times in this project.
3. **Every empty state is reachable** — the query returns zero rows rather than
   an error swallowed into `nil`.

Remove `048_seed_realistic_data` from the production path. Demo rows in a real
database are indistinguishable from customer data the moment support looks.

---

# PART 2 — Execution order

| # | Phase | Blocks |
|---|---|---|
| 1 | **1.1–1.5 Authorization + shell** | everything |
| 2 | 2.1 Roles consolidation | permissions, employees |
| 3 | 3.1–3.2 Offers + coverage | the whole customer experience |
| 4 | 3.3–3.4 Orders + discounts | checkout |
| 5 | 4 Documents | approvals |
| 6 | 2.2–2.6 Remaining merges | — |
| 7 | 5 Screens | — |
| 8 | 6 Integrity sweep | ship |

**Scale:** ~10 migrations, 125 → ~95 tables, 5 route groups, 3 shells, ~55
screens, one price resolver, one visibility query.

# PART 3 — Reporting

Per session: what landed, files changed, **commands run with real output**,
anything contradicting this document with evidence, and what is blocked. Run the
gate before claiming anything is done.
