-- Categories excluded from error_rate / health scoring (still shown in error breakdown).
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN IF NOT EXISTS ignored_error_categories TEXT[] NOT NULL DEFAULT '{}';
