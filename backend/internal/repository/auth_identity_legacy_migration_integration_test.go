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
		identity_adoption_decisions,
		auth_identities,
		auth_identity_migration_reports,
		user_external_identities,
		users
	RESTART IDENTITY CASCADE;
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

func TestAuthIdentityLegacyExternalMigrations_ChainHandlesMalformedAndNonObjectMetadata(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migration115Path := filepath.Join("..", "..", "migrations", "115_auth_identity_legacy_external_backfill.sql")
	migration115SQL, err := os.ReadFile(migration115Path)
REDACTED

	migration116Path := filepath.Join("..", "..", "migrations", "116_auth_identity_legacy_external_safety_reports.sql")
	migration116SQL, err := os.ReadFile(migration116Path)
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
	identity_adoption_decisions,
	auth_identities,
	auth_identity_migration_reports,
	user_external_identities,
	users
RESTART IDENTITY CASCADE;
`)
REDACTED

	var linuxDoMalformedUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-linuxdo-malformed@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&linuxDoMalformedUserID))

	var linuxDoArrayUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-linuxdo-array@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&linuxDoArrayUserID))

	var wechatUnionArrayUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-wechat-array@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&wechatUnionArrayUserID))

	var wechatOpenIDArrayUserID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('legacy-wechat-openid-array@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&wechatOpenIDArrayUserID))

	var linuxDoMalformedLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'linuxdo', 'linuxdo-malformed', NULL, 'legacy-linuxdo-malformed', 'Legacy LinuxDo Malformed', '{invalid')
RETURNING id
`, linuxDoMalformedUserID).Scan(&linuxDoMalformedLegacyID))

	var linuxDoArrayLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'linuxdo', 'linuxdo-array', NULL, 'legacy-linuxdo-array', 'Legacy LinuxDo Array', '["legacy-linuxdo-array"]')
RETURNING id
`, linuxDoArrayUserID).Scan(&linuxDoArrayLegacyID))

	var wechatUnionArrayLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-array', 'union-array', 'legacy-wechat-array', 'Legacy WeChat Array', '["legacy-wechat-array"]')
RETURNING id
`, wechatUnionArrayUserID).Scan(&wechatUnionArrayLegacyID))

	var wechatOpenIDArrayLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-array-only', NULL, 'legacy-wechat-array-only', 'Legacy WeChat Array Only', '["legacy-wechat-openid-array"]')
RETURNING id
`, wechatOpenIDArrayUserID).Scan(&wechatOpenIDArrayLegacyID))

	_, err = tx.ExecContext(ctx, string(migration115SQL))
REDACTED

	_, err = tx.ExecContext(ctx, string(migration116SQL))
REDACTED

	var linuxDoMalformedMetadataType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT jsonb_typeof(metadata)
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'linuxdo'
  AND provider_key = 'linuxdo'
  AND provider_subject = 'linuxdo-malformed'
`, linuxDoMalformedUserID).Scan(&linuxDoMalformedMetadataType))
	require.Equal(t, "object", linuxDoMalformedMetadataType)

	var linuxDoArrayMetadataType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT jsonb_typeof(metadata)
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'linuxdo'
  AND provider_key = 'linuxdo'
  AND provider_subject = 'linuxdo-array'
`, linuxDoArrayUserID).Scan(&linuxDoArrayMetadataType))
	require.Equal(t, "object", linuxDoArrayMetadataType)

	var wechatUnionArrayMetadataType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT jsonb_typeof(metadata)
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'wechat'
  AND provider_key = 'wechat-main'
  AND provider_subject = 'union-array'
`, wechatUnionArrayUserID).Scan(&wechatUnionArrayMetadataType))
	require.Equal(t, "object", wechatUnionArrayMetadataType)

	var invalidJSONReportDetailsType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT jsonb_typeof(details)
FROM auth_identity_migration_reports
WHERE report_type = 'legacy_external_identity_invalid_metadata_json'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(linuxDoMalformedLegacyID, 10)).Scan(&invalidJSONReportDetailsType))
	require.Equal(t, "object", invalidJSONReportDetailsType)

	var openIDOnlyReportDetailsType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT jsonb_typeof(details)
FROM auth_identity_migration_reports
WHERE report_type = 'wechat_openid_only_requires_remediation'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatOpenIDArrayLegacyID, 10)).Scan(&openIDOnlyReportDetailsType))
	require.Equal(t, "object", openIDOnlyReportDetailsType)

	var preservedArrayMetadataCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identities
WHERE id IN (
	SELECT id
	FROM auth_identities
	WHERE (user_id = $1 AND provider_subject = 'linuxdo-array')
	   OR (user_id = $2 AND provider_subject = 'union-array')
)
  AND metadata ? '_legacy_metadata_raw_json'
`, linuxDoArrayUserID, wechatUnionArrayUserID).Scan(&preservedArrayMetadataCount))
	require.Equal(t, 2, preservedArrayMetadataCount)

	require.NotZero(t, linuxDoArrayLegacyID)
	require.NotZero(t, wechatUnionArrayLegacyID)
REDACTED

func TestAuthIdentityLegacyExternalSafetyMigration_ReportsConflictsAndDowngradesInvalidJSON(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "116_auth_identity_legacy_external_safety_reports.sql")
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
		identity_adoption_decisions,
		auth_identities,
		auth_identity_migration_reports,
		user_external_identities,
		users
	RESTART IDENTITY CASCADE;
`)
REDACTED

	userIDs := make([]int64, 0, 8)
	for _, email := range []string{
		"linuxdo-conflict-legacy@example.com",
		"linuxdo-conflict-owner@example.com",
		"wechat-conflict-legacy@example.com",
		"wechat-conflict-owner@example.com",
		"wechat-channel-legacy@example.com",
		"wechat-channel-owner@example.com",
		"linuxdo-invalid-json@example.com",
		"wechat-openid-invalid-json@example.com",
REDACTED {
		var userID int64
		require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ($1, 'hash', 'user', 'active', 0, 1)
RETURNING id`, email).Scan(&userID))
		userIDs = append(userIDs, userID)
REDACTED

	linuxdoConflictLegacyUserID := userIDs[0]
	linuxdoConflictOwnerUserID := userIDs[1]
	wechatConflictLegacyUserID := userIDs[2]
	wechatConflictOwnerUserID := userIDs[3]
	wechatChannelLegacyUserID := userIDs[4]
	wechatChannelOwnerUserID := userIDs[5]
	linuxdoInvalidJSONUserID := userIDs[6]
	wechatInvalidOpenIDUserID := userIDs[7]

	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO auth_identities (user_id, provider_type, provider_key, provider_subject, metadata)
VALUES ($1, 'linuxdo', 'linuxdo', 'linuxdo-conflict', '{REDACTED'::jsonb)
RETURNING id`, linuxdoConflictOwnerUserID).Scan(new(int64)))

	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO auth_identities (user_id, provider_type, provider_key, provider_subject, metadata)
VALUES ($1, 'wechat', 'wechat-main', 'union-conflict', '{REDACTED'::jsonb)
RETURNING id`, wechatConflictOwnerUserID).Scan(new(int64)))

	var wechatChannelOwnerIdentityID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO auth_identities (user_id, provider_type, provider_key, provider_subject, metadata)
VALUES ($1, 'wechat', 'wechat-main', 'union-channel-owner', '{REDACTED'::jsonb)
RETURNING id`, wechatChannelOwnerUserID).Scan(&wechatChannelOwnerIdentityID))

	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO auth_identity_channels (
	identity_id,
	provider_type,
	provider_key,
	channel,
	channel_app_id,
	channel_subject,
	metadata
)
VALUES ($1, 'wechat', 'wechat-main', 'oa', 'wx-app-conflict', 'openid-channel-conflict', '{REDACTED'::jsonb)
RETURNING id`, wechatChannelOwnerIdentityID).Scan(new(int64)))

	var linuxdoConflictLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'linuxdo', 'linuxdo-conflict', NULL, 'legacy-linuxdo', 'Legacy LinuxDo Conflict', '{"source":"legacy"REDACTED')
RETURNING id
`, linuxdoConflictLegacyUserID).Scan(&linuxdoConflictLegacyID))

	var wechatConflictLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-union-conflict', 'union-conflict', 'legacy-wechat', 'Legacy WeChat Conflict', '{"channel":"oa","appid":"wx-app-conflict-canon"REDACTED')
RETURNING id
`, wechatConflictLegacyUserID).Scan(&wechatConflictLegacyID))

	var wechatChannelConflictLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-channel-conflict', 'union-channel-legacy', 'legacy-wechat-channel', 'Legacy WeChat Channel Conflict', '{"channel":"oa","appid":"wx-app-conflict"REDACTED')
RETURNING id
`, wechatChannelLegacyUserID).Scan(&wechatChannelConflictLegacyID))

	var linuxdoInvalidJSONLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'linuxdo', 'linuxdo-invalid-json', NULL, 'legacy-linuxdo-invalid', 'Legacy LinuxDo Invalid JSON', '{invalid')
RETURNING id
`, linuxdoInvalidJSONUserID).Scan(&linuxdoInvalidJSONLegacyID))

	var wechatInvalidOpenIDLegacyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_external_identities (
	user_id,
	provider,
	provider_user_id,
	provider_union_id,
	provider_username,
	display_name,
	metadata
) VALUES ($1, 'wechat', 'openid-invalid-json-only', NULL, 'legacy-wechat-invalid', 'Legacy WeChat Invalid JSON', '{still-invalid')
RETURNING id
`, wechatInvalidOpenIDUserID).Scan(&wechatInvalidOpenIDLegacyID))

	_, err = tx.ExecContext(ctx, string(migrationSQL))
REDACTED

	var linuxdoConflictReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'legacy_external_identity_conflict'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(linuxdoConflictLegacyID, 10)).Scan(&linuxdoConflictReportCount))
	require.Equal(t, 1, linuxdoConflictReportCount)

	var wechatConflictReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'legacy_external_identity_conflict'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatConflictLegacyID, 10)).Scan(&wechatConflictReportCount))
	require.Equal(t, 1, wechatConflictReportCount)

	var channelConflictReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'legacy_external_channel_conflict'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatChannelConflictLegacyID, 10)).Scan(&channelConflictReportCount))
	require.Equal(t, 1, channelConflictReportCount)

	var invalidJSONReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'legacy_external_identity_invalid_metadata_json'
  AND report_key IN ($1, $2)
`, "legacy_external_identity:"+strconv.FormatInt(linuxdoInvalidJSONLegacyID, 10), "legacy_external_identity:"+strconv.FormatInt(wechatInvalidOpenIDLegacyID, 10)).Scan(&invalidJSONReportCount))
	require.Equal(t, 2, invalidJSONReportCount)

	var linuxdoInvalidIdentityCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identities
WHERE user_id = $1
  AND provider_type = 'linuxdo'
  AND provider_key = 'linuxdo'
  AND provider_subject = 'linuxdo-invalid-json'
`, linuxdoInvalidJSONUserID).Scan(&linuxdoInvalidIdentityCount))
	require.Equal(t, 1, linuxdoInvalidIdentityCount)

	var wechatOpenIDOnlyReportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'wechat_openid_only_requires_remediation'
  AND report_key = $1
`, "legacy_external_identity:"+strconv.FormatInt(wechatInvalidOpenIDLegacyID, 10)).Scan(&wechatOpenIDOnlyReportCount))
	require.Equal(t, 1, wechatOpenIDOnlyReportCount)
REDACTED

func TestAuthIdentityLegacyExternalSafetyMigration_IsSafeWhenLegacyTableMissing(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationPath := filepath.Join("..", "..", "migrations", "116_auth_identity_legacy_external_safety_reports.sql")
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
