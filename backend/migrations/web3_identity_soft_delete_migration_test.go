package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeb3IdentitySoftDeleteMigration(t *testing.T) {
	content, err := FS.ReadFile("195_web3_identity_soft_delete.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ")
	require.Contains(t, sql, "SET deleted_at = u.deleted_at")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS web3_identities_address_key")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS web3_identities_active_address_key")
	require.Contains(t, sql, "WHERE deleted_at IS NULL")
}
