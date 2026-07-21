-- Add a per-group ceiling for OpenAI/Codex reasoning effort.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS max_reasoning_effort VARCHAR(20) NOT NULL DEFAULT '';
