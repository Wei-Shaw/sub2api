package service

// Command Code Provider API（https://commandcode.ai/docs/provider）：同一 API Key 下
// 按模型分流到三种原生端点，基址 https://api.commandcode.ai/provider/v1：
//   - Claude 系模型只在 /messages（Anthropic Messages）提供，发往其他端点返回 400；
//   - OpenAI 与开源模型提供 /chat/completions，多数也提供 /responses。
// 每个模型支持的端点由 /models 的 supported_endpoints 给出（公开接口），网关据此在
// 模型支持入站协议时同协议直通（见 model_protocol_catalog.go）；目录就绪前使用下面的
// 内置规则。模型名可带厂商前缀（如 deepseek/deepseek-v4-flash），原样发往上游。

// DefaultCommandCodeTestModel 是管理端连接测试未指定模型时使用的模型（Chat Completions）。
const DefaultCommandCodeTestModel = "deepseek/deepseek-v4-flash"

// DefaultCommandCodeProtocolRules 是 Command Code 的内置分流表：Claude 只走 Anthropic；
// GPT 同时支持 Responses 与 Chat Completions（入站同协议直通，其余入站首选 Responses）；
// 其余模型只保证 Chat Completions，是否支持 Responses 以上游模型目录为准。
func DefaultCommandCodeProtocolRules() []ProtocolRule {
	return []ProtocolRule{
		{Pattern: "claude-*", Protocol: APIProtocolAnthropic},
		{Pattern: "gpt-*", Protocol: APIProtocolResponses, Protocols: []string{APIProtocolResponses, APIProtocolChatCompletions}},
	}
}

// IsCommandCode 报告账号是否为 Command Code 平台账号。
func (a *Account) IsCommandCode() bool {
	return a != nil && a.Platform == PlatformCommandCode
}
