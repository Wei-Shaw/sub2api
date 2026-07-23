-- Add exact per-model overrides for the group OpenAI/Codex reasoning policy.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_reasoning_effort_rules JSONB NOT NULL DEFAULT '[]'::jsonb;
