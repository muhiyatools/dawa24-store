# Module: Commerce

## Overview

The `commerce` bounded context handles shopping carts, checkout processing, immutable order line snapshots, multi-vendor shipment partitioning, and state machine validation.

## Schema Mapping

- **PostgreSQL Schemas:** `commerce`
- **Migrations:** `006_commerce.up.sql`
- **Tables Owned:**
  - `commerce.carts` & `commerce.cart_items` — Active customer cart state.
  - `commerce.orders` — Master customer order header.
  - `commerce.order_shipments` — Vendor-specific delivery partitions.
  - `commerce.order_lines` — Immutable historical snapshot of purchased items.
  - `commerce.order_status_history` — Audit trail for status changes.

## Invariants & Rules

1. **Resolution of Defect D2:** The two separate legacy order systems (`main_orders` and `orders`/`adv_orders`) are unified into master `commerce.orders` with linked `commerce.order_shipments`.
2. **Exact Minor Unit Arithmetic:** All totals and line sums use `money.Amount`.
3. **Multi-Vendor Shipment Partitioning:** Checking out items from multiple vendors automatically generates separate `order_shipments` records.
4. **State Machine Validation:** Transitions between `pending`, `processing`, `confirmed`, `on_hold`, `shipped`, `in_transit`, `out_for_delivery`, `delivered`, `completed`, `cancelled`, `failed`, `returned`, and `refunded` are strictly validated before committing. The enum matches Laravel's `main_orders`/`adv_orders` exactly; the legacy `ready_for_pickup` was mapped to `out_for_delivery` in migration 063.

## Endpoints

- `POST /api/v1/commerce/checkout` — Execute checkout and create order.
- `GET /api/v1/commerce/orders/{id}` — Retrieve order and shipment status.
- `GET /api/v1/commerce/orders` — List customer order history.
- `POST /api/v1/commerce/orders/{id}/status` — Apply a validated status transition.
- `GET /api/v1/commerce/vendor/shipments` — List vendor shipment partitions.
