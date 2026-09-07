package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"

// kiroFakeThinkingDefaultBudget 是客户端完全没有表达推理强度诉求时的默认
// 预算——这次改动前 forwardUpstream 一直写死这个值，保留它作为兜底，不
// 改变没有诉求这一种情况下的既有行为。
const kiroFakeThinkingDefaultBudget = 4000

// kiroReasoningEffortBudgets 把 Anthropic 的 effort 档位换算成假思考预算，
// 复用 openai_reasoning_effort_policy.go 的 reasoningEffortBudgetTokens——
// 不止 low/medium/high/max 四档：虽然 Responses 客户端的 "xhigh" 会在
// apicompat.ResponsesToAnthropicRequest 里被 mapResponsesEffortToAnthropic
// 归一化成 Anthropic 的 "max"，但原生 Anthropic 客户端可以不经这条转换链、
// 直接对 /v1/messages 发送 output_config.effort:"xhigh"（
// anthropicReasoningEffortValues 明确把它列成独立的一档，不是 max 的同义
// 词）——这里若漏掉 xhigh 这个 key，查表会落空（ok=false），悄悄退化成
// kiroFakeThinkingDefaultBudget（4000），比真正的 xhigh 预算小得多，是这次
// 复查发现的真实缺口，不是假设性场景。
var kiroReasoningEffortBudgets = reasoningEffortBudgetTokens

// kiroFakeThinkingPlan 决定这次请求要不要注入假思考、注入多大的预算。
//
// 账号级 KiroFakeThinking() 仍然是总闸，默认开启——客户端携带的推理强度
// 诉求必须能在 Kiro 端真实生效，不能被这道闸默默挡住。管理员为不接受
// "假装思考"的质量/延迟代价的账号（假思考产出的是模型自写文本而非真实
// reasoning，见 KiroCredentialFields.vue 的说明）显式设置 fake_thinking:false
// 才会关闭，不管这次请求本身诉求是什么。
//
// 账号没被显式关闭的前提下，具体这次给多大预算由请求自己表达（此前 forwardUpstream
// 一直传固定的 4000，完全无视客户端诉求——真实网关请求携带的
// thinking/output_config 从未被 kiro.BuildRequest 读取过，是这次修的缺口）：
//  1. 客户端显式要求不要思考（Anthropic 原生 thinking.type == "disabled"）
//     ——尊重这个请求级的显式表达，这次不注入，即使账号开关是开的。
//  2. 客户端给了具体的 budget_tokens（原生 Anthropic thinking 参数，或
//     Claude Code 这类真正支持 thinking 的客户端直接发送）——直接用这个
//     数字，不做二次换算。
//  3. 客户端给了 output_config.effort（Codex 客户端的 reasoning.effort
//     经 apicompat.ResponsesToAnthropicRequest 转换而来，或原生 Anthropic
//     客户端直接发送 output_config）——按 kiroReasoningEffortBudgets 换算。
//  4. 都没给——退回这次改动前的固定默认值 kiroFakeThinkingDefaultBudget，
//     行为不变。
func kiroFakeThinkingPlan(account *Account, inbound *apicompat.AnthropicRequest) (enabled bool, maxTokens int) {
	if account == nil || !account.KiroFakeThinking() {
		return false, 0
	}
	if inbound == nil {
		return true, kiroFakeThinkingDefaultBudget
	}
	if inbound.Thinking != nil {
		if inbound.Thinking.Type == "disabled" {
			return false, 0
		}
		if inbound.Thinking.BudgetTokens > 0 {
			return true, inbound.Thinking.BudgetTokens
		}
	}
	if inbound.OutputConfig != nil && inbound.OutputConfig.Effort != "" {
		if budget, ok := kiroReasoningEffortBudgets[inbound.OutputConfig.Effort]; ok {
			return true, budget
		}
	}
	return true, kiroFakeThinkingDefaultBudget
}
