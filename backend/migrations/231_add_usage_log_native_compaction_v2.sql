ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS native_compaction_v2 BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN usage_logs.native_compaction_v2 IS
    'True only when the request was identified at runtime as native OpenAI remote compaction v2';
