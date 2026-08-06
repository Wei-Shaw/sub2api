package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration194ScopesSolBackfillToHiddenOneXFingerprint(t *testing.T) {
	content, err := FS.ReadFile("194_openai_sol_hidden_billing_backfill.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ul.rate_multiplier = 1.0")
	require.Contains(t, sql, "ul.actual_cost >= ul.total_cost * 1.999999")
	require.Contains(t, sql, "ul.actual_cost <= ul.total_cost * 2.000001")
	require.NotContains(t, sql, "ul.actual_cost >= ul.total_cost * 2.0;")
}
