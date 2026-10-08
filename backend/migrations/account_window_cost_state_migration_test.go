//go:build unit

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration232CreatesBoundedWindowCostState(t *testing.T) {
	content, err := FS.ReadFile("232_account_window_cost_state.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS account_window_cost_state")
	require.Contains(t, sql, "account_id BIGINT PRIMARY KEY")
	require.Contains(t, sql, "standard_cost DECIMAL(20, 10)")
	require.Contains(t, sql, "initialized BOOLEAN NOT NULL DEFAULT FALSE")
	require.NotContains(t, sql, "INSERT INTO account_window_cost_state SELECT")
}

func TestMigration232AggregatesInsertsAndInvalidatesMutations(t *testing.T) {
	content, err := FS.ReadFile("232_account_window_cost_state.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "REFERENCING NEW TABLE AS inserted_usage_logs")
	require.Contains(t, sql, "FOR EACH STATEMENT")
	require.Contains(t, sql, "SUM(total_cost) AS standard_cost")
	require.Contains(t, sql, "ORDER BY account_id")
	require.Contains(t, sql, "accounts.platform = 'anthropic'")
	require.Contains(t, sql, "accounts.type IN ('oauth', 'setup-token')")
	require.Contains(t, sql, "accounts.extra ? 'window_cost_limit'")
	require.Contains(t, sql, "ON CONFLICT (account_id) DO UPDATE")
	require.Contains(t, sql, "CREATE TRIGGER usage_logs_account_window_cost_delete")
	require.Contains(t, sql, "CREATE TRIGGER usage_logs_account_window_cost_update")
	require.Contains(t, sql, "CURRENT_TIMESTAMP - INTERVAL '6 hours'")
	require.Contains(t, sql, "CREATE TRIGGER accounts_window_cost_state_invalidate")
	require.Contains(t, sql, "AFTER UPDATE OF platform, type, session_window_start, session_window_end, extra ON accounts")
	require.Contains(t, sql, "SET initialized = FALSE")
}
