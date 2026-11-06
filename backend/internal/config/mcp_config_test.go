package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPConfigValidatePublicURL(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
	}{
		{"unset", "", ""},
		{"blank", "  ", ""},
		{"https", "https://api.example.com/api/v1/mcp", "https://api.example.com/api/v1/mcp"},
		{"normalize", " HTTPS://API.Example.COM:8443/api/v1/mcp ", "https://api.example.com:8443/api/v1/mcp"},
		{"localhost", "http://localhost:8080/api/v1/mcp", "http://localhost:8080/api/v1/mcp"},
		{"loopback IPv4", "http://127.0.0.2:8080/api/v1/mcp", "http://127.0.0.2:8080/api/v1/mcp"},
		{"loopback IPv6", "http://[::1]:8080/api/v1/mcp", "http://[::1]:8080/api/v1/mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := MCPConfig{PublicURL: tc.value}
			require.NoError(t, cfg.Validate())
			require.Equal(t, tc.want, cfg.PublicURL)
		})
	}
	for _, value := range []string{
		"/api/v1/mcp", "//example.com/api/v1/mcp", "https:///api/v1/mcp",
		"https://:443/api/v1/mcp", "https://*.example.com/api/v1/mcp",
		"http://example.com/api/v1/mcp", "http://localhost.example.com/api/v1/mcp",
		"http://10.0.0.1/api/v1/mcp", "http://[::2]/api/v1/mcp",
		"ftp://localhost/api/v1/mcp", "https://example.com/api/v1/mcp/",
		"https://example.com/mcp", "https://example.com/api/v1/%6dcp",
		"https://example.com/api/v1/mcp?", "https://example.com/api/v1/mcp?x=1",
		"https://example.com/api/v1/mcp#", "https://example.com/api/v1/mcp#fragment",
		"https://user:password@example.com/api/v1/mcp", "https://user@example.com/api/v1/mcp",
		"https://example.com:0/api/v1/mcp", "https://example.com:65536/api/v1/mcp",
		"https://example.com:invalid/api/v1/mcp", "http://[::1%25eth0]/api/v1/mcp",
	} {
		t.Run(value, func(t *testing.T) {
			cfg := MCPConfig{PublicURL: value}
			require.ErrorContains(t, cfg.Validate(), "mcp.public_url")
		})
	}
}

func TestLoadMCPPublicURLFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("MCP_PUBLIC_URL", "HTTPS://MCP.Example.COM/api/v1/mcp")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://mcp.example.com/api/v1/mcp", cfg.MCP.PublicURL)

	cfg.MCP.PublicURL = "http://example.com/api/v1/mcp"
	require.ErrorContains(t, cfg.Validate(), "mcp.public_url must use HTTPS")
}

func TestLoadMCPPublicURLDefault(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("MCP_PUBLIC_URL", "")
	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.MCP.PublicURL)
}
