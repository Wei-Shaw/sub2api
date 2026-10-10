//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// generic 链（跨族账号池走 GatewayService.Forward / CountTokens）与 OpenAI 族原生
// Anthropic 链同一拼接规则：ollama / 按模型分流平台的 Anthropic 协议 base 带 /v1 时
// 版本感知拼接，不得拼出 /v1/v1/messages；其余供应商保持朴素拼接。
func TestAnthropicUpstreamTargetURL_VersionAwareForOllamaCloud(t *testing.T) {
	svc := &GatewayService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}

	ollama := &Account{
		ID: 1, Platform: PlatformOllamaCloud, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "ollama-key",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://ollama.com/v1",
		},
	}
	got, err := svc.anthropicUpstreamTargetURL(ollama, "/v1/messages")
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", got)
	got, err = svc.anthropicUpstreamTargetURL(ollama, "/v1/messages/count_tokens")
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages/count_tokens", got)

	ollama.Credentials["base_url"] = "https://ollama.com"
	got, err = svc.anthropicUpstreamTargetURL(ollama, "/v1/messages")
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", got)

	kimi := &Account{
		ID: 2, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "kimi-key",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://api.moonshot.cn/anthropic",
		},
	}
	got, err = svc.anthropicUpstreamTargetURL(kimi, "/v1/messages")
	require.NoError(t, err)
	require.Equal(t, "https://api.moonshot.cn/anthropic/v1/messages", got)
}
