-- Migration 142: rename settings row key from
--   "service_quota.precheck_two_phase" (dot-delimited)
-- to
--   "service_quota_precheck_two_phase" (underscore-delimited)
--
-- Background:
--   Service Quota originally shipped one setting key in dot-delimited
--   namespace style (service_quota.precheck_two_phase) while every other
--   ~143 SettingKey* constants in this codebase use pure underscore style
--   (e.g. service_quota_enabled, channel_monitor_enabled, smtp_host).
--   Per the project's "unify naming for the same concept" rule, align this
--   outlier with the dominant project convention.
--
-- Impact:
--   - The Go constant SettingKeyServiceQuotaPreCheckTwoPhase value flips
--     from "service_quota.precheck_two_phase" to
--     "service_quota_precheck_two_phase".
--   - This migration rewrites any matching settings.key row so existing
--     fork deployments retain their configured boolean value across upgrade.
--
-- Idempotency:
--   - WHERE clause restricts the UPDATE to the dot-delimited legacy key, so
--     re-running is a no-op (0 rows affected).
--   - Fresh deployments / upstream installs that have never written this
--     setting also see 0 rows affected; not an error.
--
-- Upstream:
--   upstream/main never shipped service_quota before this PR, so on stock
--   upstream installs this migration is effectively a no-op.

UPDATE settings
SET key = 'service_quota_precheck_two_phase'
WHERE key = 'service_quota.precheck_two_phase';
