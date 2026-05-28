package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_IsCacheTTLOverrideEnabled_BedrockHonoursExtraFlag(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
			"cache_ttl_override_target":  "1h",
		},
	}
	require.True(t, a.IsCacheTTLOverrideEnabled(),
		"after D2 widening, Bedrock accounts with the Extra flag set must take effect; "+
			"the previous gate would have silently dropped this configuration")
	require.Equal(t, "1h", a.GetCacheTTLOverrideTarget())
}

func TestAccount_IsCacheTTLOverrideEnabled_VertexHonoursExtraFlag(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
		},
	}
	require.True(t, a.IsCacheTTLOverrideEnabled())
}

func TestAccount_IsCacheTTLOverrideEnabled_AnthropicAPIKeyHonoursExtraFlag(t *testing.T) {
	a := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
		},
	}
	require.True(t, a.IsCacheTTLOverrideEnabled())
}

func TestAccount_IsCacheTTLOverrideEnabled_StillFalseForNonAnthropic(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"cache_ttl_override_enabled": true,
		},
	}
	require.False(t, a.IsCacheTTLOverrideEnabled(),
		"OpenAI should never expose Anthropic cache TTL override semantics")
}

func TestAccount_IsCacheTTLOverrideEnabled_DefaultsFalseWithoutFlag(t *testing.T) {
	a := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	require.False(t, a.IsCacheTTLOverrideEnabled())
}
