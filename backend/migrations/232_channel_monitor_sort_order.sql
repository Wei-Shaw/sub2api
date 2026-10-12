-- Persist the administrator's display order for channel monitors.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 2147483647;

ALTER TABLE channel_monitors
    ALTER COLUMN sort_order SET DEFAULT 2147483647;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_sort_order
    ON channel_monitors(sort_order, id);
