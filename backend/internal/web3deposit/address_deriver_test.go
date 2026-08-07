package web3deposit

import (
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

const testEVMAccountXPub = "xpub6Ce9NcJvTk36xtLSrJLZqE7wtgA5deCeYs7rSQtreh4cj6ByPtrg9sD7V2FNFLPnf8heNP3FGkeV9qwfzvZNSd54JoNXVsXFYSYwHsnJxqP"

func TestDeriveEVMAddressVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		index      int64
		address    string
		normalized string
	}{
		{index: 0, address: "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", normalized: "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"},
		{index: 1, address: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8", normalized: "0x70997970c51812dc3a010c7d01b50e0d17dc79c8"},
		{index: 2, address: "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC", normalized: "0x3c44cdddb6a900fa2b585dd299e03d12fa4293bc"},
		{index: 1000, address: "0x6D58073AeeB28068c5925D618DA9f4c4F35727b3", normalized: "0x6d58073aeeb28068c5925d618da9f4c4f35727b3"},
	}
	for _, test := range tests {
		derived, err := DeriveEVMAddress(testEVMAccountXPub, test.index)
		require.NoError(t, err)
		require.Equal(t, test.index, derived.DerivationIndex)
		require.Equal(t, test.address, derived.Address)
		require.Equal(t, strings.ToLower(test.address), derived.NormalizedAddress)
		require.Equal(t, test.normalized, derived.NormalizedAddress)
	}
}

func TestDeriveEVMAddressTrimsAccountXPub(t *testing.T) {
	t.Parallel()

	derived, err := DeriveEVMAddress(" \n"+testEVMAccountXPub+"\t", 0)
	require.NoError(t, err)
	require.Equal(t, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", derived.Address)
}

func TestDeriveEVMAddressRejectsInvalidIndexes(t *testing.T) {
	t.Parallel()

	for _, index := range []int64{-1, MaxDerivationIndexExclusive} {
		_, err := DeriveEVMAddress(testEVMAccountXPub, index)
		require.ErrorIs(t, err, ErrDerivationIndexOutOfRange)
	}
}

func TestDeriveEVMAddressRejectsInvalidAccountKeysWithoutLeakingThem(t *testing.T) {
	t.Parallel()

	privateKey, err := hdkeychain.NewMaster(make([]byte, 32), &chaincfg.MainNetParams)
	require.NoError(t, err)
	privateKeyText := privateKey.String()
	privateKey.Zero()

	accountKey, err := hdkeychain.NewKeyFromString(testEVMAccountXPub)
	require.NoError(t, err)
	externalKey, err := accountKey.Derive(0)
	require.NoError(t, err)
	externalKeyText := externalKey.String()
	externalKey.Zero()
	accountKey.Zero()

	for _, invalidKey := range []string{"not-an-xpub", privateKeyText, externalKeyText} {
		_, err := DeriveEVMAddress(invalidKey, 0)
		require.True(t, errors.Is(err, ErrAccountXPubInvalid))
		require.NotContains(t, err.Error(), invalidKey)
	}
}
