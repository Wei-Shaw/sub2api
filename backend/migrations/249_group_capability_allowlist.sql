ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS capability_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.capability_allowlist IS
    'Group Codex capability allowlist for OpenAI/Composite groups: {"enabled":bool,"cyber_programs":["daybreak_blue","daybreak_red"],"service_tiers":["priority","ultrafast"]}; when enabled, requests carrying disabled capabilities are rejected and the Codex manifest omits them';
