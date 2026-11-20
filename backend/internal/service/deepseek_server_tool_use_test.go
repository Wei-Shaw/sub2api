package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// DeepSeek 2026-11-13 新计费字段：usage.server_tool_use 的按次计费计数
// （tool_search / tool_fetch / tool_image_search，各 ¥5/千次）与审核 token
// 明细 output_tokens_details.safety_tokens（已含在输出 token，仅展示）。

func TestParseSSEUsagePassthroughServerToolUse(t *testing.T) {
	usage := &ClaudeUsage{}
	parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":100,"output_tokens":1,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":2,"tool_search":1,"tool_fetch":2,"tool_image_search":3},"output_tokens_details":{"safety_tokens":64}}}}`, usage)
	require.Equal(t, 6, usage.ServerToolUseCalls, "tool_search+tool_fetch+tool_image_search 合计；web_fetch_requests 仅展示不计费")
	require.Equal(t, 64, usage.SafetyTokens)

	// message_delta 重复上报累计值时取较大值，不叠加
	parseSSEUsagePassthrough(`{"type":"message_delta","usage":{"output_tokens":42,"server_tool_use":{"tool_search":4,"tool_fetch":2,"tool_image_search":3},"output_tokens_details":{"safety_tokens":96}}}`, usage)
	require.Equal(t, 9, usage.ServerToolUseCalls)
	require.Equal(t, 96, usage.SafetyTokens)
	require.Equal(t, 42, usage.OutputTokens)
}

func TestParseSSEUsagePassthroughServerToolUseOmitted(t *testing.T) {
	// 全为 0 时官方省略整个 server_tool_use 字段；缺失时保持零值
	usage := &ClaudeUsage{}
	parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":2}}}`, usage)
	require.Zero(t, usage.ServerToolUseCalls)
	require.Zero(t, usage.SafetyTokens)

	usage = &ClaudeUsage{}
	parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":2,"server_tool_use":{"tool_search":0,"tool_fetch":0,"tool_image_search":0}}}}`, usage)
	require.Zero(t, usage.ServerToolUseCalls)
}

func TestParseClaudeUsageFromResponseBodyServerToolUse(t *testing.T) {
	usage := parseClaudeUsageFromResponseBody([]byte(`{"usage":{"input_tokens":120,"output_tokens":88,"server_tool_use":{"web_search_requests":1,"web_fetch_requests":0,"tool_search":2,"tool_fetch":1,"tool_image_search":0},"output_tokens_details":{"safety_tokens":32}}}`))
	require.Equal(t, 3, usage.ServerToolUseCalls)
	require.Equal(t, 32, usage.SafetyTokens)
	require.Equal(t, 88, usage.OutputTokens)

	// 字段缺失容错
	usage = parseClaudeUsageFromResponseBody([]byte(`{"usage":{"input_tokens":120,"output_tokens":88}}`))
	require.Zero(t, usage.ServerToolUseCalls)
	require.Zero(t, usage.SafetyTokens)
}

func TestOpenAIUsageFromGJSONServerToolUse(t *testing.T) {
	// Messages/Responses：usage.server_tool_use + output_tokens_details.safety_tokens
	usage, ok := openAIUsageFromGJSON(gjson.Parse(`{"input_tokens":100,"output_tokens":50,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":1,"tool_search":5,"tool_fetch":0,"tool_image_search":2},"output_tokens_details":{"safety_tokens":20}}`))
	require.True(t, ok)
	require.Equal(t, 7, usage.ServerToolUseCalls)
	require.Equal(t, 20, usage.SafetyTokens)

	// CC：同名嵌套 + completion_tokens_details.safety_tokens
	usage, ok = openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":100,"completion_tokens":50,"server_tool_use":{"tool_search":1,"tool_fetch":1,"tool_image_search":1},"completion_tokens_details":{"safety_tokens":8}}`))
	require.True(t, ok)
	require.Equal(t, 3, usage.ServerToolUseCalls)
	require.Equal(t, 8, usage.SafetyTokens)

	// 字段全缺省容错
	usage, ok = openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":100,"completion_tokens":50}`))
	require.True(t, ok)
	require.Zero(t, usage.ServerToolUseCalls)
	require.Zero(t, usage.SafetyTokens)
}

func TestServerToolUseBillingAmount(t *testing.T) {
	// 工具费与 Grok web_search 共用 CalculateSearchCost 通道：默认单价
	// defaultSearchPricePer1k=5.0（¥/千次），与 DeepSeek 官方口径一致。
	s := &BillingService{}
	cost := s.CalculateSearchCost(6, nil, 1)
	require.InDelta(t, 0.03, cost.TotalCost, 1e-9)
	require.InDelta(t, 0.03, cost.ActualCost, 1e-9)

	// 计费次数 = Grok SearchCount + DeepSeek server_tool_use 合计次数
	forward := &ForwardResult{SearchCount: 2}
	forward.Usage.ServerToolUseCalls = 4
	require.Equal(t, 6, forward.SearchCount+forward.Usage.ServerToolUseCalls)

	openaiResult := &OpenAIForwardResult{SearchCount: 1}
	openaiResult.Usage.ServerToolUseCalls = 5
	require.Equal(t, 6, openaiResult.SearchCount+openaiResult.Usage.ServerToolUseCalls)

	// 全为 0 时不产生费用
	require.Equal(t, 0.0, s.CalculateSearchCost(0, nil, 1).ActualCost)
}
