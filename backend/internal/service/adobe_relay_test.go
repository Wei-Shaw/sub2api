//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestIsAdobeRelayAccount(t *testing.T) {
	require.False(t, isAdobeRelayAccount(nil))
	require.False(t, isAdobeRelayAccount(&Account{Platform: PlatformAdobe, Type: AccountTypeOAuth}))
	require.False(t, isAdobeRelayAccount(&Account{
		Platform: PlatformAdobe, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
	}))
	require.False(t, isAdobeRelayAccount(&Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://relay.example"},
	}))
	require.True(t, isAdobeRelayAccount(&Account{
		Platform: PlatformAdobe, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": " https://relay.example/v1 "},
	}))
}

func TestShouldSkipAdobeNativeAccount(t *testing.T) {
	oauth := &Account{Platform: PlatformAdobe, Type: AccountTypeOAuth}
	relay := &Account{
		Platform: PlatformAdobe, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://relay.example"},
	}
	require.False(t, shouldSkipAdobeNativeAccount(false, oauth))
	require.True(t, shouldSkipAdobeNativeAccount(true, oauth))
	require.False(t, shouldSkipAdobeNativeAccount(true, relay))
}

func TestAdobeRelayDefaultMappingIsIdentity(t *testing.T) {
	relay := &Account{
		Platform: PlatformAdobe,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example",
		},
	}
	oauth := &Account{Platform: PlatformAdobe}

	require.Equal(t, "gpt-image-2", relay.GetMappedModel("gpt-image-2"))
	require.Equal(t, "firefly-gpt-image-2", oauth.GetMappedModel("gpt-image-2"))
	require.True(t, relay.IsModelSupported("gpt-image-2"))
	require.True(t, relay.IsModelSupported("nano-banana"))
	require.False(t, relay.IsModelSupported("claude-sonnet-4-6"))

	for requested := range domain.DefaultAdobeModelMapping {
		require.Equal(t, requested, relay.GetMappedModel(requested), requested)
	}
}

func TestAdobeRelayGetOpenAIBaseURL(t *testing.T) {
	relay := &Account{
		Platform: PlatformAdobe,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example/v1/",
		},
	}
	require.Equal(t, "https://relay.example/v1", relay.GetOpenAIBaseURL())

	oauth := &Account{Platform: PlatformAdobe, Type: AccountTypeOAuth}
	require.Empty(t, oauth.GetOpenAIBaseURL())
}

func TestAdobeRelayExplicitMappingOverridesIdentity(t *testing.T) {
	relay := &Account{
		Platform: PlatformAdobe,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example",
			"model_mapping": map[string]any{
				"nano-banana": "gpt-image-2",
			},
		},
	}
	require.Equal(t, "gpt-image-2", relay.GetMappedModel("nano-banana"))
	require.False(t, relay.IsModelSupported("gpt-image-2"),
		"显式映射是严格白名单，没列出的对外名不应再靠默认恒等表放行")
}
