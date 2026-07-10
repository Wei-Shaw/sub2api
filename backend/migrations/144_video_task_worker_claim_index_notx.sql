CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_video_tasks_worker_claim_due
ON video_tasks (worker_claimed_until, updated_at, id)
WHERE status IN ('queued', 'submitted', 'running');
