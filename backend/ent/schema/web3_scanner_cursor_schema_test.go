package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestWeb3ScannerCursorSchema(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	schemas := map[string]*load.Schema{}
	for _, loadedSchema := range spec.Schemas {
		schemas[loadedSchema.Name] = loadedSchema
	}

	cursor := requireSchema(t, schemas, "Web3ScannerCursor")
	requireSchemaFields(t, cursor,
		"id",
		"scanner_key",
		"chain_id",
		"token_contract",
		"scan_start_block",
		"last_scanned_block",
		"last_finalized_block",
		"lease_owner",
		"lease_token",
		"lease_expires_at",
		"last_error",
		"last_success_at",
	)

	id := requireSchemaField(t, cursor, "id")
	require.Equal(t, field.TypeInt64, id.Info.Type)
	require.True(t, id.Immutable)

	scannerKey := requireSchemaField(t, cursor, "scanner_key")
	require.True(t, scannerKey.Unique)
	require.True(t, scannerKey.Immutable)

	scanStart := requireSchemaField(t, cursor, "scan_start_block")
	require.True(t, scanStart.Immutable)
	require.Len(t, cursor.Indexes, 2)
}

func TestWeb3ScannerCursorSchemaValidators(t *testing.T) {
	require.NoError(t, validateWeb3ScannerKey("conflux_espace_mainnet:usdt0"))
	require.Error(t, validateWeb3ScannerKey("Invalid Scanner"))
	require.NoError(t, validateWeb3LeaseValue("scanner-01:01JABCDEF"))
	require.Error(t, validateWeb3LeaseValue("invalid lease value"))
}
