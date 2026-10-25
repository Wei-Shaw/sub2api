-- MySQL 方言版本（翻译自 ../232_group_reasoning_effort_over_limit.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字需反引号；ADD COLUMN IF NOT EXISTS → ADD COLUMN。

-- Per-group access control when an explicit OpenAI/Codex reasoning effort
-- exceeds max_reasoning_effort. Existing groups keep the previous behaviour
-- (automatically downgrade to the ceiling).
ALTER TABLE `groups`
    ADD COLUMN max_reasoning_effort_over_limit VARCHAR(20) NOT NULL DEFAULT 'downgrade';
