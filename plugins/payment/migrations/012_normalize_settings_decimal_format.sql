-- Migration 012: normalise payment plugin settings money values from JSON
-- number to JSON string form so the read/write round-trip preserves
-- shopspring/decimal precision after the decimal.Decimal migration.
--
-- Background:
--   The 5 affected rows were originally INSERTed as JSON numbers by host
--   migration 145 (when settings moved from the legacy `settings` table to
--   plugin_settings). After the payment plugin's float64 -> decimal.Decimal
--   cutover, new writes from the SDK encode the values as JSON strings
--   ("0.96") and readDecimalSetting tolerates both shapes via a fallback
--   path. This migration normalises pre-cutover rows so the column is
--   uniform and a future cleanup can drop the legacy float64 fallback.
--
-- Idempotency:
--   The WHERE clause filters on jsonb_typeof = 'number', so re-running on
--   an already-stringified row is a no-op. Re-applying the migration after
--   admins have edited individual fields will only touch fields they have
--   not touched (since admin saves go through the SDK and write strings).
--
-- Scope:
--   Only the 5 keys whose Go type changed to decimal.Decimal. Boolean,
--   integer, and string-typed settings are unaffected.

UPDATE plugin_settings
SET value_json = to_jsonb(value_json #>> '{}')
WHERE plugin_name = 'payment'
  AND key IN (
    'min_recharge_amount',
    'max_recharge_amount',
    'daily_recharge_limit',
    'balance_recharge_multiplier',
    'recharge_fee_rate'
  )
  AND jsonb_typeof(value_json) = 'number';
