package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeb3BalanceTransferSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3DepositDecimal("1.00000000", 20, 8, "balance transfer amount"))
	require.Error(t, validateWeb3DepositDecimal("0", 20, 8, "balance transfer amount"))
	require.NoError(t, validateWeb3TransferSnapshotAmount("0"))
	require.NoError(t, validateWeb3TransferSnapshotAmount("1.00000000"))
	require.Error(t, validateWeb3TransferSnapshotAmount("-1"))
	require.NoError(t, validateWeb3UserBalanceSnapshotAmount("-1.00000000"))
	require.NoError(t, validateWeb3UserBalanceSnapshotAmount("0"))
	require.Error(t, validateWeb3UserBalanceSnapshotAmount("-1000000000000.00000000"))
}
