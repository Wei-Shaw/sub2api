package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeb3UserBalanceSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3AssetKey("usdt"))
	require.NoError(t, validateWeb3AssetKey("usdc_ethereum"))
	require.Error(t, validateWeb3AssetKey("USDT0"))
	require.Error(t, validateWeb3AssetKey("usdt-0"))
	require.Error(t, validateWeb3AssetKey(""))

	require.NoError(t, validateWeb3BalanceAmount("0"))
	require.NoError(t, validateWeb3BalanceAmount("999999999999.99999999"))
	require.Error(t, validateWeb3BalanceAmount("-1"))
	require.Error(t, validateWeb3BalanceAmount("1000000000000.00000000"))
}
