package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration198CreatesWeb3Deposits(t *testing.T) {
	content, err := FS.ReadFile("198_web3_deposits.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "user_id BIGINT NOT NULL REFERENCES users(id)")
	require.Contains(t, sql, "deposit_address_id BIGINT NOT NULL REFERENCES web3_deposit_addresses(id)")
	require.Contains(t, sql, "raw_amount NUMERIC(78,0) NOT NULL")
	require.Contains(t, sql, "token_amount NUMERIC(38,18) NOT NULL")
	require.Contains(t, sql, "credited_amount DECIMAL(20,8)")
	require.Contains(t, sql, "tx_hash ~ '^0x[0-9a-f]{64}$'")
	require.Contains(t, sql, "to_address ~ '^0x[0-9a-f]{40}$'")
	require.Contains(t, sql, "UNIQUE (chain_id, tx_hash, log_index)")
	require.Contains(t, sql, "ON web3_deposits (status, next_retry_at, id)")
	require.Contains(t, sql, "ON web3_deposits (user_id, created_at DESC, id DESC)")
	require.Contains(t, sql, "ON web3_deposits (block_number, id)")
	require.Contains(t, sql, "ON web3_deposits (deposit_address_id, created_at DESC)")
	require.Contains(t, sql, "ON web3_deposits (LOWER(tx_hash))")
	require.NotContains(t, sql, "FLOAT")
	require.NotContains(t, sql, "DOUBLE")
	require.NotContains(t, sql, "ON DELETE CASCADE")
}
