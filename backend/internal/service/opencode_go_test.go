package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeGoDefaultProtocolRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		model string
		want  string
	}{
		{"grok-4.6", APIProtocolResponses},
		{"opencode-go/gpt-5.6-luna", APIProtocolResponses},
		{"muse-spark-1.3-contributor", APIProtocolResponses},
		{"minimax-m3", APIProtocolAnthropic},
		{"qwen3.8-max", APIProtocolAnthropic},
		{"qwen3.7-plus", APIProtocolAnthropic},
		{"glm-5.3", APIProtocolChatCompletions},
		{"kimi-k3", APIProtocolChatCompletions},
		{"deepseek-v4-pro", APIProtocolChatCompletions},
		{"mimo-v2.5-pro", APIProtocolChatCompletions},
		{"hy4-preview", APIProtocolChatCompletions},
		{"omen-alpha", APIProtocolChatCompletions},
		{"", APIProtocolChatCompletions},
		{"unknown-model", APIProtocolChatCompletions},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, matchProtocolRules(tc.model, DefaultOpenCodeGoProtocolRules()), "model=%s", tc.model)
	}
}

func TestOpenCodeGoModelRoutedProtocol(t *testing.T) {
	t.Parallel()
	adaptive := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}
	require.Equal(t, APIProtocolAnthropic, adaptive.resolveModelRoutedProtocol("minimax-m3"))
	require.Equal(t, APIProtocolResponses, adaptive.resolveModelRoutedProtocol("grok-4.6"))
	require.Equal(t, APIProtocolChatCompletions, adaptive.resolveModelRoutedProtocol("glm-5.3"))

	pinned := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}}
	require.Equal(t, APIProtocolChatCompletions, pinned.resolveModelRoutedProtocol("minimax-m3"))

	empty := &Account{Platform: PlatformOpenCodeGo}
	require.Equal(t, APIProtocolResponses, empty.resolveModelRoutedProtocol("gpt-5.6-luna"))
	require.Equal(t, AccountModeGo, empty.GetOpenCodeAccountMode())
	require.True(t, empty.IsOpenCodeGoPlan())

	zen := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"account_mode": AccountModeZen, "api_protocol": APIProtocolAdaptive}}
	require.Equal(t, APIProtocolChatCompletions, zen.resolveModelRoutedProtocol("minimax-m3"))
	require.Equal(t, APIProtocolAnthropic, zen.resolveModelRoutedProtocol("claude-opus-4-6"))
	require.Equal(t, DefaultOpenCodeZenBaseURL, zen.GetOpenAIBaseURL())
}

func TestOpenCodeGoModelRoutedProtocolUsesAccountRules(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
			protocolRulesCredentialKey: []any{
				map[string]any{"pattern": "grok-*", "protocol": APIProtocolChatCompletions},
				map[string]any{"pattern": "deepseek-v4-flash", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("grok-4.6"))
	require.Equal(t, APIProtocolResponses, account.resolveModelRoutedProtocol("deepseek-v4-flash"))
	require.Equal(t, APIProtocolAnthropic, account.resolveModelRoutedProtocol("qwen3.8-max"))
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("glm-5.3"))
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("minimax-m3"))
}

func TestOpenCodeGoModelRoutedProtocolEmptyRulesAreAllChatCompletions(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			protocolRulesCredentialKey: []any{},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("grok-4.6"))
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("minimax-m3"))
}

func TestOpenCodeGoModelRoutedProtocolFirstMatchWins(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			protocolRulesCredentialKey: []any{
				map[string]any{"pattern": "gpt-5.6-luna", "protocol": APIProtocolChatCompletions},
				map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.resolveModelRoutedProtocol("gpt-5.6-luna"))
	require.Equal(t, APIProtocolResponses, account.resolveModelRoutedProtocol("gpt-5.4"))
}

func TestOpenCodeGoUnmatchedFallsBackToChatCompletions(t *testing.T) {
	t.Parallel()
	withRules := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
			protocolRulesCredentialKey: []any{
				map[string]any{"pattern": "grok-*", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, withRules.resolveModelRoutedProtocol("deepseek-v4-flash"))
	require.Equal(t, APIProtocolChatCompletions, withRules.resolveModelRoutedProtocol("glm-5.3"))
	require.Equal(t, APIProtocolChatCompletions, withRules.resolveModelRoutedProtocol("omen-alpha"))
	require.Equal(t, APIProtocolChatCompletions, withRules.resolveModelRoutedProtocol("kimi-k3"))
	require.Equal(t, APIProtocolChatCompletions, withRules.resolveModelRoutedProtocol("unknown-model"))
	require.Equal(t, APIProtocolResponses, withRules.resolveModelRoutedProtocol("grok-4.6"))
	require.Equal(t, APIProtocolAnthropic, withRules.resolveModelRoutedProtocol("qwen3.8-flash"))

	defaults := &Account{Platform: PlatformOpenCodeGo}
	require.Equal(t, APIProtocolChatCompletions, defaults.resolveModelRoutedProtocol("deepseek-v4-flash"))
}

func TestParseOpenCodeGoProtocolRulesAcceptsTypedMaps(t *testing.T) {
	t.Parallel()
	rules, err := parseProtocolRules([]map[string]any{
		{"pattern": "grok-*", "protocol": APIProtocolResponses},
		{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
	})
	require.NoError(t, err)
	require.Equal(t, []ProtocolRule{
		{Pattern: "grok-*", Protocol: APIProtocolResponses},
		{Pattern: "qwen*", Protocol: APIProtocolAnthropic},
	}, rules)
}

func TestShouldForwardOpenAIResponsesViaRawChatCompletions_OpenCodeGoIgnoresProbe(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
		},
		Extra: map[string]any{
			"openai_responses_supported": false,
		},
	}
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account))
	require.Equal(t, APIProtocolResponses, account.resolveModelRoutedProtocol("grok-4.6"))
	require.Equal(t, APIProtocolResponses, account.resolveModelRoutedProtocol("gpt-5.6-luna"))
	require.Equal(t, APIProtocolAnthropic, account.resolveModelRoutedProtocol("qwen3.8-flash"))
}

func TestStampOpenAIResponsesUpstreamEndpoint(t *testing.T) {
	t.Parallel()
	result := &OpenAIForwardResult{}
	stampOpenAIResponsesUpstreamEndpoint(nil, result)
	require.Equal(t, "/v1/responses", result.UpstreamEndpoint)

	existing := &OpenAIForwardResult{UpstreamEndpoint: "/v1/messages"}
	stampOpenAIResponsesUpstreamEndpoint(nil, existing)
	require.Equal(t, "/v1/messages", existing.UpstreamEndpoint)
}

func TestNormalizeOpenCodeGoProtocolRulesCredentials(t *testing.T) {
	t.Parallel()
	require.NoError(t, NormalizeProtocolRulesCredentials(nil))
	require.NoError(t, NormalizeProtocolRulesCredentials(map[string]any{}))

	creds := map[string]any{
		protocolRulesCredentialKey: []any{
			map[string]any{"pattern": " Grok-* ", "protocol": APIProtocolResponses},
		},
	}
	require.NoError(t, NormalizeProtocolRulesCredentials(creds))
	rules, ok := creds[protocolRulesCredentialKey].([]any)
	require.True(t, ok)
	require.NotEmpty(t, rules)
	entry, ok := rules[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "grok-*", entry["pattern"])

	err := NormalizeProtocolRulesCredentials(map[string]any{
		protocolRulesCredentialKey: []any{
			map[string]any{"pattern": "*grok", "protocol": APIProtocolResponses},
		},
	})
	require.Error(t, err)

	err = NormalizeProtocolRulesCredentials(map[string]any{
		protocolRulesCredentialKey: []any{
			map[string]any{"pattern": "grok-*", "protocol": "adaptive"},
		},
	})
	require.Error(t, err)
}

func TestOpenCodeGoQuotaURL(t *testing.T) {
	t.Parallel()
	const want = "https://opencode.ai/zen/go/v1/usage"
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"empty falls back to default", "", want},
		{"chat base (/v1)", DefaultOpenCodeGoBaseURL, want},
		{"chat base trailing slash", DefaultOpenCodeGoBaseURL + "/", want},
		{"anthropic base (no /v1)", DefaultOpenCodeGoAnthropicBaseURL, want},
		{"anthropic base trailing slash", DefaultOpenCodeGoAnthropicBaseURL + "/", want},
		{"zen base stays idempotent", DefaultOpenCodeZenBaseURL, "https://opencode.ai/zen/v1/usage"},
		{"custom base with /v1", "https://custom.example/v1", "https://custom.example/v1/usage"},
		{"custom base with /v1 trailing slash", "https://custom.example/v1/", "https://custom.example/v1/usage"},
		// 与 kimiQuotaURL 同语义：无条件补 /v1，非官方域名同样归一。
		{"custom base without /v1", "https://custom.example", "https://custom.example/v1/usage"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, openCodeGoQuotaURL(tc.baseURL), "case=%s", tc.name)
	}
}

func TestDefaultOpenCodeGoModelIDsCoverDocumentedCatalog(t *testing.T) {
	t.Parallel()
	ids := DefaultOpenCodeGoModelIDs()
	require.Contains(t, ids, "grok-4.6")
	require.Contains(t, ids, "minimax-m3")
	require.Contains(t, ids, "glm-5.3")
	require.NotEmpty(t, ids)
}
