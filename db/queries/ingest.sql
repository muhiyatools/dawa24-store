-- name: CreateFileUpload :one
INSERT INTO ingest.file_uploads (
    organization_id, user_id, filename, storage_key, file_size_bytes, mime_type
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, public_id, created_at;

-- name: GetFileUploadByID :one
SELECT id, public_id, organization_id, user_id, filename, storage_key, file_size_bytes, mime_type, created_at
FROM ingest.file_uploads
WHERE id = $1;

-- name: CreateImportSession :one
INSERT INTO ingest.import_sessions (
    organization_id, file_upload_id, status, column_mapping, min_similarity_score, total_rows
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, public_id, created_at, updated_at;
