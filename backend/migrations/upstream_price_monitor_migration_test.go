package migrations

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamPriceMonitorMigrationDefaultsToObserveOnlyAndPersistsLedgerDate(t *testing.T) {
	raw, err := os.ReadFile("235_upstream_price_monitor.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))
	require.Contains(t, sql, "enabled boolean not null default false")
	require.Contains(t, sql, "mode varchar(24) not null default 'observe'")
	require.Contains(t, sql, "ledger_date date not null")
	require.Contains(t, sql, "primary key (account_id, model_name)")
	require.Contains(t, sql, "remote_snapshot jsonb")
	require.Contains(t, sql, "active_probe_pending boolean not null default false")
	require.Contains(t, sql, "idx_upstream_price_monitor_observations")
	require.Contains(t, sql, "idx_upstream_price_monitor_coverage")
	require.Contains(t, sql, "create table if not exists upstream_price_monitor_models")
	require.Contains(t, sql, "suspected_retired")
	require.NotContains(t, sql, "api_key")
	require.NotContains(t, sql, "refresh_token")
}

func TestUpstreamPriceMonitorUsageIndexIsConcurrent(t *testing.T) {
	raw, err := os.ReadFile("236_upstream_price_monitor_usage_window_index_notx.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))
	require.Contains(t, sql, "create index concurrently if not exists")
	require.Contains(t, sql, "on usage_logs (account_id, id)")
}
