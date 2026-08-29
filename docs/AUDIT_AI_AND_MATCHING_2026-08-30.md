# Deep audit — AI system, matching engine, and import tooling

Date: 2026-08-30. Audited against the live database
(`postgres-u74003.vm.elestio.app/dawa24_store`) and the working tree at
`F:/Dawa 24/dawa24-store`.

Every finding below was verified — by reading the code path, by querying the
live schema, or both.

---

## PART 0 — Two findings that outrank everything else

### 0.1 The production database password is stored as the AI Gateway admin credential

`platform_admin.system_settings['gateway_configuration'].api_key` currently
holds `postgres:RBSW2NW9-dy4d-63ZLK0DC`.

That is the Postgres superuser credential for the production database, not a
Gateway administrator credential. `gateway.NewAdminClient` splits it on `:` and
sends it as HTTP **Basic auth to `https://api.muhiya.com`** on every
provisioning, plan-listing, usage-summary and log-fetch call
(`internal/platform/gateway/admin_client.go:99-113`, `:411`).

Consequences, in order:

1. The production DB superuser password has been transmitted to a third-party
   host, repeatedly, and is at rest in plaintext in a table.
2. Every `/api/*` management call authenticates with a credential the Gateway
   cannot recognise, so org provisioning silently returns `("org-N", "", nil)` —
   a user id with no key — and callers fall through to the platform admin key.

**Required before any other work:** rotate the Postgres password, rotate the
Gateway admin credential, clear the field, and add validation that rejects a
DSN-shaped value. This is not something to fold into a refactor.

### 0.2 `ProvisionOrganization` mints a new virtual key on every call, from the request path

`internal/platform/gateway/admin_client.go:315` unconditionally `POST`s
`/api/keys` — it never lists existing keys for the user. Issuing a key revokes
the previous one; the admin-panel provisioner documents exactly this at
`cmd/server/gateway_admin_key.go:81-87`.

It is invoked lazily from four copy-pasted blocks:

- `cmd/server/routes.go:279-299` (UI key resolver)
- `cmd/server/routes.go:539-559` (API key resolver)
- `internal/ui/dashboard_handlers.go:573`
- `internal/ui/admin_handlers.go:1326`

with no mutex, no singleflight, no cache. Two concurrent requests for an org
with no key mint two keys and the second revokes the first. The
`o.AIVirtualKey != ""` guard means a stored-but-revoked key is never detected —
unlike the admin path, which validates (`gateway_admin_key.go:147-160`).

---

## PART 1 — The AI system

### 1.1 Organisation identity and keys

| Requirement | State |
|---|---|
| Every org has a Gateway user | Partly — 3 of 7 orgs have `ai_user_id`; the rest are `pending` and never provisioned |
| Every org has a Gateway API key | 3 of 7 |
| Employees use the org's key | Yes, by design — `keyResolver` resolves org to key (`routes.go:262`, `:525`) |
| Key provisioning is safe / idempotent | **No** — see 0.2 |
| Org Gateway plan tracks its subscription | **No** — see 1.2 |

Provisioning is triggered only by a page render. An org that never opens a
dashboard never gets an identity. There is no provisioning on approval, on
signup, or as a backfill.

### 1.2 Subscription to Gateway plan is never synchronised

`AdminClient.UpdateOrganizationPlan` exists at
`internal/platform/gateway/admin_client.go:378` and **has no callers anywhere in
the repository** (verified by grep across `*.go`).

The org Gateway plan is therefore fixed at whatever `ai_plan_id` its billing
plan had when it was first provisioned. Upgrading a subscription changes
`billing.subscriptions.plan_id` and nothing else — the tenant keeps the old AI
quota forever.

`billing.Service.Subscribe`, `SubscribeWithWallet`, `AssignDefaultSubscription`
and `ProcessDueSubscriptionRenewals` (`internal/modules/billing/service.go:284,
327, 368, 438`) all lack the hook.

Secondary problem — the fallback plan id is inconsistent:

- `plan-pos-free` — `admin_client.go:317`, `routes.go:285`, `dashboard_handlers.go:560`
- `plan-basic` — `billing/postgres/plans.go:23`, migration `110`
- `plan-dev` — `dashboard_handlers.go:37`

And the live `billing.plans` rows are not defensible either:
`basic → plan-pos-free`, `pro → plan-dev`, `enterprise → yalla`. "yalla" and
"plan-dev" are not plans; nothing validates these against `AdminClient.ListPlans`.

### 1.3 The usage cards are fabricated

`internal/ui/dashboard_handlers.go:23-140` (`loadOrgSubscriptionView`):

- **:118-126** — when the Gateway publishes no budget window, the limit is
  invented from the plan slug: `enterprise → $200`, `pro → $50`, else `$15`.
  These numbers exist nowhere but this switch statement.
- **:131-135** — when the Gateway publishes no reset time, one is fabricated as
  the first of next month.
- **:137** — `subView.HasAIUsage = true` is assigned unconditionally, after
  every branch. The flag that means "we have real usage data" is always true.

So the percentage bar on the subscription and dashboard cards is
`fabricated_spend / fabricated_limit`, rendered as fact.

### 1.4 The AI Usage Logs screen is partly fabricated

`internal/ui/dashboard_handlers.go:583-770`,
`internal/platform/gateway/admin_client.go:243-296`:

- `GatewayLogEntry.ResolvedModel()` returns the literal `"qwen3.7-flash"` when
  the Gateway reports no model (`admin_client.go:243`) — a guess presented as
  the model that ran.
- `ModelTier: "fast"` is hardcoded for every row (`dashboard_handlers.go:651`).
- Assistant rows synthesise cost from invented per-token rates (`0.0000008` in,
  `0.000002` out — `:686`) and a hardcoded `DurationMs: 280` (`:700`).
- There is **no local usage ledger**. The page makes 1–3 live HTTP calls to
  `api.muhiya.com` per render, is capped at 100 rows, has no history beyond what
  the Gateway retains, and shows nothing when the Gateway is down — even though
  `gateway.Response` already carries `RequestID`, `InputTok`, `OutputTok`,
  `CostNanoUSD`, `FromCache` and `Fallback` on every call
  (`internal/platform/gateway/gateway.go:176-196`) and discards all of it.

There is no `ai` schema in the live database despite `AGENTS.md` listing one.

### 1.5 The Assistant

`internal/modules/assistant/http/handlers.go:145-159` does resolve a per-org
virtual key. But `routes.go:296` falls back to `adminKeys.Key(ctx)` — the
platform key — whenever the org has none. Every unprovisioned tenant's assistant
traffic is billed to the platform budget and is invisible in that tenant's
usage. With 4 of 7 orgs unprovisioned, that is the common case.

---

## PART 2 — The matching engine and the four import tools

The shared engine (`internal/shared/productmatch`, 29 files) is genuinely good:
IDF-weighted token scoring, dose and dosage-form vetoes, transliteration
skeletons, an inverted index with a capped candidate pool. The problems are not
in the scorer. They are in **how each tool configures it** and in **what the
review screens do with the result**.

### 2.1 Identifier matching is force-enabled where it must not be

`internal/ui/saving_products_matcher.go:96-105` hardcodes, for every savings
import regardless of what the user mapped:

```go
opts.TrustSupplierCode   = true
opts.CodeIsAuthoritative = true
```

`CodeIsAuthoritative` disables the name-corroboration guard at
`productmatch/match.go:272`. A four-character numeric collision between the
pharmacy's internal numbering and a catalogue code is then accepted at
confidence 0.95 with no name check at all.

Worse, `saving_products_matcher.go:161` and `:176`:

```go
row.SKU     = strings.TrimSpace(rawSKU)
row.Barcode = strings.TrimSpace(rawSKU)   // the code is also fed as a barcode
```

The same value is offered to *both* identifier tiers. `matchByBarcode`
(`match.go:230`) accepts any 8+ digit value with a single hit and returns
`Score: 1` — before any scoring runs, with no name check whatsoever. An 8-digit
internal item code colliding with a GTIN produces a silent, confident, wrong
link. This is precisely the failure described.

### 2.2 Toggle defaults ignore the column mapping

- Vendor ingest: `TrustSupplierCode` is a manual switch
  (`vendor_ingest_stages.templ:342`) with no relationship to whether a code
  column was mapped in step 1. It defaults off (correct), but a user who turns
  it on without a mapped code column gets nothing.
- Savings import: no toggle at all — both identifier options are forced on.
- Admin catalog import and variants import: no user-facing matching toggles;
  thresholds are compile-time constants (`import_match.go:47, 61, 64`).

The requested behaviour — toggle state derived from the step-1 mapping, and off
by default in both cases — is implemented nowhere.

### 2.3 Review screens do not surface the lowest confidence first

`internal/modules/ingest/postgres/catalog_import_rows.go:141-155` supports
`sort=score`, but the default is `r.source_row` (`:153`). The savings review,
smart-order review and admin import review order by source row or by id.

Nothing defaults to "worst match first", which is the one ordering that makes a
20K-row review tractable.

### 2.4 Thresholds are incoherent across tools

| Tool | Auto-apply floor | Source |
|---|---|---|
| Vendor ingest | 0.30 (user-editable) | `catalog_import.go` `DefaultSettings` |
| Smart order | 0.30 | `productmatch.DefaultMatchOptions` |
| Savings import | level-based, no floor | `saving_products_matcher.go:180` |
| Admin catalog import | 0.86 bare / 0.78 corroborated | `import_match.go:47,61` |

A match percentage of 0.55 means "accepted" in one tool and "rejected" in
another, on the same two strings. The number shown to the user is not comparable
across screens, which makes it not meaningful on any of them.

### 2.5 Scale

Live catalogue: 19,998 products; 3,985 variants; 3,539 savings rows; 6,773
recorded match decisions. Target stated: 150K catalogue by 30K file rows.

`NewIndex` is O(catalogue) and allocates per product; `candidatePool`
(`match_index.go:196-243`) sorts the query tokens by IDF on **every row** and
builds a fresh `map[int64]*MasterProduct` per row. At 30K rows that is 30K map
allocations and 30K sorts on top of scoring. It will work, but it is the obvious
first target for the 20K+ row requirement, and no benchmark exists for it
(`compare/compare_30k_stress_test.go` covers a different engine).

### 2.6 Arabic normalisation is frozen for legacy parity

`internal/shared/arabic/arabic.go` is explicitly a bug-for-bug port, and its
header says so: *"Improvements to matching quality belong in a new scorer behind
a feature flag, after cutover."* The containment branch
(`0.80 + 0.18 * shorter/longer`) means "بانادول" against
"بانادول اكسترا اقراص 500 مجم" scores 0.83 — higher than many genuinely correct
full-name matches. That is a real source of wrong ranking, and it is
load-bearing for `catalog.import_sessions.min_similarity_score`.

It does **not** implement: hamza folding inside words beyond the leading forms,
lam-alef ligature decomposition, shadda expansion, Egyptian ج/چ, ق/ء, ث/س
substitution classes, or any phonetic key. For the "one letter different"
Egyptian spelling variance, a normalisation-only approach is the wrong tool — a
substitution-aware distance or a phonetic key is needed.

---

## PART 3 — AI as the final matching stage

### 3.1 It is not a final stage today; it is interleaved and in-process

- **Vendor ingest**: `catalog_enhance.go:289` fans out goroutines inside the
  import run, which itself runs in an in-process goroutine guarded by
  `s.runs.claim(publicID)` (`catalog_commit.go:38`). A pod restart or deploy
  loses the run with no resumption. Progress lives in `ingest.import_progress`,
  so the bar survives; the work does not.
- **Admin catalog import**: `import_match.go` adjudicates in-line during
  matching, capped at `maxAIAdjudicationBatches = 24` by `aiBatchSize = 25` =
  600 rows total. A 30K-row file gets AI on at most 2% of its residue and is
  given no indication the cap was hit.
- **Smart order**: the only tool with a durable background worker
  (`internal/modules/smartorder/jobs/run.go`, River queue). This is the pattern
  the others should adopt.
- **Savings import**: no AI stage at all.

### 3.2 There is no shared AI-enhancement contract

`internal/shared/matchflow` has only three small files (`cachekey.go`,
`ceilings.go`, `contract.go`) and is not the shared pipeline its name implies.
`smartorder/pipeline/enhance*.go` and `ingest/catalog_enhance*.go` are two
parallel implementations of the same idea — plan batches, call
`CapMatchEnhance`, re-verify identity, apply — with separate bugs and caps.

### 3.3 Resumability

`ingest.import_progress` exists but no table records which rows have already
been sent to AI. A resumed or retried run re-sends and re-bills.
`catalog.match_decisions` (6,773 rows) is the closest thing to a cache and is
consulted by the vendor path only.

---

## Recommended execution order

**Phase 0 — security (first, separately)**
Rotate the DB password. Rotate/replace the Gateway admin credential. Validate
the field; refuse a DSN-shaped value.

**Phase 1 — AI identity, plans, honest usage**
1. `AdminClient`: add `ListUserKeys`; list-then-reuse; make provisioning
   idempotent.
2. One `tenantKeyProvisioner` mirroring `adminKeyProvisioner`: per-org locking,
   TTL cache, stored-key validation, failure back-off. Delete the four inline
   copies.
3. Provision on org approval and via a backfill command, not on page render.
4. Wire `UpdateOrganizationPlan` into every subscription transition through a
   port defined in `billing`, implemented in `cmd/server`.
5. Validate `billing.plans.ai_plan_id` against `ListPlans` in the admin UI.
6. New `ai` schema and `ai.usage_events` ledger, written by a decorator around
   `gateway.Client`. Cards and logs read the ledger; the Gateway is consulted
   only for the live budget window. Delete every fabricated constant in 1.3 and
   1.4; render "غير متاح" where data is genuinely absent.

**Phase 2 — the shared matching engine**
1. Extend `shared/arabic` with an Egyptian-variance layer behind a new entry
   point, leaving `Similarity` untouched for parity.
2. Unify options into one `matchflow.Profile` consumed by all four tools;
   identifier tiers off unless the column was mapped **and** the user opted in.
3. Stop feeding SKU into `Row.Barcode`.
4. One calibrated confidence scale shared by all tools.
5. Default every review screen to ascending confidence.
6. Benchmark and optimise `NewIndex` / `candidatePool` for 150K by 30K; reuse
   the pool map and IDF ordering across rows.

**Phase 3 — AI as a durable final stage**
1. Move the vendor and admin imports onto River workers, like smart order.
2. One shared enhancement pipeline in `shared/matchflow`, replacing the two
   parallel implementations.
3. An `ai.match_attempts` ledger so a resumed run never re-bills a row.
4. Lift the 600-row cap; replace it with a per-run budget the user sees.
5. AI runs strictly after the deterministic pass, over unmatched rows only.
