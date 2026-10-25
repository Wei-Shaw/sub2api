-- MySQL 方言版本（翻译自 ../123_fix_legacy_auth_source_grant_on_signup_defaults.sql）
-- Auto-backfill untouched migration 110 signup-grant defaults to the corrected false value.
-- Rows still matching the migration-110 default payload and timestamp window are treated as
-- untouched legacy defaults; any remaining legacy true values are reported for manual review.
--
-- 翻译说明：
-- - PG 版使用 data-modifying CTE（WITH ... UPDATE ... RETURNING）+ INSERT ... ON CONFLICT，
--   MySQL 无 RETURNING，按语义拆为两步：
--     1) UPDATE：把「完全匹配迁移 110 遗留默认值组合且 grant_on_signup='true'」的
--        auth_source_default_<provider>_grant_on_signup 回填为 'false'；
--     2) INSERT 报告：回填后 current value 仍为 'true' 的 provider（即未被自动回填的），
--        写入 auth_identity_migration_reports 供人工复核。
--   与 PG 版语义等价：被回填的行值变为 'false' 不会再被报告；未被回填且仍为 'true'
--   的行（无论是否匹配完整遗留组合）都会被报告，current_value 恒为 'true'（与 PG
--   快照语义下的实际取值一致）。
-- - ON CONFLICT (report_type, report_key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。

-- -----------------------------------------------------------------------------
-- 1) 回填未被触碰的迁移 110 signup-grant 默认值
-- -----------------------------------------------------------------------------
UPDATE settings
JOIN (
    SELECT p.provider_type
    FROM (
        SELECT 'email' AS provider_type
        UNION ALL SELECT 'linuxdo'
        UNION ALL SELECT 'oidc'
        UNION ALL SELECT 'wechat'
    ) AS p
    JOIN schema_migrations m
      ON m.filename = '110_pending_auth_and_provider_default_grants.sql'
    JOIN settings balance
      ON balance.`key` = CONCAT('auth_source_default_', p.provider_type, '_balance')
    JOIN settings concurrency
      ON concurrency.`key` = CONCAT('auth_source_default_', p.provider_type, '_concurrency')
    JOIN settings subscriptions
      ON subscriptions.`key` = CONCAT('auth_source_default_', p.provider_type, '_subscriptions')
    JOIN settings grant_on_signup
      ON grant_on_signup.`key` = CONCAT('auth_source_default_', p.provider_type, '_grant_on_signup')
    JOIN settings grant_on_first_bind
      ON grant_on_first_bind.`key` = CONCAT('auth_source_default_', p.provider_type, '_grant_on_first_bind')
    WHERE balance.value = '0'
      AND concurrency.value = '5'
      AND subscriptions.value = '[]'
      AND grant_on_signup.value = 'true'
      AND grant_on_first_bind.value = 'false'
      AND balance.updated_at BETWEEN m.applied_at - INTERVAL 1 MINUTE AND m.applied_at + INTERVAL 1 MINUTE
      AND concurrency.updated_at BETWEEN m.applied_at - INTERVAL 1 MINUTE AND m.applied_at + INTERVAL 1 MINUTE
      AND subscriptions.updated_at BETWEEN m.applied_at - INTERVAL 1 MINUTE AND m.applied_at + INTERVAL 1 MINUTE
      AND grant_on_signup.updated_at BETWEEN m.applied_at - INTERVAL 1 MINUTE AND m.applied_at + INTERVAL 1 MINUTE
      AND grant_on_first_bind.updated_at BETWEEN m.applied_at - INTERVAL 1 MINUTE AND m.applied_at + INTERVAL 1 MINUTE
) legacy
  ON settings.`key` = CONCAT('auth_source_default_', legacy.provider_type, '_grant_on_signup')
SET settings.value = 'false',
    settings.updated_at = NOW(3)
WHERE settings.value = 'true';

-- -----------------------------------------------------------------------------
-- 2) 剩余仍为 'true' 的 provider 记录人工复核报告
-- -----------------------------------------------------------------------------
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_auth_source_signup_grant_review',
    p.provider_type,
    JSON_OBJECT(
        'provider_type', p.provider_type,
        'current_value', grant_on_signup.value,
        'auto_backfilled', FALSE,
        'reason', 'legacy_true_default_not_auto_backfilled'
    )
FROM (
    SELECT 'email' AS provider_type
    UNION ALL SELECT 'linuxdo'
    UNION ALL SELECT 'oidc'
    UNION ALL SELECT 'wechat'
) AS p
JOIN settings grant_on_signup
  ON grant_on_signup.`key` = CONCAT('auth_source_default_', p.provider_type, '_grant_on_signup')
WHERE grant_on_signup.value = 'true'
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;
