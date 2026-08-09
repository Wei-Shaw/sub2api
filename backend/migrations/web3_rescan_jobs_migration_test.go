package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeb3RescanJobsMigrationDefinesRecoverableJobState(t *testing.T) {
	content, err := FS.ReadFile("204_web3_rescan_jobs.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS web3_rescan_jobs",
		"requested_by",
		"attempt_count",
		"lease_expires_at",
		"started_at",
		"completed_at",
		"event_count",
		"matched_count",
		"deposit_count",
		"error_message",
		"'pending', 'running', 'succeeded', 'failed'",
	} {
		require.Contains(t, sql, fragment)
	}
}
