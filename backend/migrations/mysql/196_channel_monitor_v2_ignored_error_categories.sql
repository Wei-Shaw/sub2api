-- MySQL 方言版本（翻译自 ../196_channel_monitor_v2_ignored_error_categories.sql）
-- Categories excluded from error_rate / health scoring (still shown in error breakdown).
--
-- 说明：TEXT[]（默认 '{}'）→ JSON，默认空数组 DEFAULT ('[]')（数组语义）。

ALTER TABLE channel_monitor_v2_config
    ADD COLUMN ignored_error_categories JSON NOT NULL DEFAULT ('[]');
