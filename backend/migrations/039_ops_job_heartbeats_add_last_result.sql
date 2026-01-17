-- Add last_result to ops_job_heartbeats for UI job details.

ALTER TABLE IF EXISTS ops_job_heartbeats
    ADD COLUMN IF NOT EXISTS last_result TEXT;

COMMENT ON COLUMN ops_job_heartbeats.last_result IS 'Last successful run result summary (human readable).';
