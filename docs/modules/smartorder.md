# Module: Smart Ordering (نظام الطلب الذكي)

## Overview

A pharmacy uploads a spreadsheet of what it needs. The system resolves every row
to a catalogue product, finds the vendors who can actually deliver it, picks one
per line under the buyer's criteria, and hands back an order to review and place.

It is reached from the pharmacy's **Purchase Request (طلب الشراء)** area as a
third way to start a request, alongside the supplier directory and cross-supplier
catalogue search. Those two are untouched.

Spec: `specs/001-smart-ordering-system/`.

## Schema Mapping

- **PostgreSQL schema:** `smartorder`
- **Migrations:** `123_drop_automation_requests`, `124_smartorder`,
  `125_match_decisions`, `126_product_index_variant_stock`
- **Tables owned:**
  - `smartorder.runs` — one end-to-end execution, its counters and totals.
  - `smartorder.run_config` — the configuration snapshot the run executed under.
  - `smartorder.column_mappings` — file columns to target fields, with the
    automatic guess kept alongside the confirmed mapping.
  - `smartorder.run_lines` — one row per imported spreadsheet row.
  - `smartorder.line_candidates` — every vendor offer considered, **including the
    rejected ones and why**.
  - `smartorder.line_selections` — the chosen candidate and what decided it.
  - `smartorder.run_events` — append-only progress and audit.
  - `smartorder.criteria_profiles` — the buyer's remembered defaults.
- **Tables shared with `catalog`:** `catalog.match_decisions` (adjudication
  cache), `catalog.product_aliases` (learned alias channel). Both are catalogue
  knowledge rather than buyer data, and are deliberately **not** tenant-scoped.

## Invariants & Rules

1. **Availability is never read from `catalog.product_index`.** Supplier offers
   come from `catalog.product_variants` joined to `inventory.stocks`. The read
   model's `variant_id` and `stock_quantity` were the literal NULL and 0 for all
   28,786 rows in production, so every query filtering on them returned nothing —
   silently, for every product. Migration 126 and `catalog/jobs/reindex_sql.go`
   fix the read model; this module still reads the authoritative tables.

2. **No stage issues a query per row.** Each deterministic tier is one query for
   the whole file; supplier offers for every matched product are one query. A
   ten-thousand-row import must stay inside five minutes, and per-row work is the
   single easiest way to lose that. `pipeline/retrieval` asserts the query count
   does not scale with row count.

3. **The deterministic engine is primary; AI is the last and smallest tier.**
   A line resolved at or above the **0.850 cutoff** is never sent for
   adjudication and is never overwritten by it. With AI off the run still
   completes and produces a finalisable order.

4. **Adjudication is batched, bounded and cached.** At most 25 items per request,
   5 candidates per item, 40 requests and 90 seconds per run. Every decision is
   cached on `sha256(normalised text ‖ sorted candidate ids ‖ prompt version)`,
   so a recurring file costs almost nothing the second time. A result naming a
   product outside the candidate list is rejected.

5. **Supplier selection is strict priority within a tolerance band.** The
   highest-priority enabled criterion decides, but only among candidates within
   the tolerance (default 5%) of the cheapest eligible net price. When the band
   rejects a criterion's winner, the line names the skipped supplier and by how
   much it missed. Ties break deterministically, ending in `vendor_org_id`, so
   re-running an unchanged file selects the same suppliers.

6. **Eligibility checks are ordered, and the order is the design.** own_org →
   inactive → institutional → coverage → stock → min_qty. The **first** failure
   is reported, because it is the most actionable: an offer both out of stock and
   outside coverage is a coverage problem, and saying "out of stock" sends the
   buyer hunting for a supplier they did not need to find.

7. **Nothing is silently substituted or dropped.** Finalisation re-verifies every
   line; a line that changed blocks the whole order and is named. Quantity cells
   that cannot be read (`"2-3"`, negatives, garbage) produce no quantity and a
   note, never a guess.

8. **The review cart is not the shopping cart.** No path in this module reads or
   writes `commerce.carts` or `commerce.cart_items`. An abandoned import must not
   leave items in a cart the buyer believes is empty.

9. **One order path.** Finalisation goes through `commerce.Service.Checkout`, so
   multi-vendor shipment partitioning, order numbering, status history and the
   documents gate are identical to an ordinary order.

10. **Money is `money.Amount` throughout.** Net prices, bands and line totals are
    integer minor-unit arithmetic. The one exception is `runs.ai_cost_estimate`,
    which is USD telemetry and deliberately not `money.Amount`.

11. **Row level security** on all seven tenant-owned tables via
    `platform.tenant_visible(organization_id)`. Cross-tenant catalogue reads use
    `database.AsSystem` with the marketplace justification stated at the call
    site. A run belonging to another organisation is Not Found, never Forbidden.

## Module boundaries

`smartorder` imports no other module. Coverage, Corporate Operations, branch
locations and order placement arrive as narrow function types
(`smartorder/adapters.go`) that the composition root fills in
(`cmd/server/smartorder.go`, `cmd/worker/smartorder.go`). The matching engine
lives in `internal/shared/productmatch`, promoted there from
`modules/ingest/engine` so `ingest`, `compare` and `smartorder` share one
implementation.

## Web routes

| Route | Purpose |
|---|---|
| `GET /customer/smart-order/new` | Step 1 — upload and configuration |
| `POST /customer/smart-order` | Create the run |
| `GET/POST /customer/smart-order/{id}/mapping` | Step 2 — confirm columns |
| `GET /customer/smart-order/{id}/progress` | Step 3 — live progress |
| `GET /customer/smart-order/{id}/results` | Step 4 — matching and supplier results |
| `GET /customer/smart-order/{id}/review` | Step 5 — the dedicated review cart |
| `POST …/lines/{lineID}/{quantity,supplier,remove}` | Review edits |
| `POST /customer/smart-order/{id}/finalize` | Re-verify and place |

A JSON API mirroring these lives in `internal/modules/smartorder/http`.

## Known gaps

- **Gateway budget.** Both organisations sit on a plan budgeted at 0.05 USD per
  week, which one large run exhausts. The AI toggle renders disabled until an
  organisation has a virtual key; an adequately budgeted plan is an owner action.
- **No embedding model** is published on the Gateway (`/v1/embeddings` → 404), so
  the semantic recall channel is the structural identity key rather than vectors.
  The `CandidateProvider` seam is where an embedding implementation would go.
- **Uploaded files are held in memory** between step 1 and step 2, with a TTL. On
  a multi-instance deployment a buyer can land elsewhere and be asked to re-upload.
  The chunked-upload path in `ingest` is the answer when that starts to matter.
- **History** shares the import screen rather than having its own view.
- **Results export** (FR-041) is not implemented.
