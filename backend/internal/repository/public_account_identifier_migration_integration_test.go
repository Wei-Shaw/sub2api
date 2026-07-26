//go:build integration

package repository

import (
	"context"
	"os"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestPublicAccountIdentifierMigrationBothPhases(t *testing.T) {
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

	var legacyID int64
	require.NoError(t, conn.QueryRowContext(ctx, `INSERT INTO users(email) VALUES('legacy@example.com') RETURNING id`).Scan(&legacyID))
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type) VALUES('invalid@example.com','0123456789012345','0123456789012345','root')`)
	require.Error(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type) VALUES('root@example.com','1719905235756637','1719905235756637','root')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type) VALUES('duplicate@example.com','1719905235756637','1719905235756637','root')`)
	require.Error(t, err)

	finalize, err := os.ReadFile("../../tools/finalize_public_account_identifiers.sql")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(finalize))
	require.Error(t, err, "phase two must refuse an unbackfilled legacy row")

	_, err = conn.ExecContext(ctx, `UPDATE users SET account_id='2719905235756637',external_user_id='2719905235756637',identity_type='root' WHERE id=$1`, legacyID)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(finalize))
	require.NoError(t, err)

	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,identity_type) VALUES('missing@example.com','root')`)
	require.Error(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type) VALUES('mismatch@example.com','3719905235756637','4719905235756637','root')`)
	require.Error(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type,login_name) VALUES('iam@example.com','1719905235756637','201705485041478971','iam','finance.reader')`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO users(email,account_id,external_user_id,identity_type,login_name,deleted_at) VALUES('reused@example.com','1719905235756637','201705485041478971','iam','archived',NOW())`)
	require.Error(t, err, "soft-deleted public IDs remain globally reserved")
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
