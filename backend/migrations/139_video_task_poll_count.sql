-- VA2 guard: per-task poll attempt counter for the video gateway worker.
--
-- The worker re-polls every non-terminal task once per tick. Before this column
-- the only bound was the wall-clock task timeout; poll_count adds an explicit
-- per-task poll cap so a task that never reaches a terminal status is failed
-- deterministically (config: video_gateway.max_poll_attempts) instead of being
-- re-polled until the outer timeout. Additive + idempotent; existing rows default
-- to 0 polls.
ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS poll_count INTEGER NOT NULL DEFAULT 0;
