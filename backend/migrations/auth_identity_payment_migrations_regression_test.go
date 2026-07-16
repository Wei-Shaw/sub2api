package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration112UsesIdempotentAddColumn(t *testing.T) {
	content, err := FS.ReadFile("112_add_payment_order_provider_key_snapshot.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30)")
	require.NotContains(t, sql, "ADD COLUMN provider_key VARCHAR(30);")
}

func TestMigration118DoesNotForceOverwriteAuthSourceGrantDefaults(t *testing.T) {
	content, err := FS.ReadFile("118_wechat_dual_mode_and_auth_source_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "UPDATE settings")
	require.NotContains(t, sql, "SET value = 'false'")
	require.True(t, strings.Contains(sql, "ON CONFLICT (key) DO NOTHING"))
	require.Contains(t, sql, "THEN ''")
}

func TestAuthIdentityReportTypeWideningRunsBeforeLongReportWritersAndStillReconcilesAt121(t *testing.T) {
	preflightContent, err := FS.ReadFile("108a_widen_auth_identity_migration_report_type.sql")
	require.NoError(t, err)

	preflightSQL := string(preflightContent)
	require.Contains(t, preflightSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, preflightSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")

	content, err := FS.ReadFile("109_auth_identity_compat_backfill.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "ALTER TABLE auth_identity_migration_reports")

	followupContent, err := FS.ReadFile("121_auth_identity_migration_report_type_widen.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, followupSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")
}

func TestMigration119DefersPaymentIndexRolloutToOnlineFollowup(t *testing.T) {
	content, err := FS.ReadFile("119_enforce_payment_orders_out_trade_no_unique.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.Contains(t, sql, "NULL;")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "DROP INDEX")

	followupContent, err := FS.ReadFile("120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "explicit duplicate out_trade_no precheck")
	require.Contains(t, followupSQL, "stale invalid paymentorder_out_trade_no_unique index")
	require.Contains(t, followupSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique")
	require.NotContains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no_unique")
	require.Contains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no")
	require.Contains(t, followupSQL, "WHERE out_trade_no <> ''")

	alignmentContent, err := FS.ReadFile("120a_align_payment_orders_out_trade_no_index_name.sql")
	require.NoError(t, err)

	alignmentSQL := string(alignmentContent)
	require.Contains(t, alignmentSQL, "paymentorder_out_trade_no_unique")
	require.Contains(t, alignmentSQL, "RENAME TO paymentorder_out_trade_no")
}

func TestMigration110SeedsAuthSourceSignupGrantsDisabledByDefault(t *testing.T) {
	content, err := FS.ReadFile("110_pending_auth_and_provider_default_grants.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "('auth_source_default_email_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_linuxdo_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_oidc_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_wechat_grant_on_signup', 'false')")
	require.NotContains(t, sql, "('auth_source_default_email_grant_on_signup', 'true')")
}

func TestMigration122ScrubsPendingOAuthCompletionTokensAtRest(t *testing.T) {
	content, err := FS.ReadFile("122_pending_auth_completion_token_cleanup.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE pending_auth_sessions")
	require.Contains(t, sql, "completion_response")
	require.Contains(t, sql, "access_token")
	require.Contains(t, sql, "refresh_token")
	require.Contains(t, sql, "expires_in")
	require.Contains(t, sql, "token_type")
}

func TestMigration123BackfillsLegacyAuthSourceGrantDefaultsSafely(t *testing.T) {
	content, err := FS.ReadFile("123_fix_legacy_auth_source_grant_on_signup_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "110_pending_auth_and_provider_default_grants.sql")
	require.Contains(t, sql, "schema_migrations")
	require.Contains(t, sql, "updated_at")
	require.Contains(t, sql, "'_grant_on_signup'")
	require.Contains(t, sql, "value = 'false'")
	require.Contains(t, sql, "auth_identity_migration_reports")
}

func TestMigration124BackfillsLegacyOIDCSecurityFlagsSafely(t *testing.T) {
	content, err := FS.ReadFile("124_backfill_legacy_oidc_security_flags.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "oidc_connect_use_pkce")
	require.Contains(t, sql, "oidc_connect_validate_id_token")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, "oidc_connect_enabled")
	require.Contains(t, sql, "'false'")
}

func TestMigration134AddsAffiliateLedgerAuditFieldsWithoutJSONCast(t *testing.T) {
	content, err := FS.ReadFile("134_affiliate_ledger_audit_snapshots.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS source_order_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS balance_after DECIMAL(20,8)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS aff_quota_after DECIMAL(20,8)")
	require.Contains(t, sql, "substring(")
	require.Contains(t, sql, `"rebateAmount"`)
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ra.order_id) AS order_match_count")
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ual.id) AS ledger_match_count")
	require.NotContains(t, sql, "detail::jsonb")
}

func TestMigration135AllowsGitHubAndGoogleAuthProviders(t *testing.T) {
	content, err := FS.ReadFile("135_allow_email_oauth_provider_types.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "users_signup_source_check")
	require.Contains(t, sql, "auth_identities_provider_type_check")
	require.Contains(t, sql, "auth_identity_channels_provider_type_check")
	require.Contains(t, sql, "pending_auth_sessions_provider_type_check")
	require.Contains(t, sql, "'github'")
	require.Contains(t, sql, "'google'")
}

func TestMigration151AddsAccountAutoPauseExpiryPartialIndex(t *testing.T) {
	content, err := FS.ReadFile("151_account_autopause_expiry_index_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_autopause_expiry_due")
	require.Contains(t, sql, "ON accounts (expires_at)")
	require.Contains(t, sql, "WHERE deleted_at IS NULL")
	require.Contains(t, sql, "schedulable = TRUE")
	require.Contains(t, sql, "auto_pause_on_expired = TRUE")
	require.Contains(t, sql, "expires_at IS NOT NULL")
}

func TestMigration158BackfillsGrokMediaGenerationGroups(t *testing.T) {
	content, err := FS.ReadFile("158_enable_grok_media_generation_groups.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE groups")
	require.Contains(t, sql, "SET allow_image_generation = true")
	require.Contains(t, sql, "WHERE platform = 'grok'")
	require.Contains(t, sql, "AND allow_image_generation = false")
}

func TestMigration154AddsSparkShadowColumnsAndConstraintsWithoutHotIndexes(t *testing.T) {
	content, err := FS.ReadFile("154_account_spark_shadow.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS parent_account_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota_dimension VARCHAR(20) NOT NULL DEFAULT 'global'")
	require.Contains(t, sql, "chk_accounts_parent_dimension")
	// 约束已放开为「影子 ⇒ 非 global 维度」（spark 不再写死进 parent 约束）
	require.Contains(t, sql, "parent_account_id IS NOT NULL AND quota_dimension <> 'global'")
	require.NotContains(t, sql, "parent_account_id IS NOT NULL AND quota_dimension = 'spark'")
	require.Contains(t, sql, "chk_accounts_parent_not_self")
	require.Contains(t, sql, "fk_accounts_parent_account_id")
	require.Contains(t, sql, "FOREIGN KEY (parent_account_id) REFERENCES accounts(id)")
	require.Contains(t, sql, "ON DELETE RESTRICT")
	require.Contains(t, sql, "NOT VALID")
	require.NotContains(t, sql, "CREATE INDEX")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "CONCURRENTLY")
}

func TestMigration154aAddsSparkShadowIndexesConcurrently(t *testing.T) {
	content, err := FS.ReadFile("154a_account_spark_shadow_indexes_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_parent_account_id")
	require.Contains(t, sql, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_spark_shadow_per_parent")
	require.Contains(t, sql, "ON accounts (parent_account_id)")
	require.Contains(t, sql, "WHERE parent_account_id IS NOT NULL")
	require.Contains(t, sql, "quota_dimension = 'spark'")
	require.Contains(t, sql, "deleted_at IS NULL")
}

func TestMigration173AllowsCyberBlockedUsageRequestType(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)

	previousIndex := -1
	currentIndex := -1
	for i, entry := range entries {
		switch entry.Name() {
		case "172_video_per_second_billing_metadata.sql":
			previousIndex = i
		case "173_allow_cyber_blocked_usage_request_type.sql":
			currentIndex = i
		}
	}
	require.NotEqual(t, -1, previousIndex)
	require.NotEqual(t, -1, currentIndex)
	require.Less(t, previousIndex, currentIndex)

	content, err := FS.ReadFile("173_allow_cyber_blocked_usage_request_type.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS usage_logs_request_type_check")
	require.Contains(t, sql, "ADD CONSTRAINT usage_logs_request_type_check")
	require.Contains(t, sql, "CHECK (request_type IN (0, 1, 2, 3, 4)) NOT VALID")
}
