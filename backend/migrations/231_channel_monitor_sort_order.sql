-- Persist the administrator's display order for channel monitors.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;

-- Preserve the current ID-descending list order on first rollout.
UPDATE channel_monitors
SET sort_order = (SELECT COALESCE(MAX(id), 0) FROM channel_monitors) - id
WHERE sort_order = 0;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_sort_order
    ON channel_monitors(sort_order, id);
