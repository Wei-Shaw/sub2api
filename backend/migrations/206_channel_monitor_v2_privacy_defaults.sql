-- Correct factory defaults introduced by migrations 203 and 204 without
-- mutating those already-applied migration files (which are checksum-locked).
-- The setting was factory-created as false by migration 204. Existing
-- installations need the same privacy-preserving default as new installs.
UPDATE settings
SET value = 'true', updated_at = NOW()
WHERE key = 'channel_monitor_hide_throughput'
  AND LOWER(TRIM(value)) IN ('false', '0', 'off', 'disabled');
