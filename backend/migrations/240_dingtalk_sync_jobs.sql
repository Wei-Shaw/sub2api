CREATE TABLE IF NOT EXISTS dingtalk_sync_jobs (
 app_id VARCHAR(64) PRIMARY KEY,
 job_id UUID NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
 started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 finished_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ NOT NULL,
 error TEXT NOT NULL DEFAULT '',
 departments INTEGER NOT NULL DEFAULT 0,
 members INTEGER NOT NULL DEFAULT 0
);
ALTER TABLE dingtalk_manager_budgets ALTER COLUMN limit_cents SET DEFAULT 50000;
