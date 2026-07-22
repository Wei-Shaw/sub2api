//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func bailianURLTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{}}
}

func TestBailianChatCompletionsURLDefault(t *testing.T) {
	svc := bailianURLTestService()
	account := &Account{
		Platform:    PlatformBailian,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "k"},
	}
	url, err := svc.bailianChatCompletionsURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", url)
}

func TestBailianChatCompletionsURLStripsPastedSuffix(t *testing.T) {
	svc := bailianURLTestService()
	account := &Account{
		Platform: PlatformBailian,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "k",
			"base_url": "https://ws-9.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/",
		},
	}
	url, err := svc.bailianChatCompletionsURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://ws-9.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/chat/completions", url)
}

func TestRawChatCompletionsURLRoutesBailian(t *testing.T) {
	svc := bailianURLTestService()
	account := &Account{
		Platform:    PlatformBailian,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "k"},
	}
	url, err := svc.rawChatCompletionsURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", url)
}

func TestResolveCCFallbackTargetBailian(t *testing.T) {
	svc := bailianURLTestService()
	account := &Account{
		ID:          3,
		Platform:    PlatformBailian,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": " dashscope-key "},
	}
	apiKey, targetURL, err := svc.resolveCCFallbackTarget(account)
	require.NoError(t, err)
	require.Equal(t, "dashscope-key", apiKey)
	require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", targetURL)

	missing := &Account{ID: 4, Platform: PlatformBailian, Type: AccountTypeAPIKey, Credentials: map[string]any{}}
	_, _, err = svc.resolveCCFallbackTarget(missing)
	require.Error(t, err)
}

func TestNormalizeOpenAICompatiblePlatformBailian(t *testing.T) {
	require.Equal(t, PlatformBailian, normalizeOpenAICompatiblePlatform(PlatformBailian))
	require.Equal(t, PlatformGrok, normalizeOpenAICompatiblePlatform(PlatformGrok))
	require.Equal(t, PlatformOpenAI, normalizeOpenAICompatiblePlatform(PlatformOpenAI))
	require.Equal(t, PlatformOpenAI, normalizeOpenAICompatiblePlatform(PlatformAnthropic))
}

func TestBailianAccountIsOpenAICompatible(t *testing.T) {
	account := &Account{Platform: PlatformBailian, Type: AccountTypeAPIKey}
	require.True(t, account.IsOpenAICompatible())
	require.True(t, account.IsBailian())
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityEmbeddings))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAlphaSearch))
	require.True(t, account.SupportsOpenAIEndpointCapability(""))
}
