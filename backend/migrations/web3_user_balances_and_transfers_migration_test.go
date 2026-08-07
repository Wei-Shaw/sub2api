package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration200CreatesWeb3BalancesAndTransfers(t *testing.T) {
	content, err := FS.ReadFile("200_web3_user_balances_and_transfers.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS web3_user_balances")
	require.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "user_id BIGINT NOT NULL REFERENCES users(id)")
	require.Contains(t, sql, "asset_key VARCHAR(64) NOT NULL")
	require.Contains(t, sql, "available_amount DECIMAL(20,8) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "UNIQUE (id, user_id)")
	require.Contains(t, sql, "UNIQUE (user_id, asset_key)")
	require.Contains(t, sql, "available_amount = total_deposited - total_transferred")

	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS web3_balance_transfers")
	require.Contains(t, sql, "web3_balance_id BIGINT NOT NULL")
	require.Contains(t, sql, "FOREIGN KEY (web3_balance_id, user_id)")
	require.Contains(t, sql, "REFERENCES web3_user_balances(id, user_id)")
	require.Contains(t, sql, "idempotency_key VARCHAR(180) NOT NULL UNIQUE")
	require.Contains(t, sql, "metadata JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "web3_balance_after = web3_balance_before - amount")
	require.Contains(t, sql, "user_balance_after = user_balance_before + amount")
	require.Contains(t, sql, "ON web3_balance_transfers (user_id, created_at DESC, id DESC)")
	require.Contains(t, sql, "ON web3_balance_transfers (web3_balance_id, created_at DESC, id DESC)")
	require.NotContains(t, sql, "FLOAT")
	require.NotContains(t, sql, "DOUBLE")
	require.NotContains(t, sql, "ON DELETE CASCADE")
}
