package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyTransportOptionsMigration(t *testing.T) {
	content, err := FS.ReadFile("221_proxy_transport_options.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS force_http1 BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS disable_keep_alive BOOLEAN NOT NULL DEFAULT FALSE")
}
