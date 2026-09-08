package web3deposit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressStatusIsValid(t *testing.T) {
	t.Parallel()

	require.True(t, AddressStatusActive.IsValid())
	require.True(t, AddressStatusDisabled.IsValid())
	require.False(t, AddressStatus("").IsValid())
	require.False(t, AddressStatus("unknown").IsValid())
}
