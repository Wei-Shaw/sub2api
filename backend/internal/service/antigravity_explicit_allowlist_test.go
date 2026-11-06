//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityExplicitAllowlistDoesNotExpand(t *testing.T) {
	account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{
		"model_mapping": map[string]any{"gemini-4-argon-high": "gemini-4-argon-high"},
	}}
	require.Equal(t, map[string]string{"gemini-4-argon-high": "gemini-4-argon-high"}, account.GetModelMapping())
	gateway := &GatewayService{}
	forwarder := &AntigravityGatewayService{}
	for _, model := range []string{
		"gemini-3.1-pro", "gemini-3.1-pro-high", "gemini-3.1-pro-preview",
		"gemini-3.8-flash", "gemini-3.8-flash-high", "gemini-3-flash", "claude-sonnet-4-5",
	} {
		t.Run(model, func(t *testing.T) {
			require.False(t, account.IsModelSupported(model))
			require.False(t, gateway.isModelSupportedByAccount(account, model))
			for _, level := range []string{"", "low", "medium", "high"} {
				require.Empty(t, forwarder.getMappedModelForThinkingLevel(account, model, level))
				require.Empty(t, forwarder.getMappedModelForThinkingLevel(account, "models/"+model, level))
			}
		})
	}
	require.True(t, gateway.isModelSupportedByAccount(account, "gemini-4-argon-high"))
	require.Equal(t, "gemini-4-argon-high", forwarder.getMappedModelForThinkingLevel(account, "gemini-4-argon", "low"))
}

func TestAntigravityExplicitAliasesAndWildcards(t *testing.T) {
	account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{
		"model_mapping": map[string]any{
			"my-model":         "gemini-4-argon-high",
			"gemini-4-*":       "gemini-4-argon-high",
			"gemini-pro-agent": "gemini-pro-agent",
		},
	}}
	require.True(t, account.IsModelSupported("my-model"))
	require.Equal(t, "gemini-4-argon-high", mapAntigravityModel(account, "my-model"))
	require.True(t, account.IsModelSupported("gemini-4-argon-low"))
	require.False(t, account.IsModelSupported("gemini-3.1-pro"), "compatibility aliases must be explicitly allowed")
	account.Credentials["model_mapping"] = map[string]any{"invalid": 123, "empty": ""}
	require.False(t, account.IsModelSupported("gemini-4-argon-high"))
	require.Empty(t, mapAntigravityModel(account, "gemini-4-argon-high"))
}
