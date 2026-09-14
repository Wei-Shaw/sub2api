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

func TestAdobeMaskRelaySelectionExclusions(t *testing.T) {
	failed := map[int64]struct{}{11: {}}
	natives := map[int64]struct{}{21: {}, 22: {}}

	relayOnly := AdobeMaskRelaySelectionExclusions(failed, true, natives)
	require.Equal(t, map[int64]struct{}{11: {}, 21: {}, 22: {}}, relayOnly)
	require.Equal(t, map[int64]struct{}{11: {}}, failed, "failed 不得被就地改写")

	afterFallback := AdobeMaskRelaySelectionExclusions(failed, false, natives)
	require.Equal(t, map[int64]struct{}{11: {}}, afterFallback)

	empty := AdobeMaskRelaySelectionExclusions(nil, true, natives)
	require.Equal(t, natives, empty)
	natives[99] = struct{}{}
	require.NotContains(t, empty, int64(99), "应返回拷贝而不是共用 map")
}

func TestAdobePrefersMaskRelay(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		mask   bool
		want   bool
	}{
		{name: "gpt-image-2 with mask", model: "gpt-image-2", mask: true, want: true},
		{name: "firefly-gpt-image-2 with mask", model: "firefly-gpt-image-2", mask: true, want: true},
		{name: "gpt-image-2.5-flare with mask", model: "gpt-image-2.5-flare", mask: true, want: true},
		{name: "gpt-image-2 without mask", model: "gpt-image-2", mask: false, want: false},
		{name: "nano-banana2 with mask", model: "nano-banana2", mask: true, want: false},
		{name: "firefly-nano-banana2 with mask", model: "firefly-nano-banana2", mask: true, want: false},
		{name: "flux-pro with mask", model: "flux-pro", mask: true, want: false},
		{name: "gpt-4o-image with mask", model: "gpt-4o-image", mask: true, want: false},
		{name: "nil request", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *OpenAIImagesRequest
			if tt.model != "" || tt.mask {
				req = &OpenAIImagesRequest{Model: tt.model, HasMask: tt.mask}
			}
			require.Equal(t, tt.want, AdobePrefersMaskRelay(req))
		})
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
