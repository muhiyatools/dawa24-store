-- 070_merge_file_uploads (up)
--
-- Rebuild V2 §2.4d — ingest.file_uploads merges into platform_admin.documents
-- with document_type = 'import_file'. The legacy columns map 1:1 onto the
-- 049-expanded documents shape (filename → original_name, file_size_bytes →
-- size_bytes). import_sessions.file_upload_id re-points to documents.

BEGIN;

INSERT INTO platform_admin.documents (
    public_id, organization_id, user_id, title, document_type, storage_key,
    original_name, file_url, status, mime_type, size_bytes, meta, created_at, updated_at
)
SELECT public_id, organization_id, user_id, filename, 'import_file', storage_key,
       filename, '', 'pending', mime_type, file_size_bytes, NULL, created_at, now()
FROM ingest.file_uploads;

ALTER TABLE ingest.import_sessions
    DROP CONSTRAINT IF EXISTS import_sessions_file_upload_id_fkey;

ALTER TABLE ingest.import_sessions
    ADD CONSTRAINT import_sessions_file_upload_id_fkey
    FOREIGN KEY (file_upload_id) REFERENCES platform_admin.documents(id) ON DELETE CASCADE;

DROP TABLE ingest.file_uploads;

COMMIT;
