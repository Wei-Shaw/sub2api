package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyConsoleURLMigration(t *testing.T) {
	content, err := FS.ReadFile("239_add_proxy_console_url.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE proxies ADD COLUMN IF NOT EXISTS console_url VARCHAR(2048)")
}
