package service

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// resolveUpstreamProtocol 决定 OpenAI 网关本次请求发往上游的协议
// （chat_completions / responses / anthropic），是 /v1/responses、
// /v1/chat/completions、/v1/messages 三个入站入口共用的唯一判定。
// 只做判断，不改写请求体、不发请求；各入口再按「入站 × 上游」选择转换路径。
//
// inbound 为入站协议；/v1/chat/completions 收到 Responses 形状请求体时按
// responses 计。model 仅按模型分流的账号使用，取 upstreamRoutingModel 的结果。
// catalog 为上游模型目录给出的该模型支持的协议（见 model_protocol_catalog.go），未知时为 nil。
//
// 优先级：
//  1. 按模型分流的供应商（多模型聚合平台，如 OpenCode、Command Code）：显式
//     api_protocol 直接生效；否则先得到该模型支持的协议集合（见 modelProtocolSet），
//     入站协议在集合中时同协议直通，不在时走集合首项（首选协议）。
//  2. adaptive（按入站协议分流）：供应商提供入站协议的原生端点则走同协议，
//     否则回落 Chat Completions。
//  3. 显式 anthropic 协议 → Anthropic。
//  4. shouldForwardOpenAIResponsesViaRawChatCompletions（显式 chat_completions、
//     探针确认不支持 Responses 等）→ Chat Completions。
//  5. 其余 → Responses。
//
// Grok 在 Responses / Chat Completions 入口有专属链路，须在调用本函数前分流。
func resolveUpstreamProtocol(account *Account, inbound, model string, catalog []string) string {
	if account.routesByModel() {
		return account.resolveModelRoutedProtocolFor(model, inbound, catalog)
	}
	if account.IsAdaptiveAPIProtocol() {
		if account.providerSupportsProtocol(inbound) {
			return inbound
		}
		return APIProtocolChatCompletions
	}
	if account.IsAnthropicProtocol() {
		return APIProtocolAnthropic
	}
	if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		return APIProtocolChatCompletions
	}
	return APIProtocolResponses
}

// resolveModelRoutedProtocol 返回按模型分流账号对该模型的首选上游协议（不考虑入站
// 协议与上游模型目录），供连接测试等没有入站协议的场景使用。
func (a *Account) resolveModelRoutedProtocol(model string) string {
	return a.resolveModelRoutedProtocolFor(model, "", nil)
}

// resolveModelRoutedProtocolFor 按模型名解析按模型分流账号的上游协议：显式 pinned
// 协议直接生效；否则取该模型支持的协议集合，入站协议在其中时同协议直通（省去一次
// 协议转换），不在时走首选协议。
func (a *Account) resolveModelRoutedProtocolFor(model, inbound string, catalog []string) string {
	switch proto := a.GetAPIProtocol(); proto {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return proto
	}
	set := a.modelProtocolSet(model, catalog)
	if slices.Contains(set, inbound) {
		return inbound
	}
	return set[0]
}

// modelProtocolSet 返回模型支持的上游协议集合（首项为首选），来源依次为：
//  1. 账号 protocol_rules 中命中的规则（管理员显式配置）；
//  2. 上游模型目录（catalog）：首选沿用内置规则对该模型的首选（在集合内时），否则取目录首项；
//  3. profile 内置规则——仅在账号未配置 protocol_rules 时，已配置但未命中视为有意不走内置规则；
//  4. Chat Completions，避免落入 Responses 转换链。
func (a *Account) modelProtocolSet(model string, catalog []string) []string {
	rules, configured := a.configuredProtocolRules()
	if configured {
		if set, ok := matchProtocolRuleSet(model, rules); ok {
			return set
		}
	}
	profileRules := a.providerProfile().Endpoints(a.GetCredential("account_mode")).ProtocolRules
	if set := a.supportedCatalogProtocols(catalog); len(set) > 0 {
		preferred := matchProtocolRules(model, profileRules)
		if slices.Contains(set, preferred) {
			return normalizeProtocolSet(preferred, set)
		}
		return set
	}
	if !configured {
		if set, ok := matchProtocolRuleSet(model, profileRules); ok {
			return set
		}
	}
	return []string{APIProtocolChatCompletions}
}

// supportedCatalogProtocols 过滤出账号接入模式确有原生端点的目录协议。
func (a *Account) supportedCatalogProtocols(catalog []string) []string {
	out := make([]string, 0, len(catalog))
	for _, protocol := range catalog {
		if isNativeUpstreamProtocol(protocol) && a.providerSupportsProtocol(protocol) && !slices.Contains(out, protocol) {
			out = append(out, protocol)
		}
	}
	return out
}

// upstreamRoutingModel 返回参与协议分流的模型名：仅按模型分流的账号需要，按账号
// 模型映射与上游归一后的名字匹配规则；其余账号的分流与模型无关，返回空串。
func upstreamRoutingModel(account *Account, body []byte, defaultMappedModel string) string {
	if !account.routesByModel() {
		return ""
	}
	return resolveMappedUpstreamModel(account, body, defaultMappedModel)
}

// convertResponsesShapedChatBody 把发到 /v1/chat/completions 的 Responses 形状
// 请求体（Cursor 兼容）转换为 Chat Completions 请求体。
func (s *OpenAIGatewayService) convertResponsesShapedChatBody(body []byte) ([]byte, error) {
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses-shaped chat completions request: %w", err)
	}
	chatReq, err := apicompat.ResponsesToChatCompletionsRequestWithOptions(
		&responsesReq,
		&apicompat.ResponsesToChatOptions{ReasoningContentByID: s.reasoningContentByID},
	)
	if err != nil {
		return nil, fmt.Errorf("convert responses-shaped chat completions request: %w", err)
	}
	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal converted chat completions request: %w", err)
	}
	return chatBody, nil
}
