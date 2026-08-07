package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration197CreatesChainAgnosticWeb3DepositAddresses(t *testing.T) {
	content, err := FS.ReadFile("197_web3_deposit_addresses.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "user_id BIGINT NOT NULL REFERENCES users(id)")
	require.Contains(t, sql, "wallet_id VARCHAR(64) NOT NULL REFERENCES web3_deposit_wallets(wallet_id)")
	require.Contains(t, sql, "UNIQUE (user_id, wallet_id)")
	require.Contains(t, sql, "UNIQUE (wallet_id, derivation_index)")
	require.Contains(t, sql, "UNIQUE (normalized_address)")
	require.Contains(t, sql, "derivation_index >= 0 AND derivation_index < 2147483648")
	require.Contains(t, sql, "ON web3_deposit_addresses (user_id, created_at DESC)")
	require.NotContains(t, sql, "chain_id")
	require.NotContains(t, sql, "ON DELETE CASCADE")
}
