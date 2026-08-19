# PLAN V5 — Open Questions

Record every decision you had to infer rather than read. Format:

```markdown
## Q<N> — <one-line question>

**Phase/Task:** <N.M>
**Laravel evidence read:** <file:line, what it showed>
**Interpretations considered:**
1. …
2. …
**Chosen:** <which, and why>
**If wrong, what changes:** <blast radius>
**Status:** open | resolved-by-<who/what>
```

Do not stall a phase on an open question. Implement your best interpretation,
record it here, and continue. Never mark a feature complete-but-stubbed.

---

## Q1 — Does the compare engine keep its own plan family, or reuse billing.plans?

**Phase/Task:** 0.6.3, resolved in 2.1
**Laravel evidence read:** `compare_discount_plans`, `_plan_features`,
`_plan_requests`, `_plan_subscriptions`, `_plan_subscription_users`,
`_user_sessions` exist as a distinct family in `u924222867_Testv5.sql`.
Go's `compare_handlers.go:53` subscribes against `billing.plans` with a
`"compare"` string.
**Interpretations considered:**
1. Keep the string shortcut; add limits as billing plan features.
2. Build the separate `compare` schema Laravel has.
**Chosen:** 2 — the shortcut cannot express the 8/22 archive limits or the
device cap, both of which `/what-in` advertises.
**If wrong, what changes:** migration 087 and the entitlement service.
**Status:** resolved — see Phase 2 Task 2.1. Migration 087 must also move any
subscriptions taken through the interim `billing.plans` path.
