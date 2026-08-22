//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestUserAccountIDMigrationReassignsIAMUsers(t *testing.T) {
	ctx := context.Background()
	conn, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SET search_path TO public`)
		_ = conn.Close()
	}()

	schemaName := "public_id_phase_" + uuid.NewString()
	schema := pq.QuoteIdentifier(schemaName)
	_, err = conn.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer func() { _, _ = integrationDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	_, err = conn.ExecContext(ctx, "SET search_path TO "+schema)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `
		CREATE TABLE users (
			id BIGSERIAL PRIMARY KEY,
			email VARCHAR(255) NOT NULL,
			deleted_at TIMESTAMPTZ
		)`)
	require.NoError(t, err)

	phaseOne, err := dbmigrations.FS.ReadFile("187_public_account_identifiers.sql")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(phaseOne))
	require.NoError(t, err)

	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type) VALUES('root@example.com','1719905235756637','1719905235756637','root')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type,login_name) VALUES('iam@example.com','1719905235756637','201705485041478971','iam','finance.reader')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type,login_name) VALUES('iam2@example.com','1719905235756637','237123304625250835','iam','test231')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type,login_name) VALUES('iam3@example.com','2719905235756637','271990523575663701','iam','already.unique')`)
	require.NoError(t, err)

	migration, err := dbmigrations.FS.ReadFile("231_user_account_ids.sql")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	var total, duplicate int
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total))
	require.Equal(t, 4, total)
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT account_id FROM users GROUP BY account_id HAVING COUNT(*) > 1) duplicates`).Scan(&duplicate))
	require.Zero(t, duplicate)

	var rootAccount, iamAccount, iam2Account, uniqueIAMAccount string
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT account_id FROM users WHERE identity_type='root'`).Scan(&rootAccount))
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT account_id FROM users WHERE login_name='finance.reader'`).Scan(&iamAccount))
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT account_id FROM users WHERE login_name='test231'`).Scan(&iam2Account))
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT account_id FROM users WHERE login_name='already.unique'`).Scan(&uniqueIAMAccount))
	require.Equal(t, "1719905235756637", rootAccount)
	require.NotEqual(t, rootAccount, iamAccount)
	require.NotEqual(t, rootAccount, iam2Account)
	require.NotEqual(t, iamAccount, iam2Account)
	require.Equal(t, "2719905235756637", uniqueIAMAccount)
	require.Regexp(t, `^[1-9][0-9]{15}$`, iamAccount)
	require.Regexp(t, `^[1-9][0-9]{15}$`, iam2Account)

	_, err = conn.ExecContext(ctx, `SELECT external_user_id FROM users`)
	require.Error(t, err, "external_user_id must be removed by the migration")
}

func TestCompanyManagedPolicySeedsAreExactAndIdempotent(t *testing.T) {
	ctx := context.Background()
	for policyKey, action := range map[string]string{
		"CompanyFinanceReadOnly":  "organization.finance.balance.read",
		"CompanySharedBalanceUse": "organization.balance.shared.use",
	} {
		var policyCount, actionCount int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM managed_policies WHERE policy_key=$1 AND policy_type='system' AND version=1`, policyKey).Scan(&policyCount))
		require.Equal(t, 1, policyCount)
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM managed_policy_actions a JOIN managed_policies p ON p.id=a.policy_id WHERE p.policy_key=$1 AND a.action=$2`, policyKey, action).Scan(&actionCount))
		require.Equal(t, 1, actionCount)
	}
	require.NoError(t, ApplyMigrations(ctx, integrationDB))
}
