package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageLogProxyIDMigration(t *testing.T) {
	content, err := FS.ReadFile("177_add_usage_log_proxy_id.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS proxy_id BIGINT")
	require.NotContains(t, sql, "REFERENCES proxies")
}
