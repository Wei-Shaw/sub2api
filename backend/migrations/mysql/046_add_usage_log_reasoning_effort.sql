-- MySQL 方言版本（翻译自 ../046_add_usage_log_reasoning_effort.sql）

-- Add reasoning_effort field to usage_logs for OpenAI/Codex requests.
-- This stores the request's reasoning effort level (e.g. low/medium/high/xhigh).
ALTER TABLE usage_logs ADD COLUMN reasoning_effort VARCHAR(20) NULL;
