-- Migration 145: copy payment_* keys from the host's `settings` table into
-- the `plugin_settings` table under plugin_name='payment'.
--
-- Background:
--   The payment system has been extracted into a gRPC plugin (plugins/payment).
--   The plugin owns its own settings namespace and reads them via the host's
--   plugin_settings table. To preserve operator configuration across the
--   cut-over the host migration writes one plugin_settings row per setting
--   key, transforming legacy SCREAMING_SNAKE / mixed-case names to the
--   plugin's canonical snake_case keys defined in
--   plugins/payment/internal/settings/settings_schema.json.
--
-- Idempotency:
--   ON CONFLICT DO NOTHING — re-running the migration after operators have
--   tuned plugin settings via the new admin form must NOT clobber their
--   values. Initial cut-over only.
--
-- Notes:
--   - value column in `settings` is TEXT, value_json in plugin_settings is JSONB.
--   - Boolean values in settings are stored as the string 'true' / 'false'.
--   - Numeric values are stored as text and cast at read time.
--   - ENABLED_PAYMENT_TYPES is a CSV; we cast to a JSONB array.
--   - Cancel rate-limit settings are window+unit+mode tuples.
--   - Stripe-specific provider config lives in payment_provider_instances
--     (already migrated to the plugin's ent client) — no settings here.

INSERT INTO plugin_settings (plugin_name, key, value_json, revision, updated_at)
SELECT 'payment', plugin_key, plugin_value, 1, NOW()
FROM (
    -- enabled
    SELECT 'enabled' AS plugin_key,
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::boolean FROM settings WHERE key='payment_enabled'), false)) AS plugin_value
    UNION ALL
    -- min_recharge_amount (numeric)
    SELECT 'min_recharge_amount',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::numeric FROM settings WHERE key='MIN_RECHARGE_AMOUNT'), 1::numeric))
    UNION ALL
    -- max_recharge_amount
    SELECT 'max_recharge_amount',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::numeric FROM settings WHERE key='MAX_RECHARGE_AMOUNT'), 10000::numeric))
    UNION ALL
    -- daily_recharge_limit
    SELECT 'daily_recharge_limit',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::numeric FROM settings WHERE key='DAILY_RECHARGE_LIMIT'), 0::numeric))
    UNION ALL
    -- order_timeout_minutes
    SELECT 'order_timeout_minutes',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::int FROM settings WHERE key='ORDER_TIMEOUT_MINUTES'), 30))
    UNION ALL
    -- max_pending_orders
    SELECT 'max_pending_orders',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::int FROM settings WHERE key='MAX_PENDING_ORDERS'), 3))
    UNION ALL
    -- enabled_payment_types: CSV -> jsonb array
    SELECT 'enabled_payment_types',
           COALESCE(
             (SELECT to_jsonb(ARRAY(
                 SELECT trim(part) FROM regexp_split_to_table(value, ',') AS part WHERE trim(part) <> ''
             )) FROM settings WHERE key='ENABLED_PAYMENT_TYPES'),
             '[]'::jsonb
           )
    UNION ALL
    -- load_balance_strategy
    SELECT 'load_balance_strategy',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='LOAD_BALANCE_STRATEGY'), 'round_robin'))
    UNION ALL
    -- balance_payment_disabled
    SELECT 'balance_payment_disabled',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::boolean FROM settings WHERE key='BALANCE_PAYMENT_DISABLED'), false))
    UNION ALL
    -- balance_recharge_multiplier
    SELECT 'balance_recharge_multiplier',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::numeric FROM settings WHERE key='BALANCE_RECHARGE_MULTIPLIER'), 1::numeric))
    UNION ALL
    -- recharge_fee_rate
    SELECT 'recharge_fee_rate',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::numeric FROM settings WHERE key='RECHARGE_FEE_RATE'), 0::numeric))
    UNION ALL
    -- product_name_prefix
    SELECT 'product_name_prefix',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='PRODUCT_NAME_PREFIX'), ''))
    UNION ALL
    -- product_name_suffix
    SELECT 'product_name_suffix',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='PRODUCT_NAME_SUFFIX'), ''))
    UNION ALL
    -- help_image_url
    SELECT 'help_image_url',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='PAYMENT_HELP_IMAGE_URL'), ''))
    UNION ALL
    -- help_text
    SELECT 'help_text',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='PAYMENT_HELP_TEXT'), ''))
    UNION ALL
    -- cancel_rate_limit_enabled
    SELECT 'cancel_rate_limit_enabled',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::boolean FROM settings WHERE key='CANCEL_RATE_LIMIT_ENABLED'), false))
    UNION ALL
    -- cancel_rate_limit_max
    SELECT 'cancel_rate_limit_max',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::int FROM settings WHERE key='CANCEL_RATE_LIMIT_MAX'), 0))
    UNION ALL
    -- cancel_rate_limit_window
    SELECT 'cancel_rate_limit_window',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::int FROM settings WHERE key='CANCEL_RATE_LIMIT_WINDOW'), 0))
    UNION ALL
    -- cancel_rate_limit_unit
    SELECT 'cancel_rate_limit_unit',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='CANCEL_RATE_LIMIT_UNIT'), 'minute'))
    UNION ALL
    -- cancel_rate_limit_window_mode
    SELECT 'cancel_rate_limit_window_mode',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='CANCEL_RATE_LIMIT_WINDOW_MODE'), 'sliding'))
    UNION ALL
    -- visible_method_alipay_enabled
    SELECT 'visible_method_alipay_enabled',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::boolean FROM settings WHERE key='payment_visible_method_alipay_enabled'), true))
    UNION ALL
    -- visible_method_alipay_source
    SELECT 'visible_method_alipay_source',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='payment_visible_method_alipay_source'), 'auto'))
    UNION ALL
    -- visible_method_wxpay_enabled
    SELECT 'visible_method_wxpay_enabled',
           to_jsonb(COALESCE((SELECT NULLIF(value, '')::boolean FROM settings WHERE key='payment_visible_method_wxpay_enabled'), true))
    UNION ALL
    -- visible_method_wxpay_source
    SELECT 'visible_method_wxpay_source',
           to_jsonb(COALESCE((SELECT value FROM settings WHERE key='payment_visible_method_wxpay_source'), 'auto'))
) AS migrated
ON CONFLICT (plugin_name, key) DO NOTHING;
