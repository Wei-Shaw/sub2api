-- MySQL 方言版本（翻译自 ../039_ops_job_heartbeats_add_last_result.sql）

-- Add last_result to ops_job_heartbeats for UI job details.

ALTER TABLE ops_job_heartbeats
    ADD COLUMN last_result TEXT NULL;

-- (comment skipped)
