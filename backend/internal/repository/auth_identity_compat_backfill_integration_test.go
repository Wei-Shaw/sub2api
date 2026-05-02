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

func TestAuthIdentityCompatBackfillMigration_AllowsLongReportTypes(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migration108Path := filepath.Join("..", "..", "migrations", "108_auth_identity_foundation_core.sql")
	migration108SQL, err := os.ReadFile(migration108Path)
	require.NoError(t, err)

	migration108aPath := filepath.Join("..", "..", "migrations", "108a_widen_auth_identity_migration_report_type.sql")
	migration108aSQL, err := os.ReadFile(migration108aPath)
	require.NoError(t, err)

	migration109Path := filepath.Join("..", "..", "migrations", "109_auth_identity_compat_backfill.sql")
	migration109SQL, err := os.ReadFile(migration109Path)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
DROP TABLE IF EXISTS auth_identity_migration_reports CASCADE;
DROP TABLE IF EXISTS auth_identity_channels CASCADE;
DROP TABLE IF EXISTS identity_adoption_decisions CASCADE;
DROP TABLE IF EXISTS pending_auth_sessions CASCADE;
DROP TABLE IF EXISTS auth_identities CASCADE;

ALTER TABLE users
	DROP COLUMN IF EXISTS signup_source,
	DROP COLUMN IF EXISTS last_login_at,
	DROP COLUMN IF EXISTS last_active_at;
`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migration108SQL))
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(migration108aSQL))
	require.NoError(t, err)

	var userID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('oidc-demo-subject@oidc-connect.invalid', 'hash', 'user', 'active', 0, 1)
RETURNING id`).Scan(&userID))

	_, err = tx.ExecContext(ctx, string(migration109SQL))
	require.NoError(t, err)

	var reportCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM auth_identity_migration_reports
WHERE report_type = 'oidc_synthetic_email_requires_manual_recovery'
  AND report_key = $1
`, strconv.FormatInt(userID, 10)).Scan(&reportCount))
	require.Equal(t, 1, reportCount)

	var reportTypeLimit int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT character_maximum_length
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'auth_identity_migration_reports'
  AND column_name = 'report_type'
`).Scan(&reportTypeLimit))
	require.GreaterOrEqual(t, reportTypeLimit, 45)

	require.NotZero(t, userID)
}
