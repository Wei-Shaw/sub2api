package service

import (
	"context"
	"strings"
)

// composite 跨平台候选池（account_pool）在 generic（Anthropic 原生）网关侧的接入点。
//
// 池上下文契约与能力谓词的所有权在 composite_platform.go / composite_account_claims.go；
// OpenAI 兼容族接入点镜像见 openai_gateway_scheduling.go。已定稿设计中 generic 网关
// 是统一池执行器：按选中账号的协议能力自动分发转发，因此池成员资格只要求
// 「平台 ∈ 候选池」且账号可被 generic messages 路径服务，具体协议分支（原生
// Anthropic / anthropic-协议 / adaptive / OpenAI 兼容委派）由 handler 层分发决定。
// 本文件能力判定全部走能力谓词，不引入新的平台硬编码清单——ollama_cloud 合入后
// 凭账号协议能力自动继承，无需改这里。

// GenericCompositePoolActive 报告当前请求是否按「composite account_pool」进入
// generic 调度器。语义镜像 openAICompositePoolActive：显式 resolved 平台优先于池
// （ctx 已带 ResolvedTargetPlatform，含 Gemini pin 语义，此时即使 ctx 残留候选池
// 也按单平台旧行为调度）；池请求 ctx 携带候选池且无 resolved 平台。
// 导出供 handler 层做池激活分发判定，与调度器共用同一谓词。
func GenericCompositePoolActive(ctx context.Context) bool {
	if _, resolved := ResolvedTargetPlatformFromContext(ctx); resolved {
		return false
	}
	return len(CompositeCandidatePlatformsFromContext(ctx)) > 0
}

// genericCompositePoolAllowsAccount 判定账号是否属于本次请求的候选池，且账号可被
// generic messages 路径服务。无池时恒为 false，调用方据此保持原平台相等门行为；
// 候选池 immutable，选号 / sticky / 抢槽后复检都必须重新经过本谓词，防止池外账号
// 经复检路径混入（池塌缩）。
func genericCompositePoolAllowsAccount(ctx context.Context, account *Account) bool {
	if account == nil || !genericCompositePoolAccountServable(account) {
		return false
	}
	for _, platform := range CompositeCandidatePlatformsFromContext(ctx) {
		if platform == account.Platform {
			return true
		}
	}
	return false
}

// genericCompositePoolAccountServable 报告账号能否被 generic messages 路径服务。
// 纯能力判定，不依赖候选池：
//   - PlatformAnthropic：generic 网关原生账号（OAuth/APIKey/ServiceAccount）；
//   - gemini / antigravity：既有 generic 调度支持的平台；
//   - anthropic-协议 / adaptive 多协议账号：要求能解析出 Anthropic 协议上游 base
//     （GetAnthropicProtocolBaseURL 非空，含 api_base_urls.anthropic 与供应商×模式
//     默认端点），避免入池后转发期才发现无端点可用；
//   - OpenAI 兼容账号（openai/grok/国产供应商/opencode_go）：转发由 handler 层
//     委派协议转换链服务，不因账号停留在 Chat Completions 协议而被池排除。
func genericCompositePoolAccountServable(account *Account) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case PlatformAnthropic, PlatformGemini, PlatformAntigravity:
		return true
	}
	if account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol() {
		return strings.TrimSpace(account.GetAnthropicProtocolBaseURL()) != ""
	}
	return account.IsOpenAICompatible()
}

// GenericCompositePoolCountableAccount 报告账号可被 generic count_tokens 链路服务
// （端点转发或本地估算兜底由 handler 分发决定）：anthropic 原生 / anthropic-协议 /
// adaptive / gemini / antigravity 可计数；纯 OpenAI 族（无 Anthropic 协议能力）不算。
// 导出供 handler 层在池请求跳过不可计数账号（重选而非 500）。
func GenericCompositePoolCountableAccount(account *Account) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case PlatformAnthropic, PlatformGemini, PlatformAntigravity:
		return true
	}
	return account.IsAnthropicProtocol() || account.IsAdaptiveAPIProtocol()
}
