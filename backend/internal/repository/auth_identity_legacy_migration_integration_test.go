//go:build integration

package repository

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthIdentityLegacyExternalBackfillMigration(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "115_auth_identity_legacy_external_backfill.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
REDACTED

	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS user_external_identities (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
	provider TEXT NOT NULL,
	provider_user_id TEXT NOT NULL,
	provider_union_id TEXT NULL,
	provider_username TEXT NOT NULL DEFAULT '',
	display_name TEXT NOT NULL DEFAULT '',
	profile_url TEXT NOT NULL DEFAULT '',
	avatar_url TEXT NOT NULL DEFAULT '',
	metadata TEXT NOT NULL DEFAULT '{REDACTED',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

	TRUNCATE TABLE
		auth_identity_channels,
		auth_identities,
		auth_identity_migration_reports,
		user_external_identities,
		users
	RESTART IDENTITY;
`)
REDACTED

	var linuxDoUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-linuxdo@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&linuxDoUserID))

	var wechatUnionUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-wechat-union@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&wechatUnionUserID))

	var wechatOpenIDOnlyUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-wechat-openid@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&wechatOpenIDOnlyUserID))

	var syntheticAuthIdentityID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO auth_identities (user_id, provider_type, provider_key, provider_subject, metadata)
VALUES ($1, 'wechat', 'wechat-main', 'openid-synthetic', '{"backfill_source":"synthetic_email"REDACTED'::jsonb)
RETURNING id`, wechatOpenIDOnlyUserID).Scan(&syntheticAuthIdentityID))

	var linuxDoLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'linuxdo', 'linuxdo-user-1', NULL, 'linux-user', 'Linux User', '{"source":"legacy"REDACTED')
RETURNING id
`, linuxDoUserID).Scan(&linuxDoLegacyID))

	var wechatUnionLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-union-1', 'union-1', 'wechat-union-user', 'WeChat Union User', '{"channel":"oa","appid":"wx-app-1"REDACTED')
RETURNING id
`, wechatUnionUserID).Scan(&wechatUnionLegacyID))

	var wechatOpenIDLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-only-1', NULL, 'wechat-openid-user', 'WeChat OpenID User', '{"channel":"oa","appid":"wx-app-2"REDACTED')
RETURNING id
`, wechatOpenIDOnlyUserID).Scan(&wechatOpenIDLegacyID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
REDACTED

	var linuxDoCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'linuxdo'
  AND provider_key = 'linuxdo'
  AND provider_subject = 'linuxdo-user-1'
`, linuxDoUserID).Scan(&linuxDoCount))
	require.Equal(t, 1, linuxDoCount)

	var wechatSubject string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT provider_subject
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'wechat'
  AND provider_key = 'wechat-main'
  AND provider_subject = 'union-1'
`, wechatUnionUserID).Scan(&wechatSubject))
	require.Equal(t, "union-1", wechatSubject)

	var wechatChannelCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_channels channel
JOIN auth_identities ai ON ai.id = channel.identity_id
WHERE ai.user_id = $1
  AND channel.provider_type = 'wechat'
  AND channel.provider_key = 'wechat-main'
  AND channel.channel = 'oa'
  AND channel.channel_app_id = 'wx-app-1'
  AND channel.channel_subject = 'openid-union-1'
`, wechatUnionUserID).Scan(&wechatChannelCount))
	require.Equal(t, 1, wechatChannelCount)

	var legacyOpenIDOnlyReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'wechat_openid_only_requires_remediation'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatOpenIDLegacyID, 10)).Scan(&legacyOpenIDOnlyReportCount))
	require.Equal(t, 1, legacyOpenIDOnlyReportCount)

	var syntheticReviewCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'wechat_openid_only_requires_remediation'
  AND report_key = $1
`, "synthetic_auth_identity:"+strconv.FormatInt(syntheticAuthIdentityID, 10)).Scan(&syntheticReviewCount))
	require.Equal(t, 1, syntheticReviewCount)

	var unionLegacyReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'wechat_openid_only_requires_remediation'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatUnionLegacyID, 10)).Scan(&unionLegacyReportCount))
	require.Zero(t, unionLegacyReportCount)
	require.NotZero(t, linuxDoLegacyID)
REDACTED

func TestAuthIdentityLegacyExternalBackfillMigration_IsSafeWhenLegacyTableMissing(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "115_auth_identity_legacy_external_backfill.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
REDACTED

	var beforeCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
`).Scan(&beforeCount))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
REDACTED

	var afterCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
`).Scan(&afterCount))
	require.Equal(t, beforeCount, afterCount)
REDACTED
