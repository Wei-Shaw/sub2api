package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration195RevertsOnlyRowsWrittenBeforeMigration194(t *testing.T) {
	content, err := FS.ReadFile("195_revert_openai_sol_hidden_billing_backfill.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "filename = '194_openai_sol_hidden_billing_backfill.sql'")
	require.Contains(t, sql, "ul.created_at < cutoff.applied_at")
	require.Contains(t, sql, "ul.total_cost / 2.0")
	require.Contains(t, sql, "ul.actual_cost >= ul.total_cost * 0.999999")
	require.Contains(t, sql, "d.bucket_date = cutoff.day_start::date")
}
