package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// OpenDesign Go 只登记一份端点画像：同一 /v1 基址服务 Chat Completions 与原生
// Responses，裸域另服务 Anthropic 兼容端点。
func TestProviderProfileOpenDesign(t *testing.T) {
	profile := LookupProviderProfile(PlatformOpenDesign)
	require.NotNil(t, profile, "open_design must register a provider profile")
	require.Equal(t, AccountModePayG, profile.DefaultMode)
	require.Equal(t, ProviderRoutingByInbound, profile.Routing)

	endpoints, ok := profile.Modes[AccountModePayG]
	require.True(t, ok, "payg mode must be registered")
	require.Equal(t, DefaultOpenDesignBaseURL, endpoints.BaseURLs[APIProtocolChatCompletions])
	require.Equal(t, DefaultOpenDesignBaseURL, endpoints.BaseURLs[APIProtocolResponses])
	require.Equal(t, DefaultOpenDesignAnthropicBaseURL, endpoints.BaseURLs[APIProtocolAnthropic])
}

// 自适应账号必须能为每一种入站协议解析出端点。
func TestProviderProfileOpenDesignAdaptiveResolution(t *testing.T) {
	profile := LookupProviderProfile(PlatformOpenDesign)
	require.NotNil(t, profile)

	for protocol, want := range map[string]string{
		APIProtocolChatCompletions: DefaultOpenDesignBaseURL,
		APIProtocolResponses:       DefaultOpenDesignBaseURL,
		APIProtocolAnthropic:       DefaultOpenDesignAnthropicBaseURL,
	} {
		require.True(t, profile.SupportsProtocol(AccountModePayG, protocol), "protocol %s must be supported", protocol)
		require.Equal(t, want, profile.DefaultBaseURL(AccountModePayG, protocol), "protocol %s", protocol)
	}
}
