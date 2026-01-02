-- Add request counts to ops_system_metrics so the UI/alerts can distinguish "no traffic" from "healthy".

ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS request_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS success_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS error_count BIGINT NOT NULL DEFAULT 0;
