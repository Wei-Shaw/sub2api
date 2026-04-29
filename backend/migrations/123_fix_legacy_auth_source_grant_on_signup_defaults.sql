-- Auto-backfill untouched migration 110 signup-grant defaults to the corrected false value.
-- Rows still matching the migration-110 default payload and timestamp window are treated as
-- untouched legacy defaults; any remaining legacy true values are reported for manual review.

WITH migration_110 AS (
    SELECT applied_at
    FROM schema_migrations
    WHERE filename = '110_pending_auth_and_provider_default_grants.sql'
),
providers AS (
    SELECT provider_type
    FROM (
        VALUES ('email'), ('linuxdo'), ('oidc'), ('wechat')
    ) AS providers(provider_type)
),
legacy_provider_defaults AS (
    SELECT providers.provider_type
    FROM providers
    CROSS JOIN migration_110
    JOIN settings balance
      ON balance.key = 'auth_source_default_' || providers.provider_type || '_balance'
    JOIN settings concurrency
      ON concurrency.key = 'auth_source_default_' || providers.provider_type || '_concurrency'
    JOIN settings subscriptions
      ON subscriptions.key = 'auth_source_default_' || providers.provider_type || '_subscriptions'
    JOIN settings grant_on_signup
      ON grant_on_signup.key = 'auth_source_default_' || providers.provider_type || '_grant_on_signup'
    JOIN settings grant_on_first_bind
      ON grant_on_first_bind.key = 'auth_source_default_' || providers.provider_type || '_grant_on_first_bind'
    WHERE balance.value = '0'
      AND concurrency.value = '5'
      AND subscriptions.value = '[]'
      AND grant_on_signup.value = 'true'
      AND grant_on_first_bind.value = 'false'
      AND balance.updated_at BETWEEN migration_110.applied_at - INTERVAL '1 minute' AND migration_110.applied_at + INTERVAL '1 minute'
      AND concurrency.updated_at BETWEEN migration_110.applied_at - INTERVAL '1 minute' AND migration_110.applied_at + INTERVAL '1 minute'
      AND subscriptions.updated_at BETWEEN migration_110.applied_at - INTERVAL '1 minute' AND migration_110.applied_at + INTERVAL '1 minute'
      AND grant_on_signup.updated_at BETWEEN migration_110.applied_at - INTERVAL '1 minute' AND migration_110.applied_at + INTERVAL '1 minute'
      AND grant_on_first_bind.updated_at BETWEEN migration_110.applied_at - INTERVAL '1 minute' AND migration_110.applied_at + INTERVAL '1 minute'
),
updated_signup_grants AS (
    UPDATE settings
    SET
        value = 'false',
        updated_at = NOW()
    FROM legacy_provider_defaults
    WHERE settings.key = 'auth_source_default_' || legacy_provider_defaults.provider_type || '_grant_on_signup'
      AND settings.value = 'true'
    RETURNING legacy_provider_defaults.provider_type
)
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_auth_source_signup_grant_review',
    providers.provider_type,
    jsonb_build_object(
        'provider_type', providers.provider_type,
        'current_value', grant_on_signup.value,
        'auto_backfilled', FALSE,
        'reason', 'legacy_true_default_not_auto_backfilled'
    )
FROM providers
JOIN settings grant_on_signup
  ON grant_on_signup.key = 'auth_source_default_' || providers.provider_type || '_grant_on_signup'
LEFT JOIN updated_signup_grants
  ON updated_signup_grants.provider_type = providers.provider_type
WHERE grant_on_signup.value = 'true'
  AND updated_signup_grants.provider_type IS NULL
ON CONFLICT (report_type, report_key) DO NOTHING;
