-- Add exact rewrites for explicit OpenAI/Codex reasoning effort values.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS reasoning_effort_mappings JSONB NOT NULL DEFAULT '[]'::jsonb;
