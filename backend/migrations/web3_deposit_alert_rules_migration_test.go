package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWeb3DepositAlertRulesMigrationSeedsDefaultRules(t *testing.T) {
	content, err := FS.ReadFile("203_web3_deposit_alert_rules.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "ON CONFLICT (name) DO NOTHING")
	for _, metricType := range []string{
		"web3_rpc_unhealthy",
		"web3_scanner_lag_blocks",
		"web3_finalizer_lag_blocks",
		"web3_manual_review_count",
		"web3_credit_failures_total",
	} {
		require.Contains(t, sql, "'"+metricType+"'")
	}
	require.GreaterOrEqual(t, strings.Count(sql, "INSERT INTO ops_alert_rules"), 5)
}
