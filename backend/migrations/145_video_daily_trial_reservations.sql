CREATE TABLE IF NOT EXISTS video_daily_trial_reservations (
    id BIGSERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    created_by BIGINT NOT NULL,
    trial_date DATE NOT NULL,
    video_task_id BIGINT NULL REFERENCES video_tasks(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, created_by, trial_date)
);

CREATE INDEX IF NOT EXISTS idx_video_daily_trial_reservations_created_by
    ON video_daily_trial_reservations (created_by, trial_date DESC);
