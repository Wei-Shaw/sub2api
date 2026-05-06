-- Migration 013: normalise cancel_rate_limit_window_mode legacy 'sliding' value
-- to 'rolling' so the value satisfies the plugin's settings schema.
--
-- Background:
--   Host migration 145 originally seeded plugin_settings.cancel_rate_limit_window_mode
--   from the legacy host setting CANCEL_RATE_LIMIT_WINDOW_MODE, defaulting to
--   'sliding' when absent. The payment plugin's settings_schema.json only
--   permits {'fixed','rolling'}, so any plugin_settings row carrying
--   'sliding' fails schema validation when read by the SDK.
--
--   Migration 145 has been amended to map 'sliding' -> 'rolling' for fresh
--   installs; this migration normalises any rows that were already inserted
--   with 'sliding' before the amendment landed.
--
-- Idempotency:
--   The WHERE clause filters on the literal jsonb value '"sliding"', so
--   re-running this migration on already-normalised rows is a no-op. Admins
--   who later re-select 'fixed' or other valid values via the SDK are not
--   touched (the filter only matches the exact 'sliding' value).

UPDATE plugin_settings
SET value_json = '"rolling"'::jsonb
WHERE plugin_name = 'payment'
  AND key = 'cancel_rate_limit_window_mode'
  AND value_json::text = '"sliding"';