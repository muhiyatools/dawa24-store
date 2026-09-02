-- 160_capsule_agent (down)

BEGIN;

DROP TABLE IF EXISTS assistant.tool_audit;
DROP TABLE IF EXISTS assistant.attachments;
DROP TABLE IF EXISTS assistant.turns;

DROP INDEX IF EXISTS assistant.idx_assistant_conv_expiry;

ALTER TABLE assistant.conversations DROP COLUMN IF EXISTS expires_at;
ALTER TABLE assistant.conversations DROP COLUMN IF EXISTS agent_role;

COMMIT;
