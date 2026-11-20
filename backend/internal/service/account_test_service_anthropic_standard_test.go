package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestClaudeAccountTestAPIKeyUsesStandardAPI(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://api.anthropic.com"}}
	payload, err := createClaudeAccountTestPayload(account, "claude-fable-5-1")
	require.NoError(t, err)
	require.Equal(t, "claude-fable-5-1", payload["model"])
	require.Equal(t, true, payload["stream"])
	require.NotContains(t, payload, "system")
	require.NotContains(t, payload, "metadata")
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "Claude Code")
	require.NotContains(t, string(encoded), "cache_control")

	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	require.NoError(t, err)
	applyClaudeAccountTestHeaders(req, account, "test-api-key")
	require.Equal(t, "test-api-key", req.Header.Get("x-api-key"))
	require.Empty(t, req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("anthropic-beta"))
	require.Empty(t, req.Header.Get("X-App"))
	require.NotContains(t, req.Header.Get("User-Agent"), "claude-cli")
}

func TestClaudeAccountTestOAuthPreservesClaudeCode(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	payload, err := createClaudeAccountTestPayload(account, "claude-fable-5-1")
	require.NoError(t, err)
	require.Contains(t, payload, "metadata")
	require.Contains(t, payload, "system")
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(encoded), claudeCodeSystemPrompt)

	req, err := http.NewRequest(http.MethodPost, testClaudeAPIURL, nil)
	require.NoError(t, err)
	applyClaudeAccountTestHeaders(req, account, "test-oauth-token")
	require.Equal(t, "Bearer test-oauth-token", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("x-api-key"))
	require.Equal(t, claude.DefaultBetaHeader, req.Header.Get("anthropic-beta"))
	for key, value := range claude.DefaultHeaders() {
		require.Equal(t, value, req.Header.Get(key))
	}
}

func TestClaudeAccountTestAPIKeyPreservesHeaderOverrides(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"header_override_enabled": true,
		"header_overrides":        map[string]any{"user-agent": "standard-api-test", "anthropic-beta": claude.BetaInterleavedThinking},
	}}
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	require.NoError(t, err)
	applyClaudeAccountTestHeaders(req, account, "test-api-key")
	require.Equal(t, "standard-api-test", req.Header.Get("User-Agent"))
	require.Equal(t, claude.BetaInterleavedThinking, getHeaderRaw(req.Header, "anthropic-beta"))
	require.False(t, strings.Contains(getHeaderRaw(req.Header, "anthropic-beta"), claude.BetaClaudeCode))
}
