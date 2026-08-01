package migrations

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageBillingSettlementMigrationIsLocalOnly(t *testing.T) {
	_, err := UpstreamFS.ReadFile("183_usage_billing_settlements.sql")
	require.ErrorIs(t, err, fs.ErrNotExist)

	content, err := LocalFS.ReadFile("local/local_0022_usage_billing_settlements.sql")
	require.NoError(t, err)
	require.Contains(t, string(content), "CREATE TABLE IF NOT EXISTS usage_billing_settlements")
}
