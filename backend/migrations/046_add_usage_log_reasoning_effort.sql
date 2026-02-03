-- Add reasoning_effort field to usage_logs for OpenAI/Codex requests.
-- This stores the request's reasoning effort level (e.g. low/medium/high/xhigh).
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(20);

