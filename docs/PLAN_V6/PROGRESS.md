# PLAN V6 — Progress

Update after **every task**. Statuses: `not-started` | `in-progress` | `done` | `blocked`

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| A | A.0 Real test harness (`newRealUIHandler`) | not-started | | | **do first — nothing is verifiable without it** |
| A | A.1 Delete 2FA theatre + close login hole | not-started | | D2,D4 | REVIEW §2.1 |
| A | A.2 Delete fake PDF generators | not-started | | | REVIEW §2.4 |
| A | A.3 Delete fabricated datasets (5 screens) | not-started | | D1 red | REVIEW §2.2 |
| A | A.4 Fix/remove 21 no-op submit handlers | not-started | | D2,D3,D4 | REVIEW §2.3 |
| A | A.5 Fix misleading doc comments | not-started | | | |
| B | B.1 Consolidate 6 user-list screens | not-started | | D1,D3 | |
| B | B.2 Collapse organization aliases | not-started | | | |
| B | B.3 Collapse sponsorship + saving aliases | not-started | | | |
| B | B.4 Resolve policies duplication | not-started | | D1,D2 | user-reported |
| B | B.5 Resolve catalog naming collision | not-started | | | user-reported |
| B | B.6 Merge deletes-lists / trash-list | not-started | | | |
| B | B.7 Rebuild admin + vendor navigation | not-started | | D3 | 162 orphan routes |
| B | B.8 Delete screens Laravel does not have | not-started | | | |
| C | C.2 Vendor finance | not-started | | D1,D3,D4 | worked example |
| C | C.3 Billing & subscriptions admin | not-started | | | 4 dead tables |
| C | C.4 Employee activities (+ write hook) | not-started | | | |
| C | C.5 Vendor policies & social media | not-started | | | |
| C | C.6 Admin reference data (4 screens) | not-started | | | never render a key |
| C | C.7 Trash & deletes (real registry) | not-started | | D2 | |
| C | C.8 Monetisation (13 screens, 9 tables) | not-started | | | apply Q1/Q2 first |
| C | C.9 2FA rebuild | not-started | | D2 | wrong code must be rejected |
| C | C.10 PDF rebuild | not-started | | D1 | Arabic shaping check |
| C | C.11 Saving products (10 screens) | not-started | | | |
| C | C.12 Vendor institutional & pharmacy coverage | not-started | | | |
| C | C.13 Bulk employee upload | not-started | | D3 | security-sensitive |
| C | C.14 Temp warehouses (or drop 3 tables) | not-started | | | scope decision first |
| C | C.15 Session plans & report issues | not-started | | | |
| C | C.16 Notifications & remaining admin | not-started | | | |
| D | D.1 Eliminate 233 silent-failure sites | not-started | | | file by file |
| D | D.2 Add `make check-error-swallow` gate | not-started | | | prove it fails |
| D | D.3 Sweep `internal/modules/` | not-started | | | |
| D | D.4 Verify error paths reachable | not-started | | | ≥20 screens |
| E | E.1 Seven mechanical scans | not-started | | | |
| E | E.2 Six-link chain audit | not-started | | | |
| E | E.3 Delete-the-body test (15 screens) | not-started | | | |
| E | E.4 Walk the product per account type | not-started | | | |
| E | E.5 Security verification | not-started | | | |
| E | E.6 Business-logic parity | not-started | | | |
| E | E.7 Structural health metrics | not-started | | | all must go down |
| E | E.8 Final report | not-started | | | |
