-- MySQL 方言版本（翻译自 ../195_channel_monitor_mode.sql）
-- Channel monitor exclusive mode: v1 (active probes) or v2 (passive aggregation).
-- Default v1 preserves existing active-probe behavior; operators opt in to v2.
--
-- 说明：`key` 为 MySQL 保留字需反引号；ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。
INSERT INTO settings (`key`, value)
VALUES ('channel_monitor_mode', 'v1')
ON DUPLICATE KEY UPDATE id = id;
