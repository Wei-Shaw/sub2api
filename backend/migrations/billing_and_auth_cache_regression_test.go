package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogBillingStatusMigration(t *testing.T) {
	content, err := FS.ReadFile("194_usage_log_billing_status.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "billing_status VARCHAR(16) NOT NULL DEFAULT 'settled'")
	require.Contains(t, sql, "CHECK (billing_status IN ('settled', 'unsettled'))")
}

func TestGroupAuthCacheInvalidatesEveryUpdate(t *testing.T) {
	content, err := FS.ReadFile("195_group_auth_cache_all_updates.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()")
	require.Contains(t, sql, "INSERT INTO auth_cache_invalidation_outbox")
	require.NotContains(t, sql, "IF TG_OP = 'UPDATE'")
}
