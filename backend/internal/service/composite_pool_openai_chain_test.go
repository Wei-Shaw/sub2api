//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 跨族账号池中 adaptive 多协议账号的转发链选择：按模型分流的聚合平台与没有
// Anthropic 原生端点的供应商只能由 OpenAI 网关链转发，其余具备 Anthropic 原生
// 端点的供应商仍走 generic 转发。
func TestRequiresOpenAIProtocolChain(t *testing.T) {
	apiKey := func(platform string, credentials map[string]any) *Account {
		creds := map[string]any{"api_key": "sk-test"}
		for k, v := range credentials {
			creds[k] = v
		}
		return &Account{ID: 1, Platform: platform, Type: AccountTypeAPIKey, Credentials: creds}
	}
	cases := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"kimi adaptive has native anthropic", apiKey(PlatformKimi, map[string]any{"api_protocol": APIProtocolAdaptive}), false},
		{"ollama_cloud default adaptive has native anthropic", apiKey(PlatformOllamaCloud, nil), false},
		{"opencode_go routes by model", apiKey(PlatformOpenCodeGo, nil), true},
		{"command_code routes by model", apiKey(PlatformCommandCode, nil), true},
		{"cline adaptive has no anthropic endpoint", apiKey(PlatformCline, map[string]any{"api_protocol": APIProtocolAdaptive}), true},
		{"cline default chat completions is not adaptive", apiKey(PlatformCline, nil), false},
		{"kimi pinned anthropic", apiKey(PlatformKimi, map[string]any{"api_protocol": APIProtocolAnthropic}), false},
		{"openai is not multi-protocol", apiKey(PlatformOpenAI, nil), false},
		{"nil account", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.account.RequiresOpenAIProtocolChain())
		})
	}
}

func TestGenericCompositePoolOpenAIChainAccounts(t *testing.T) {
	cline := &Account{ID: 2, Platform: PlatformCline, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk", "api_protocol": APIProtocolAdaptive}}
	commandCode := &Account{ID: 3, Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}
	kimi := &Account{ID: 4, Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk", "api_protocol": APIProtocolAdaptive}}

	// 委派链账号可入池（不要求 Anthropic 协议 base），但不能由 generic count_tokens 计数。
	for _, account := range []*Account{cline, commandCode} {
		require.True(t, genericCompositePoolAccountServable(account), account.Platform)
		require.False(t, GenericCompositePoolCountableAccount(account), account.Platform)
	}
	require.True(t, genericCompositePoolAccountServable(kimi))
	require.True(t, GenericCompositePoolCountableAccount(kimi))
}
