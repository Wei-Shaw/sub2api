//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildSchedulerMetadataAccount_KeepsOpenAIWSFlags(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
			"openai_ws_force_http":                         true,
			"mixed_scheduling":                             true,
			"unused_large_field":                           "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, true, got.Extra["openai_oauth_responses_websockets_v2_enabled"])
	require.Equal(t, service.OpenAIWSIngressModePassthrough, got.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, true, got.Extra["openai_ws_force_http"])
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestBuildSchedulerMetadataAccount_KeepsOpenAIReserveFields(t *testing.T) {
	account := service.Account{
		ID:       43,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"plan_type":     "free",
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
		},
		Extra: map[string]any{
			"codex_7d_used_percent":      12.0,
			"codex_primary_used_percent": 34.0,
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, "free", got.Credentials["plan_type"])
	require.Equal(t, map[string]any{"gpt-5.4": "gpt-5.4"}, got.Credentials["model_mapping"])
	require.Equal(t, 12.0, got.Extra["codex_7d_used_percent"])
	require.Equal(t, 34.0, got.Extra["codex_primary_used_percent"])
}
