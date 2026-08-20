# AI Assistant — Decisions

Record every answer to an open question and every assumption made.

```markdown
## Q<N> — <question>
**Evidence:** <file:line in the Gateway or Store source>
**Answer:**
**If wrong:** <what breaks, and the test that would catch it>
```

---

## Q1 — Does the Gateway SSE carry a separate `reasoning` delta?
**Evidence:** `proxy/agent.go:538` mentions "reasoning delta to the MuhiyaChat client".
**Status:** OPEN — confirm whether it is emitted for non-MuhiyaChat client apps.
**If unavailable:** drop the `reasoning` SSE frame; render content only.
