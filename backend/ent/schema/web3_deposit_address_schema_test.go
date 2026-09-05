package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositAddressSchema(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, loadedSchema := range spec.Schemas {
		schemas[loadedSchema.Name] = loadedSchema
	}

	address := requireSchema(t, schemas, "Web3DepositAddress")
	requireSchemaFields(t, address,
		"id",
		"user_id",
		"wallet_id",
		"derivation_index",
		"address",
		"normalized_address",
		"status",
		"allocated_at",
		"disabled_at",
		"last_deposit_at",
	)

	id := requireSchemaField(t, address, "id")
	require.Equal(t, field.TypeInt64, id.Info.Type)
	require.True(t, id.Immutable)

	normalizedAddress := requireSchemaField(t, address, "normalized_address")
	require.Equal(t, field.TypeString, normalizedAddress.Info.Type)
	require.True(t, normalizedAddress.Immutable)

	status := requireSchemaField(t, address, "status")
	require.True(t, status.Default)
	require.Equal(t, "active", status.DefaultValue)

	require.Len(t, address.Indexes, 4)
}

func TestWeb3DepositAddressSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3DepositAddress("0x1234567890abcdef1234567890ABCDEF12345678"))
	require.Error(t, validateWeb3DepositAddress("0x1234"))

	require.NoError(t, validateWeb3DepositNormalizedAddress("0x1234567890abcdef1234567890abcdef12345678"))
	require.Error(t, validateWeb3DepositNormalizedAddress("0x1234567890abcdef1234567890ABCDEF12345678"))

	require.NoError(t, validateWeb3DepositAddressStatus("active"))
	require.NoError(t, validateWeb3DepositAddressStatus("disabled"))
	require.Error(t, validateWeb3DepositAddressStatus("unknown"))
}
