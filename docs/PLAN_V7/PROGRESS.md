# PLAN V7 — Progress

Record before/after counts for every phase.

| | Baseline | P1 | P2 | P3 | P4 | P5 | Target |
|---|---:|---:|---:|---:|---:|---:|---:|
| Routes | 447 | | | | | | ≤330 |
| Pages | 97 | | | | | | ≤75 |
| Tables | 161 | | | | | | ≤140 |
| Inline styles | 4938 | | | | | | ≤1000 |
| Data-less pages | 31 | | | | | | 0 |

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| 1 | 1.1 Weekly coverage TIME bugs | **done** | 31ce6fd | unit ✅ shown failing first; repo round-trip written (runs in CI) | root cause: TIME columns vs Go string, both directions. Same bug fixed in hr.work_times. |
| 1 | 1.2 Cart & order availability | **done** | 7194799 | 26 unit cases ✅ | CheckAvailability + probe at composition root; wired to add/quantity/checkout, HTML + JSON. RACE test still owed (needs a DB). |
| 2 | 2.1 Settings 7 → 1 | not-started | | | keep OLD tabbed page |
| 2 | 2.2 Branches 3 → 2 write paths | not-started | | D2,D3 | |
| 2 | 2.3 Policies → settings tab | not-started | | D1,D2 | |
| 2 | 2.4 User lists 6 → 1 | not-started | | | |
| 2 | 2.5 Five remaining clusters | not-started | | | |
| 2 | 2.6 Catalog 4 → 2 + naming | not-started | | | |
| 3 | 3.1 Remove translations | not-started | | | confirm nothing reads it |
| 3 | 3.2 Remove cPanel | not-started | | | move anything unique first |
| 3 | 3.3 Delete dead files | not-started | | | pharmacy.templ confirmed dead |
| 3 | 3.4 Resolve 31 data-less pages | not-started | | | expect mostly deletion |
| 3 | 3.5 Drop dead tables | not-started | | | |
| 3 | 3.6 Dead code sweep | not-started | | | |
| 4 | 4.1 Decide brand↔category cardinality | not-started | | | recommend many-to-many |
| 4 | 4.2 Migration + backfill | not-started | | | |
| 4 | 4.3 One category tree screen | not-started | | | cycle guard |
| 4 | 4.4 Cascading selector | not-started | | D1,D2,D4 | server-side enforcement |
| 4 | 4.5 Seed real categories | not-started | | | |
| 5 | 5.1 Design tokens | not-started | | | extend, don't restart |
| 5 | 5.2 Replace inline styles | not-started | | | file by file + CI gate |
| 5 | 5.3 Adopt the component library | not-started | | | reduce card nesting |
| 5 | 5.4 Fix the footer | not-started | | | none on dashboards |
| 5 | 5.5 Final navigation | not-started | | | NAVIGATION.md |
| 5 | 5.6 Full product walk | not-started | | | WALKTHROUGH.md |
