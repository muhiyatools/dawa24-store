# Architecture Decision Records

One file while the set is small. Split into `NNNN-slug.md` when it outgrows this.

---

## ADR 0001 — Go with templ + HTMX for the Store

**Status:** Accepted · 2026-08-16

### Context

The legacy application is Laravel 12 with **353 Livewire components and 514 Blade
templates**, and roughly 20 HTTP controllers. Business logic lives in the Livewire
components, not in a service layer. The UI is server-rendered and server-reactive,
and it is the largest single artifact to be rebuilt.

Constraints: one developer working with AI agents; Elest.io billing is per
service; Arabic/RTL is a first-class requirement including for SEO; the Gateway
is already Go.

### Options

- **A.** Go + templ + HTMX + Alpine — one deployable, server-rendered
- **B.** Go API + Next.js/React — two deployables, client state
- **C.** TypeScript end-to-end — one language, two runtimes
- **D.** Stay on Laravel, modernise in place — Octane, modular refactor

### Decision

**Option A.**

### Why

1. **Closest conceptual port.** Livewire's model — server holds state, server
   renders, small deltas over the wire — maps almost 1:1 onto templ + HTMX. Onto
   React it maps close to 0:1: client state management would have to be invented,
   because the legacy app has none.
2. **One language with the Gateway.** Shared idioms, shared money handling, one
   toolchain, one CI shape.
3. **Cost.** One container at roughly 128 MB rather than two, on per-service
   billing.
4. **Arabic SSR.** HTML generated in one place; no hydration mismatch on RTL.
5. **Agent determinism.** Compile-checked templates and explicit SQL give an AI
   agent a far narrower failure surface than a framework built on runtime
   convention.

Option D was evaluated seriously and rejected on the *data* problems rather than
the language: two competing order systems, a 137-column `users` table, four
subscription systems and two RBAC systems all require a schema rewrite regardless
of language. Once the schema is being rebuilt, keeping the Laravel code buys
little.

### Consequences

- Rich data grids need work HTMX does not give free. Mitigation: TanStack Table
  and Chart.js as islands, not a React application.
- Smaller hiring pool than PHP or JS. Accepted: the codebase is small,
  conventional and heavily linted.
- **Revisit if** a customer mobile app or third-party vendor SPA becomes a
  requirement. The Go backend already speaks JSON; add Next.js then, against a
  stable API, without touching the domain layer.

---

## ADR 0002 — Modular monolith, not microservices

**Status:** Accepted · 2026-08-16

### Context

The platform is expected to grow. That is not, by itself, a reason to split it.

### Decision

A **modular monolith**: one binary, one database, module boundaries enforced by
the linter and mirrored by PostgreSQL schemas. The worker runs as a separate
*process* from the same image, not a separate service.

The **AI Gateway is the only separate service**, and it earns that on criteria no
Store module currently meets.

### Why the Gateway is separate

- **Different failure domain** — third-party LLM providers, minute-scale
  latencies, independent outages.
- **Different scaling curve** — I/O-bound proxying versus transactional commerce.
- **Shared across products** — it already serves other Muhiya applications.
- **Holds credentials the Store must never see.**

### Extraction triggers

A module leaves the monolith only on a specific, measurable condition:

| Candidate | Trigger |
|---|---|
| Import/ingestion | Sustained ingestion > 100k rows/hour, **or** import work causes > 20% of API P99 latency |
| Search indexer | Catalogue > 250k active SKUs, **or** search P95 > 300 ms after index tuning |
| Notifications | > 1M sends/month, **or** an integration needs an independent release cadence |
| Reporting | Analytical queries measurably degrade OLTP and read replicas are already in use |

"It will scale" is not a trigger. Neither is "the team prefers services".

### Consequences

- One deploy for most changes; coupling must be prevented by tooling rather than
  by network boundaries. `depguard` rules in `.golangci.yml` do this.
- One database is one failure domain for business data. Accepted: PostgreSQL with
  PITR and a tested restore is more reliable than distributed transactions spread
  across services nobody is staffed to operate.

---

## ADR 0003 — Tenant isolation via PostgreSQL row-level security

**Status:** Accepted · 2026-08-16

### Context

This is a **multi-vendor marketplace**: suppliers who compete with each other keep
catalogue, pricing, stock and order data in one database. A leak between tenants
is the most damaging bug the system can have.

The legacy application enforced isolation by hand — `where organization_id = ?`
repeated across 353 Livewire components. Every new query is another chance to
forget. The audit also found **36 `*_id` columns with no foreign key**, so this
was not a codebase where such invariants were reliably maintained.

### Decision

Defence in depth, with the database as backstop:

1. Middleware resolves the active organisation into `context.Context`.
2. `database.InTx` / `InReadTx` issue `SET LOCAL app.current_org_id` per
   transaction.
3. Tenant-owned tables `ENABLE` **and `FORCE`** row-level security, with a policy
   calling `platform.tenant_visible(organization_id)`.
4. Cross-tenant access requires `database.AsSystem(ctx)` — explicit and greppable.

### Why FORCE matters

Without `FORCE ROW LEVEL SECURITY` the table owner bypasses every policy. In a
small deployment the application usually connects as the owner, which would make
the whole mechanism decorative.

### Why SET LOCAL rather than SET

`SET LOCAL` is scoped to the transaction and discarded when the connection
returns to the pool. Session-level `SET` would leak one tenant's id into the next
request that borrowed the same pooled connection — precisely the bug this exists
to prevent.

### Consequences

- A forgotten `WHERE` returns **zero rows** instead of a competitor's data: it
  fails closed, surfacing as a visible bug rather than a silent breach.
- All tenant-scoped access goes through the transaction helpers; `db.Pool()`
  stays out of module code.
- Negligible per-query GUC overhead relative to the queries themselves.
- **Required test:** every tenant-owned table gets a cross-tenant read test
  asserting zero rows. CI gate, not a convention.

---

## ADR 0004 — Migrations run before promotion; rollback is redeploy

**Status:** Accepted · 2026-08-16

### Context

The legacy project had no CI, no containers, no IaC and no version control. Its
schema could not be rebuilt from its migrations: 49 active migrations against 114
models, plus 153 archived ones and a `migrations.zip`.

### Decision

- Migrations run as a **discrete pipeline step**, before the new image is
  promoted. Never on server startup.
- Every migration is **backward-compatible with the running version**
  (expand/contract): add and backfill in one release, remove in a later one.
- **Rollback = redeploy the previous image.** No schema reversal in the hot path.
- Migrations are **embedded in the binary** and **checksummed**; editing an
  applied migration is refused.
- An **advisory lock** serialises concurrent deploys.
- `/ready` reports unready while migrations are pending, so an instance never
  serves traffic against a schema it does not expect.

### Why not migrate on startup

N instances starting together race. Worse, it couples "the schema is ready" to "a
web process started", so a migration failure presents as a crash loop instead of
a failed deployment step with a readable log.

### Consequences

- A destructive change takes two releases. That is the point.
- The pipeline needs database credentials at the migration step.
- `down.sql` files exist for local development. They are **not** the production
  rollback plan; the previous image is.

---

## ADR 0005 — AI lives behind the Gateway; the Store stays provider-agnostic

**Status:** Accepted · 2026-08-16

### Context

The legacy application wired five providers directly into business code, plus a
provider SDK dependency, an `ai_providers` table, two competing traits
(`HasAiServices` and `OldHasAiServices`), and provider API keys in `.env`. Roughly
19 files referenced providers directly.

A mature Gateway already exists (`muhiyallm-gateway`): OpenAI-compatible surface,
40 migrations, exact-money billing, virtual keys, per-model pricing windows,
request logging, and a `client_app` column already designed for multiple
consumers.

### Decision

1. The Store contains **no provider SDK, no provider key, no model name**. CI
   enforces it (`make check-provider-isolation`).
2. The Store requests **capabilities** — `product.match`, `import.detect_columns`,
   `order.optimize` — which the Gateway maps to model aliases.
3. **Do not fork the Gateway.** Deploy the same codebase as a second Elest.io
   service with its own database and keys. A fork means maintaining two diverging
   money implementations forever.
4. **Every capability has a deterministic fallback.** The Gateway being
   unreachable degrades quality, never availability.

### Fallbacks

| Capability | Fallback |
|---|---|
| `import.detect_columns` | Heuristic header matcher (legacy `ColumnDetector` path) |
| `product.match` | Trigram + Arabic-normalised fuzzy match in Postgres |
| `product.enrich` | Skip; leave fields null and flag for review |
| `order.optimize` | Rule engine: price, then stock, then distance |
| `catalog.chat` | Assistant unavailable; ordinary search still works |
| `search.expand_query` | Plain search without expansion |

### Consequences

- Swapping providers is a Gateway admin change with **zero Store deploys**.
- The only Gateway code change needed is logging a tenant header (~20 lines),
  written generically so it lands upstream and benefits every client.
- AI spend is attributed per organisation via `X-Dawa-Org-ID` and capped by
  Gateway plans.
- Store developers cannot experiment with a provider directly. That is the
  intent: the capability boundary is what keeps the Store portable.
