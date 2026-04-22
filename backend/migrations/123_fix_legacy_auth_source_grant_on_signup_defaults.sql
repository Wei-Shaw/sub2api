WITH migration_110 AS (
    SELECT applied_at
    FROM schema_migrations
    WHERE filename = '110_pending_auth_and_provider_default_grants.sql'
),
legacy_provider_defaults AS (
    SELECT provider_type
    FROM (
        VALUES ('email'), ('linuxdo'), ('oidc'), ('wechat')
    ) AS providers(provider_type)
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
)
UPDATE settings
SET
    value = 'false',
    updated_at = NOW()
FROM legacy_provider_defaults
WHERE settings.key = 'auth_source_default_' || legacy_provider_defaults.provider_type || '_grant_on_signup'
  AND settings.value = 'true';
