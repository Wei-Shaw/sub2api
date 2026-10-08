package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// aggregatorMappedAccount 复现"把 GPT 模型名映射到聚合站上的 deepseek-* 模型"
// 的账号：platform=openai、hostname 不是 api.deepseek.com、model_mapping 指向
// deepseek-*。这类账号与官方 DeepSeek 接受同一套兼容处理（实测确认）。
func aggregatorMappedAccount() *Account {
	return &Account{
		ID:       9001,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_responses_mode": "force_chat_completions"},
		Credentials: map[string]any{
			"api_key":  "sk-aggregator",
			"base_url": "https://aggregator.example",
			"model_mapping": map[string]any{
				"gpt-5.6-sol": "deepseek-v4.1-flash",
				"gpt-image-2": "some-image-model",
			},
		},
	}
}

func TestIsDeepSeekSemanticsUpstream(t *testing.T) {
	official := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.deepseek.com"},
	}
	officialLookalike := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.deepseek.com.evil.example"},
	}
	plain := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.openai.com/v1"},
	}

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		want           bool
	}{
		{name: "nil account", account: nil, want: false},
		{name: "official hostname", account: official, want: true},
		{name: "hostname lookalike rejected", account: officialLookalike, want: false},
		{
			// 聚合站：hostname 不是 DeepSeek，但本次请求映射到 deepseek-*。
			name: "aggregator mapped model", account: aggregatorMappedAccount(),
			requestedModel: "gpt-5.6-sol", want: true,
		},
		{
			// 同一账号上映射到非 DeepSeek 模型的请求不应被改道。
			name: "aggregator non-deepseek model", account: aggregatorMappedAccount(),
			requestedModel: "gpt-image-2", want: false,
		},
		{name: "non deepseek account", account: plain, requestedModel: "gpt-5.6-sol", want: false},
		{
			// 未提供请求模型时退回账号级判定：该账号确实有 DeepSeek 映射。
			name: "aggregator without requested model", account: aggregatorMappedAccount(), want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isDeepSeekResponsesUpstream(tt.account, tt.requestedModel))
		})
	}
}

func TestShouldForwardAggregatorDeepSeekLiteViaChatCompletions(t *testing.T) {
	account := aggregatorMappedAccount()

	// Responses Lite 形状：工具声明在 input[].additional_tools。
	lite := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"exec_command"}]}]}`)
	require.True(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(account, lite))
	require.True(t, shouldForwardOpenAIResponsesViaChatCompletions(account, lite))

	// 同一账号上未映射到 DeepSeek 的模型不应被改道（避免误伤混合映射账号）。
	nonDeepSeekLite := []byte(`{"model":"gpt-image-2","input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"exec_command"}]}]}`)
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(account, nonDeepSeekLite))

	// 传统顶层 tools 形状本来就工作正常，不应触发改道。
	classic := []byte(`{"model":"gpt-5.6-sol","tools":[{"type":"function","name":"exec_command"}],"input":[]}`)
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(account, classic))
}

func TestShouldForwardAggregatorDeepSeekCompactViaChatCompletions(t *testing.T) {
	account := aggregatorMappedAccount()
	compact := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":"hi"},{"type":"compaction_trigger"}]}`)
	require.True(t, shouldForwardDeepSeekResponsesCompactViaChatCompletions(account, compact))

	notCompact := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":"hi"}]}`)
	require.False(t, shouldForwardDeepSeekResponsesCompactViaChatCompletions(account, notCompact))
}

func TestStripAggregatorDeepSeekJSONSchemaResponseFormat(t *testing.T) {
	account := aggregatorMappedAccount()
	jsonSchema := []byte(`{"model":"deepseek-v4.1-flash","response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`)
	stripped := stripDeepSeekUnsupportedChatResponseFormat(account, jsonSchema)
	require.NotContains(t, string(stripped), "response_format")

	// 非 DeepSeek 模型保持原样（字节一致）。
	plainBody := []byte(`{"model":"some-other-model","response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}}}`)
	require.Equal(t, string(plainBody), string(stripDeepSeekUnsupportedChatResponseFormat(account, plainBody)))
}

// TestResponsesCompactCapabilityAllowsChatBridgeAccounts 覆盖 #7285 的压缩 503：
// 纯 chat 账号（openai_responses_supported=false）但映射 deepseek-* 模型时，
// native remote compaction v2 由 chat 桥承接，调度不应再判 capability_mismatch。
func TestResponsesCompactCapabilityAllowsChatBridgeAccounts(t *testing.T) {
	chatBridge := aggregatorMappedAccount()
	chatBridge.Extra = map[string]any{"openai_responses_supported": false}
	require.True(t,
		chatBridge.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesCompact),
		"chat 桥账号必须能承接 native compaction v2")

	// 同一账号走生图意图的 Responses 能力时仍被排除：生图不能降级到 chat。
	require.False(t,
		chatBridge.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses),
		"生图意图仍要求原生 Responses，避免静默降级（#4417）")

	// 无 DeepSeek 映射的 chat-only 账号维持原判定。
	plainChatOnly := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Extra:       map[string]any{"openai_responses_supported": false},
		Credentials: map[string]any{"api_key": "sk-x", "base_url": "https://plain.example"},
	}
	require.False(t,
		plainChatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesCompact),
		"非 DeepSeek 的 chat-only 账号不应被放行")

	// 原生 Responses 账号行为不变。
	native := aggregatorMappedAccount()
	native.Extra = map[string]any{"openai_responses_supported": true}
	require.True(t, native.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesCompact))
	require.True(t, native.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
}
