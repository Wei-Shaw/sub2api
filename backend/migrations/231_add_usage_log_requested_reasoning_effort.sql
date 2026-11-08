-- Persist the client-requested reasoning effort before group policy rewriting
-- and model-family remapping (e.g. max -> xhigh). NULL means historical rows
-- written before this dual-write, or requests that never declared an effort.
--
-- Nullable with no default: on PostgreSQL 11+ this is a metadata-only change
-- and does not rewrite the (potentially large, partitioned) usage_logs table.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS requested_reasoning_effort VARCHAR(20);
