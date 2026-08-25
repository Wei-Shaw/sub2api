package service

// 讯飞星火（iFLYTEK Spark）注册。
//
//   - 端点：星火 OpenAI 兼容端点（Chat Completions）。
//   - 余额/额度：无轻量公开查询端点，BalanceProbe/QuotaProbe 均为 nil——
//     纯响应式 402/429 处理（与 zhipu 余额探测同型）。
//
// 平台 code 取 "xfspark" 而非 "spark"：后者已被 OpenAI Codex Spark
// （gpt-5.3-codex-spark）影子账号的配额维度（QuotaDimensionSpark）占用，
// 避免同名字符串在不同字段间产生语义混淆。模型名前缀 "spark-" 与
// OpenAI 的 spark 模型（均带 gpt- 前缀）不冲突。
func init() { RegisterCNProvider(xfsparkProviderSpec) }

var xfsparkProviderSpec = CNProviderSpec{
	Code:        PlatformXFSpark,
	DisplayName: "iFLYTEK Spark (讯飞星火)",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, ""}: DefaultXFSparkBaseURL,
		{APIProtocolResponses, ""}:       DefaultXFSparkBaseURL,
	},
	BalanceProbe:            nil,
	QuotaProbe:              nil,
	SupportsNativeResponses: false,
	MatchCodingPlanBaseURL:  nil, // 无 coding plan 形态
	ModelDetectPrefixes:     []string{"spark-"},
	ModelDetectAliases:      []string{"xfspark", "iflytek", "xunfei"},
	LiteLLMProvider:         "", // LiteLLM 定价目录无独立讯飞条目
}
