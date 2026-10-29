-- Add per-group controls for explicit OpenAI/Codex reasoning effort values.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS max_reasoning_effort VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reasoning_effort_mappings JSONB NOT NULL DEFAULT '[]'::jsonb;
