# Module: Ingest Pipeline

## Overview

The `ingest` bounded context provides vendor bulk catalog file ingestion, heuristic and AI column header detection, row staging, and fuzzy Arabic product matching against the master catalogue.

## Schema Mapping

- **PostgreSQL Schemas:** `ingest`
- **Migrations:** `008_ingest.up.sql`
- **Tables Owned:**
  - `ingest.file_uploads` — Pointers to spreadsheet files stored in S3/MinIO (Defect D5 resolution: object storage keys, zero raw BLOBs).
  - `ingest.import_sessions` — Status, detected column mappings, and progress metrics.
  - `ingest.import_rows` — Staged rows with normalized Arabic text and match confidence scores.

## Invariants & Rules

1. **Heuristic Header Detection First:** Spreadsheets are scanned against common Arabic and English pharmacy column synonyms before attempting any AI capability fallback.
2. **Deterministic String Matching First:** Rows are matched using `arabic.Similarity` + `pg_trgm` scoring against master products. Escalation to AI matching occurs only when similarity falls below the session threshold (`min_similarity_score`, default 0.85).
3. **Graceful Degrade:** Ingestion operates fully deterministically without failure even when `GATEWAY_ENABLED=false`.

## Endpoints

- `POST /api/v1/ingest/uploads` — Register uploaded file metadata.
- `POST /api/v1/ingest/sessions` — Initiate import session and compute column detection.
- `GET /api/v1/ingest/sessions/{id}` — Poll import processing metrics.
