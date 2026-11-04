package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  "at-secret",
			"refresh_token": "rt-secret",
			"id_token":      "id-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://api.example.com",
			"model_mapping": map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "id-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_RedactsOllamaCloudManagedExtra(t *testing.T) {
	snapshot := map[string]any{
		"status":          service.OllamaCloudUsageStatusOK,
		"last_attempt_at": "2026-07-22T12:00:00Z",
		"next_refresh_at": "2026-07-22T13:00:00Z",
		"data":            map[string]any{"plan": "Pro"},
	}
	src := &service.Account{
		ID: 9, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://ollama.com", "api_key": "secret-key"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "ciphertext-secret",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey:    snapshot,
			"ordinary":                                  "kept",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSnapshotExtraKey)
	require.Equal(t, "kept", got.Extra["ordinary"])
	require.NotNil(t, got.OllamaCloudUsage)
	require.True(t, got.OllamaCloudUsage.Configured)
	require.True(t, got.OllamaCloudUsage.AutoRefreshEnabled)
	require.Equal(t, "Pro", got.OllamaCloudUsage.Snapshot.Data.Plan)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "ciphertext-secret")
	require.NotContains(t, string(raw), "secret-key")
	require.Contains(t, src.Extra, service.OllamaCloudUsageSessionExtraKey)
}

func TestAccountFromServiceShallow_RedactsCodexTurnTicketState(t *testing.T) {
	blob := "gAAAAA" + strings.Repeat("B", 286)
	src := &service.Account{
		ID: 41, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Extra: map[string]any{
			"codex_harvest_proxy_url": "http://user:legacy-proxy-secret@proxy.example.com:8080",
			"codex_turn_ticket:gpt-6-astra": map[string]any{
				"state":       blob,
				"length":      292,
				"model":       "gpt-6-astra",
				"captured_at": time.Now().Add(-time.Minute),
				"expires_at":  time.Now().Add(time.Hour),
				"attempts":    3,
			},
		},
	}
	got := AccountFromServiceShallow(src)
	require.NotContains(t, got.Extra, "codex_turn_ticket:gpt-6-astra")
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), blob)
	require.NotContains(t, string(raw), "legacy-proxy-secret")
	require.NotContains(t, got.Extra, "codex_harvest_proxy_url")
	require.Contains(t, src.Extra, "codex_harvest_proxy_url")
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}

func TestAccountFromService_RedactsGrokModelDiagnostics(t *testing.T) {
	src := &service.Account{
		ID: 25, Name: "grok", Platform: service.PlatformGrok, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "access-secret",
			"refresh_token": "refresh-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://cli-chat-proxy.grok.com/v1",
			"model_mapping": map[string]any{"friendly": "grok-4.7"},
		},
		Extra: map[string]any{"grok_observed_models": map[string]any{"models": []any{"grok-4.7"}}},
	}

	got := AccountFromService(src)
	require.Nil(t, got.GrokModelDiagnostics)
	shallow := AccountFromServiceShallow(src)
	require.Nil(t, shallow.GrokModelDiagnostics)

	diagnostics := &service.GrokModelDiagnostics{
		State:    "fresh",
		Endpoint: "https://cli-chat-proxy.grok.com/v1/models",
		Models:   []string{"grok-4.7"},
		Mappings: []service.GrokModelDiagnosticItem{{Requested: "friendly", Upstream: "grok-4.7", Status: "listed"}},
		LastAttempt: &service.GrokCatalogAttempt{
			StartedAt: "2026-10-25T06:00:00Z", FinishedAt: "2026-10-25T06:00:01Z", Status: "failed", ErrorCode: "http_error",
			Scope: &service.GrokCatalogScope{AccountID: 25, AccountType: service.AccountTypeOAuth, Endpoint: "https://cli-chat-proxy.grok.com/v1/models"},
		},
	}
	diagRaw, err := json.Marshal(diagnostics)
	require.NoError(t, err)
	require.NotContains(t, string(diagRaw), "access-secret")
	require.NotContains(t, string(diagRaw), "query-secret")
	require.NotContains(t, string(diagRaw), "s3cret")
	require.NotContains(t, string(diagRaw), "user:")
	require.Contains(t, string(diagRaw), `"models":["grok-4.7"]`)
	require.NotContains(t, string(diagRaw), `"models":null`)
	require.NotContains(t, string(diagRaw), `"mappings":null`)
	got.GrokModelDiagnostics = diagnostics
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, `"grok_model_diagnostics"`)
	require.Contains(t, body, `"mappings":[`)
	require.NotContains(t, body, "access-secret")
	require.NotContains(t, body, "refresh-secret")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "query-secret")
	require.NotContains(t, body, "s3cret")

	listRaw, err := json.Marshal(AccountListItemFromAccount(got))
	require.NoError(t, err)
	require.NotContains(t, string(listRaw), "grok_model_diagnostics")
	require.NotContains(t, string(listRaw), "access-secret")
}
