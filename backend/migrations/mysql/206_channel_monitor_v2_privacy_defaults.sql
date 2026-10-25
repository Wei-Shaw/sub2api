-- MySQL 方言版本（翻译自 ../206_channel_monitor_v2_privacy_defaults.sql）
-- 翻译说明：NOW() → NOW(3)；`key` 为 MySQL 保留字需反引号。

-- Correct factory defaults introduced by migrations 203 and 204 without
-- mutating those already-applied migration files (which are checksum-locked).
-- The setting was factory-created as false by migration 204. Existing
-- installations need the same privacy-preserving default as new installs.
UPDATE settings
SET value = 'true', updated_at = NOW(3)
WHERE `key` = 'channel_monitor_hide_throughput'
  AND LOWER(TRIM(value)) IN ('false', '0', 'off', 'disabled');
