# Module: Workflow

## Overview

The `workflow` bounded context manages background purchasing priority optimization, geographic branch route coverage, and defect/support ticket tracking.

## Schema Mapping

- **PostgreSQL Schemas:** `workflow`
- **Migrations:** `010_workflow.up.sql`
- **Tables Owned:**
  - `workflow.purchase_priority_engines` — Bulk order supplier selection calculations.
  - `workflow.weekly_coverages` — Daily branch delivery routes and coverage zones.
  - `workflow.report_issues` — Customer issue and defect reports.

## Invariants & Rules

1. **Purchasing Optimization:** Supports constraint weighting across highest discount, lowest price, fastest delivery, and preferred suppliers.
2. **Weekly Route Schedules:** Delivery schedules per branch are isolated per tenant with `FORCE ROW LEVEL SECURITY`.
