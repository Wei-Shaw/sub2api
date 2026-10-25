-- MySQL 方言版本（翻译自 ../231_add_usage_log_requested_reasoning_effort.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN。

-- Persist the client-requested reasoning effort before group policy rewriting
-- and model-family remapping (e.g. max -> xhigh). NULL means historical rows
-- written before this dual-write, or requests that never declared an effort.
--
-- Nullable with no default: on PostgreSQL 11+ this is a metadata-only change
-- and does not rewrite the (potentially large, partitioned) usage_logs table.
-- （MySQL 8 的 InnoDB ADD COLUMN 同为 INSTANT 元数据变更，不影响存量行。）
ALTER TABLE usage_logs ADD COLUMN requested_reasoning_effort VARCHAR(20);
