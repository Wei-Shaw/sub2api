-- MySQL 方言版本（翻译自 ../204_channel_monitor_hide_throughput.sql）
-- Soft switch: hide RPM/TPM throughput rates on user-facing Channel Monitor V2.
-- Default false (rates visible). Admins always see full metrics.
--
-- 说明：`key` 为 MySQL 保留字需反引号；ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。
INSERT INTO settings (`key`, value)
VALUES ('channel_monitor_hide_throughput', 'false')
ON DUPLICATE KEY UPDATE id = id;
