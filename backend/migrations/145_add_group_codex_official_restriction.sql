-- Add OpenAI group-level Codex official client restriction.

ALTER TABLE groups
ADD COLUMN IF NOT EXISTS codex_official_only BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_groups_codex_official_only
ON groups(codex_official_only) WHERE deleted_at IS NULL;

COMMENT ON COLUMN groups.codex_official_only IS 'OpenAI 分组是否仅允许 Codex 官方客户端';
