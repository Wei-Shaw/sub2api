package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestWeb3DepositSchema(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, loadedSchema := range spec.Schemas {
		schemas[loadedSchema.Name] = loadedSchema
	}

	deposit := requireSchema(t, schemas, "Web3Deposit")
	requireSchemaFields(t, deposit,
		"id",
		"user_id",
		"deposit_address_id",
		"chain_id",
		"token_contract",
		"tx_hash",
		"log_index",
		"block_number",
		"block_hash",
		"from_address",
		"to_address",
		"raw_amount",
		"token_decimals",
		"token_amount",
		"credited_amount",
		"status",
		"retry_count",
		"next_retry_at",
		"detected_at",
		"finalized_at",
		"credited_at",
	)

	id := requireSchemaField(t, deposit, "id")
	require.Equal(t, field.TypeInt64, id.Info.Type)
	require.True(t, id.Immutable)

	status := requireSchemaField(t, deposit, "status")
	require.True(t, status.Default)
	require.Equal(t, "detected", status.DefaultValue)

	require.Len(t, deposit.Indexes, 5)
}

func TestWeb3DepositSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3DepositHash("0x"+stringOf('a', 64)))
	require.Error(t, validateWeb3DepositHash("0x"+stringOf('A', 64)))
	require.NoError(t, validateWeb3DepositCanonicalAddress("0x"+stringOf('a', 40)))
	require.Error(t, validateWeb3DepositCanonicalAddress("0x"+stringOf('A', 40)))

	require.NoError(t, validateWeb3DepositRawAmount("1"))
	require.NoError(t, validateWeb3DepositRawAmount(stringOf('9', 78)))
	require.Error(t, validateWeb3DepositRawAmount("0"))
	require.Error(t, validateWeb3DepositRawAmount(stringOf('9', 79)))

	require.NoError(t, validateWeb3DepositDecimal("1.000001", 38, 18, "token amount"))
	require.NoError(t, validateWeb3DepositDecimal("999999999999.99999999", 20, 8, "credited amount"))
	require.Error(t, validateWeb3DepositDecimal("0", 38, 18, "token amount"))
	require.Error(t, validateWeb3DepositDecimal("1000000000000.00000000", 20, 8, "credited amount"))

	require.NoError(t, validateWeb3DepositTokenDecimals(6))
	require.Error(t, validateWeb3DepositTokenDecimals(-1))
	require.NoError(t, validateWeb3DepositStatus("detected"))
	require.Error(t, validateWeb3DepositStatus("unknown"))
}

func stringOf(character rune, count int) string {
	value := make([]rune, count)
	for index := range value {
		value[index] = character
	}
	return string(value)
}
