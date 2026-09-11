package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeGoModelProtocol(t *testing.T) {
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
		require.Equal(t, tc.want, OpenCodeGoModelProtocol(tc.model), "model=%s", tc.model)
	}
}

func TestResolveOpenCodeGoUpstreamProtocol(t *testing.T) {
	t.Parallel()
	adaptive := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"api_protocol": APIProtocolAdaptive}}
	require.Equal(t, APIProtocolAnthropic, adaptive.ResolveOpenCodeGoUpstreamProtocol("minimax-m3"))
	require.Equal(t, APIProtocolResponses, adaptive.ResolveOpenCodeGoUpstreamProtocol("grok-4.6"))
	require.Equal(t, APIProtocolChatCompletions, adaptive.ResolveOpenCodeGoUpstreamProtocol("glm-5.3"))

	pinned := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}}
	require.Equal(t, APIProtocolChatCompletions, pinned.ResolveOpenCodeGoUpstreamProtocol("minimax-m3"))

	empty := &Account{Platform: PlatformOpenCodeGo}
	require.Equal(t, APIProtocolResponses, empty.ResolveOpenCodeGoUpstreamProtocol("gpt-5.6-luna"))
	require.Equal(t, AccountModeGo, empty.GetOpenCodeAccountMode())
	require.True(t, empty.IsOpenCodeGoPlan())

	zen := &Account{Platform: PlatformOpenCodeGo, Credentials: map[string]any{"account_mode": AccountModeZen, "api_protocol": APIProtocolAdaptive}}
	require.Equal(t, APIProtocolChatCompletions, zen.ResolveOpenCodeGoUpstreamProtocol("minimax-m3"))
	require.Equal(t, APIProtocolAnthropic, zen.ResolveOpenCodeGoUpstreamProtocol("claude-opus-4-6"))
	require.Equal(t, DefaultOpenCodeZenBaseURL, zen.GetOpenAIBaseURL())

	require.Equal(t, "", (&Account{Platform: PlatformKimi}).ResolveOpenCodeGoUpstreamProtocol("glm-5.3"))
}

func TestResolveOpenCodeGoUpstreamProtocolUsesAccountRules(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
			openCodeGoProtocolRulesKey: []any{
				map[string]any{"pattern": "grok-*", "protocol": APIProtocolChatCompletions},
				map[string]any{"pattern": "deepseek-v4-flash", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("grok-4.6"))
	require.Equal(t, APIProtocolResponses, account.ResolveOpenCodeGoUpstreamProtocol("deepseek-v4-flash"))
	require.Equal(t, APIProtocolAnthropic, account.ResolveOpenCodeGoUpstreamProtocol("qwen3.8-max"))
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("glm-5.3"))
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("minimax-m3"))
}

func TestResolveOpenCodeGoUpstreamProtocolEmptyRulesAreAllChatCompletions(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			openCodeGoProtocolRulesKey: []any{},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("grok-4.6"))
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("minimax-m3"))
}

func TestResolveOpenCodeGoUpstreamProtocolFirstMatchWins(t *testing.T) {
	t.Parallel()
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			openCodeGoProtocolRulesKey: []any{
				map[string]any{"pattern": "gpt-5.6-luna", "protocol": APIProtocolChatCompletions},
				map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, account.ResolveOpenCodeGoUpstreamProtocol("gpt-5.6-luna"))
	require.Equal(t, APIProtocolResponses, account.ResolveOpenCodeGoUpstreamProtocol("gpt-5.4"))
}

func TestOpenCodeGoNativeProtocolUnmatchedFallsBackToChatCompletions(t *testing.T) {
	t.Parallel()
	withRules := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
			openCodeGoProtocolRulesKey: []any{
				map[string]any{"pattern": "grok-*", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "gpt-*", "protocol": APIProtocolResponses},
				map[string]any{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
			},
		},
	}
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(withRules, "deepseek-v4-flash"))
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(withRules, "glm-5.3"))
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(withRules, "omen-alpha"))
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(withRules, "kimi-k3"))
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(withRules, "unknown-model"))
	require.Equal(t, APIProtocolResponses, openCodeGoNativeProtocol(withRules, "grok-4.6"))
	require.Equal(t, APIProtocolAnthropic, openCodeGoNativeProtocol(withRules, "qwen3.8-flash"))

	defaults := &Account{Platform: PlatformOpenCodeGo}
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(defaults, "deepseek-v4-flash"))
	require.Equal(t, APIProtocolChatCompletions, openCodeGoNativeProtocol(nil, "grok-4.6"))
}

func TestParseOpenCodeGoProtocolRulesAcceptsTypedMaps(t *testing.T) {
	t.Parallel()
	rules, err := parseOpenCodeGoProtocolRules([]map[string]any{
		{"pattern": "grok-*", "protocol": APIProtocolResponses},
		{"pattern": "qwen*", "protocol": APIProtocolAnthropic},
	})
	require.NoError(t, err)
	require.Equal(t, []OpenCodeGoProtocolRule{
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
	require.Equal(t, APIProtocolResponses, account.ResolveOpenCodeGoUpstreamProtocol("grok-4.6"))
	require.Equal(t, APIProtocolResponses, account.ResolveOpenCodeGoUpstreamProtocol("gpt-5.6-luna"))
	require.Equal(t, APIProtocolAnthropic, account.ResolveOpenCodeGoUpstreamProtocol("qwen3.8-flash"))
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
	require.NoError(t, NormalizeOpenCodeGoProtocolRulesCredentials(nil))
	require.NoError(t, NormalizeOpenCodeGoProtocolRulesCredentials(map[string]any{}))

	creds := map[string]any{
		openCodeGoProtocolRulesKey: []any{
			map[string]any{"pattern": " Grok-* ", "protocol": APIProtocolResponses},
		},
	}
	require.NoError(t, NormalizeOpenCodeGoProtocolRulesCredentials(creds))
	rules, ok := creds[openCodeGoProtocolRulesKey].([]any)
	require.True(t, ok)
	require.NotEmpty(t, rules)
	entry, ok := rules[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "grok-*", entry["pattern"])

	err := NormalizeOpenCodeGoProtocolRulesCredentials(map[string]any{
		openCodeGoProtocolRulesKey: []any{
			map[string]any{"pattern": "*grok", "protocol": APIProtocolResponses},
		},
	})
	require.Error(t, err)

	err = NormalizeOpenCodeGoProtocolRulesCredentials(map[string]any{
		openCodeGoProtocolRulesKey: []any{
			map[string]any{"pattern": "grok-*", "protocol": "adaptive"},
		},
	})
	require.Error(t, err)
}

func TestOpenCodeGoQuotaURL(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://opencode.ai/zen/go/v1/usage", openCodeGoQuotaURL(""))
	require.Equal(t, "https://opencode.ai/zen/go/v1/usage", openCodeGoQuotaURL(DefaultOpenCodeGoBaseURL+"/"))
	require.Equal(t, "https://custom.example/v1/usage", openCodeGoQuotaURL("https://custom.example/v1"))
}

func TestDefaultOpenCodeGoModelIDsCoverDocumentedCatalog(t *testing.T) {
	t.Parallel()
	ids := DefaultOpenCodeGoModelIDs()
	require.Contains(t, ids, "grok-4.6")
	require.Contains(t, ids, "minimax-m3")
	require.Contains(t, ids, "glm-5.3")
	require.NotEmpty(t, ids)
}
