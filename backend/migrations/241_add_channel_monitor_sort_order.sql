-- Migration: 241_add_channel_monitor_sort_order
-- Stable display ordering for channel monitors.

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 1000;

UPDATE channel_monitors
SET sort_order = 1000
WHERE sort_order IS NULL OR sort_order <= 0;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_sort_order
    ON channel_monitors (sort_order, id);
