-- MySQL 方言版本（翻译自 ../185_group_reasoning_effort_policy.sql）
-- Add per-group controls for explicit OpenAI/Codex reasoning effort values.
--
-- 说明：JSONB NOT NULL DEFAULT '[]'::jsonb → JSON NOT NULL DEFAULT ('[]')。
ALTER TABLE `groups`
    ADD COLUMN max_reasoning_effort VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN reasoning_effort_mappings JSON NOT NULL DEFAULT ('[]');
