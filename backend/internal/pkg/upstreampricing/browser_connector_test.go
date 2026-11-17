package upstreampricing

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserConnectorScope(t *testing.T) {
	script, err := BrowserConnectorScript("https://station.example", "http://127.0.0.1:8080")
	require.NoError(t, err)
	require.Contains(t, script, "// @match        https://station.example/*")
	require.Contains(t, script, "// @connect      127.0.0.1")
	require.NotContains(t, script, "__CONNECTOR_CONFIG__")
	require.NotContains(t, script, "GM_cookie")
	require.NotContains(t, script, "@connect      *")
	require.False(t, strings.Contains(script, "console.log"))
	for _, origin := range []string{"https://remote.example", "http://user@localhost:8080", "http://localhost:8080/?token=x"} {
		_, err := BrowserConnectorScript("https://station.example", origin)
		require.Error(t, err)
	}
	_, err = BrowserConnectorScript("https://station.example\n// @match *://*/*", "http://localhost:8080")
	require.Error(t, err)
}
