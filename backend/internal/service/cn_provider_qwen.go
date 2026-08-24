package service

// 通义千问 Qwen（阿里云百炼 / DashScope 兼容模式）注册。
//
//   - 端点：DashScope 兼容模式（OpenAI Chat Completions 兼容）。
//   - 余额/额度：百炼账户余额需经阿里云 BSS OpenAPI 查询（签名体系独立），
//     无轻量公开端点，BalanceProbe/QuotaProbe 均为 nil——纯响应式 402/429
//     处理（与 zhipu 余额探测同型）。
//   - 无原生 Anthropic / Responses 端点。
func init() { RegisterCNProvider(qwenProviderSpec) }

var qwenProviderSpec = CNProviderSpec{
	Code:        PlatformQwen,
	DisplayName: "Qwen (通义千问)",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, ""}: DefaultQwenBaseURL,
		{APIProtocolResponses, ""}:       DefaultQwenBaseURL,
	},
	BalanceProbe:            nil,
	QuotaProbe:              nil,
	SupportsNativeResponses: false,
	MatchCodingPlanBaseURL:  nil, // 无 coding plan 形态
	ModelDetectPrefixes:     []string{"qwen-"},
	ModelDetectAliases:      []string{"qwen", "dashscope"},
	LiteLLMProvider:         "dashscope",
}
