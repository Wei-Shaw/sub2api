package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration196CreatesChainAgnosticWeb3DepositWallets(t *testing.T) {
	content, err := FS.ReadFile("196_web3_deposit_wallets.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS web3_deposit_wallets")
	require.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "wallet_id VARCHAR(64) NOT NULL UNIQUE")
	require.Contains(t, sql, "account_path VARCHAR(64) NOT NULL")
	require.Contains(t, sql, "xpub_fingerprint VARCHAR(64) NOT NULL")
	require.Contains(t, sql, "next_derivation_index BIGINT NOT NULL DEFAULT 0")
	require.Contains(t, sql, "next_derivation_index >= 0 AND next_derivation_index < 2147483648")
	require.Contains(t, sql, "status IN ('active', 'disabled')")
	require.NotContains(t, sql, "chain_id")
	require.NotContains(t, sql, "account_xpub")
}
