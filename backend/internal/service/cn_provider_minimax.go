package service

// MiniMax 注册。
//
//   - 端点：OpenAI Chat Completions 兼容；另提供 Anthropic 兼容端点
//     （{base}/anthropic，适配 Claude Code 等客户端）。
//   - 余额/额度：无轻量公开查询端点，BalanceProbe/QuotaProbe 均为 nil——
//     纯响应式 402/429 处理（与 zhipu 余额探测同型）。
func init() { RegisterCNProvider(minimaxProviderSpec) }

var minimaxProviderSpec = CNProviderSpec{
	Code:        PlatformMinimax,
	DisplayName: "MiniMax",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, ""}: DefaultMinimaxBaseURL,
		{APIProtocolResponses, ""}:       DefaultMinimaxBaseURL,
		{APIProtocolAnthropic, ""}:       DefaultMinimaxAnthropicBaseURL,
	},
	BalanceProbe:            nil,
	QuotaProbe:              nil,
	SupportsNativeResponses: false,
	MatchCodingPlanBaseURL:  nil, // 无 coding plan 形态
	ModelDetectPrefixes:     []string{"minimax-"},
	ModelDetectAliases:      []string{"minimax", "minimaxi"},
	LiteLLMProvider:         "minimax",
}
