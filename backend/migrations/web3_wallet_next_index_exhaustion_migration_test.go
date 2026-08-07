package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration201AllowsExhaustedWalletNextIndex(t *testing.T) {
	content, err := FS.ReadFile("201_web3_wallet_next_index_exhaustion.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS web3_deposit_wallets_next_index_check")
	require.Contains(t, sql, "next_derivation_index >= 0 AND next_derivation_index <= 2147483648")
	require.NotContains(t, sql, "next_derivation_index < 2147483648")
}
