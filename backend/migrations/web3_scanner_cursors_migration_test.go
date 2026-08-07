package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration199CreatesWeb3ScannerCursorsAndLeases(t *testing.T) {
	content, err := FS.ReadFile("199_web3_scanner_cursors.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "id BIGSERIAL PRIMARY KEY")
	require.Contains(t, sql, "scanner_key VARCHAR(128) NOT NULL UNIQUE")
	require.Contains(t, sql, "UNIQUE (chain_id, token_contract)")
	require.Contains(t, sql, "last_scanned_block >= scan_start_block")
	require.Contains(t, sql, "last_finalized_block >= scan_start_block")
	require.Contains(t, sql, "last_finalized_block <= last_scanned_block")
	require.Contains(t, sql, "lease_owner IS NULL")
	require.Contains(t, sql, "lease_token IS NOT NULL")
	require.Contains(t, sql, "ON web3_scanner_cursors (lease_expires_at)")
}
