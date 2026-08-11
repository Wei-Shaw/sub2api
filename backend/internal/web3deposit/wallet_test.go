package web3deposit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalletStatusIsValid(t *testing.T) {
	t.Parallel()

	require.True(t, WalletStatusActive.IsValid())
	require.True(t, WalletStatusDisabled.IsValid())
	require.False(t, WalletStatus("").IsValid())
	require.False(t, WalletStatus("unknown").IsValid())
}
