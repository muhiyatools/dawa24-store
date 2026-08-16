# Module: Promo, Offers & Ads

## Overview

The `promo` bounded context handles promotional campaigns, vendor discount configurations, sponsorship tier packages, banner advertisements, and impression/click analytics.

## Schema Mapping

- **PostgreSQL Schemas:** `promo`
- **Migrations:** `009_promo.up.sql`
- **Tables Owned:**
  - `promo.offers` — Tenant-owned discount promotions (percentage or fixed).
  - `promo.offer_products` — Product mapping relationships.
  - `promo.offer_packages` & `promo.offer_sponsorships` — Paid promotion tiers and active vendor sponsorships.
  - `promo.ads` & `promo.ad_clicks` — Display advertisement banners and engagement tracking.

## Invariants & Rules

1. **Exact Minor Unit Pricing:** Percentages and fixed discounts are processed with `money.Amount` minor unit arithmetic (basis points for percentage discounts).
2. **Impression & Click Auditing:** Views and click counters are updated atomically without blocking customer reads.
3. **Tenant Row Level Security:** Vendor offers and sponsorships are isolated with `FORCE ROW LEVEL SECURITY`.

## Endpoints

- `GET /api/v1/promo/offers` — List all running promotions across the marketplace.
- `POST /api/v1/promo/offers` — Configure a new vendor discount offer.
- `GET /api/v1/promo/offers/{id}` — Get offer details and increment view count.
- `POST /api/v1/promo/offers/{id}/click` — Record click event on an offer.
- `GET /api/v1/promo/packages` — List available sponsorship tiers.
- `GET /api/v1/promo/ads` — List display advertisements.
- `POST /api/v1/promo/ads/{id}/click` — Log ad click analytics.
