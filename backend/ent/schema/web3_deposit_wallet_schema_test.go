package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositWalletSchema(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, loadedSchema := range spec.Schemas {
		schemas[loadedSchema.Name] = loadedSchema
	}

	wallet := requireSchema(t, schemas, "Web3DepositWallet")
	requireSchemaFields(t, wallet,
		"id",
		"wallet_id",
		"account_path",
		"xpub_fingerprint",
		"next_derivation_index",
		"status",
	)

	id := requireSchemaField(t, wallet, "id")
	require.Equal(t, field.TypeInt64, id.Info.Type)
	require.True(t, id.Immutable)

	walletID := requireSchemaField(t, wallet, "wallet_id")
	require.Equal(t, field.TypeString, walletID.Info.Type)
	require.True(t, walletID.Unique)
	require.True(t, walletID.Immutable)

	nextIndex := requireSchemaField(t, wallet, "next_derivation_index")
	require.True(t, nextIndex.Default)
	require.EqualValues(t, 0, nextIndex.DefaultValue)
	require.Equal(t, 1, nextIndex.Validators)

	status := requireSchemaField(t, wallet, "status")
	require.True(t, status.Default)
	require.Equal(t, "active", status.DefaultValue)
	require.GreaterOrEqual(t, status.Validators, 1)
}

func TestWeb3DepositWalletSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3DepositWalletID("evm_deposit_v1"))
	require.Error(t, validateWeb3DepositWalletID("Invalid-Wallet"))

	require.NoError(t, validateWeb3DepositWalletAccountPath("m/44'/60'/0'"))
	require.Error(t, validateWeb3DepositWalletAccountPath("m/44'/0'/0'"))

	require.NoError(t, validateWeb3DepositWalletFingerprint("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	require.Error(t, validateWeb3DepositWalletFingerprint("not-a-fingerprint"))

	require.NoError(t, validateWeb3DepositDerivationIndex(0))
	require.NoError(t, validateWeb3DepositDerivationIndex(maxWeb3DepositDerivationIndex-1))
	require.Error(t, validateWeb3DepositDerivationIndex(-1))
	require.Error(t, validateWeb3DepositDerivationIndex(maxWeb3DepositDerivationIndex))

	require.NoError(t, validateWeb3DepositWalletStatus("active"))
	require.NoError(t, validateWeb3DepositWalletStatus("disabled"))
	require.Error(t, validateWeb3DepositWalletStatus("unknown"))
}
