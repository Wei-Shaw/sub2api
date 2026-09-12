package service

import (
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpenCode Go 是 OpenCode Zen 的订阅网关：同一 API Key 下按模型分流到
// Responses / Chat Completions / Anthropic Messages 三种原生端点，
// 额度窗口为 rolling(5h) / weekly / monthly。

const (
	openCodeGoUsagePath = "/usage"
	// DefaultOpenCodeGoTestModel is the admin connection-test fallback when
	// the UI does not pick a model. glm-5.3 is a Chat Completions catalog ID.
	DefaultOpenCodeGoTestModel = "glm-5.3"

	openCodeGoProtocolRulesKey         = "protocol_rules"
	maxOpenCodeGoProtocolRules         = 64
	maxOpenCodeGoProtocolPatternLength = 128
)

// DefaultOpenCodeGoModelIDs 是官方文档当前公开的模型 ID 目录，
// 供 /v1/models 在尚未同步上游列表时回退，以及账号白名单预填。
func DefaultOpenCodeGoModelIDs() []string {
	return []string{
		"grok-4.6",
		"gpt-5.6-luna",
		"glm-5.3-flash",
		"glm-5.3",
		"glm-5.2",
		"glm-5.1",
		"kimi-k3",
		"kimi-k2.7-code",
		"kimi-k2.6",
		"longcat-2.0",
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"deepseek-v4-flash-vision-exp",
		"mimo-v2.5",
		"mimo-v2.5-pro",
		"minimax-m3",
		"minimax-m2.7",
		"minimax-m2.5",
		"muse-spark-1.3-contributor",
		"muse-spark-1.2-contributor",
		"qwen3.8-max",
		"qwen3.8-flash",
		"qwen3.7-max",
		"qwen3.7-plus",
		"qwen3.6-plus",
		"hy4-preview",
		"hy3",
		"omen-alpha",
	}
}

func normalizeOpenCodeGoModelID(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"opencode-go/", "opencode_go/", "opencode/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	return model
}

// OpenCodeGoProtocolRule is one model-pattern → native protocol mapping.
// Pattern is an exact ID or a suffix glob (foo* / *). First match wins.
type OpenCodeGoProtocolRule struct {
	Pattern  string `json:"pattern"`
	Protocol string `json:"protocol"`
}

// DefaultOpenCodeGoProtocolRules is the built-in adaptive routing table for Go.
func DefaultOpenCodeGoProtocolRules() []OpenCodeGoProtocolRule {
	return []OpenCodeGoProtocolRule{
		{Pattern: "grok-*", Protocol: APIProtocolResponses},
		{Pattern: "gpt-*", Protocol: APIProtocolResponses},
		{Pattern: "muse-spark-*", Protocol: APIProtocolResponses},
		{Pattern: "minimax-*", Protocol: APIProtocolAnthropic},
		{Pattern: "qwen*", Protocol: APIProtocolAnthropic},
	}
}

// DefaultOpenCodeZenProtocolRules 对齐 https://opencode.ai/docs/zen/ 端点表：
// GPT/Grok/Muse Spark → Responses，Claude/Qwen → Anthropic，其余 Chat Completions。
func DefaultOpenCodeZenProtocolRules() []OpenCodeGoProtocolRule {
	return []OpenCodeGoProtocolRule{
		{Pattern: "grok-*", Protocol: APIProtocolResponses},
		{Pattern: "gpt-*", Protocol: APIProtocolResponses},
		{Pattern: "muse-spark-*", Protocol: APIProtocolResponses},
		{Pattern: "claude-*", Protocol: APIProtocolAnthropic},
		{Pattern: "qwen*", Protocol: APIProtocolAnthropic},
	}
}

func defaultOpenCodeProtocolRules(mode string) []OpenCodeGoProtocolRule {
	if mode == AccountModeZen {
		return DefaultOpenCodeZenProtocolRules()
	}
	return DefaultOpenCodeGoProtocolRules()
}

func isNativeOpenCodeGoProtocol(protocol string) bool {
	switch protocol {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return true
	default:
		return false
	}
}

func openCodeGoPatternMatches(pattern, model string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	model = normalizeOpenCodeGoModelID(model)
	if pattern == "" || model == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return model == pattern
}

func matchOpenCodeGoProtocolRules(model string, rules []OpenCodeGoProtocolRule) string {
	for _, rule := range rules {
		if !isNativeOpenCodeGoProtocol(rule.Protocol) {
			continue
		}
		if openCodeGoPatternMatches(rule.Pattern, model) {
			return rule.Protocol
		}
	}
	return APIProtocolChatCompletions
}

// OpenCodeGoModelProtocol 返回内置默认表下模型对应的原生上游协议。
// 未命中时为 Chat Completions。账号自定义 protocol_rules 见
// ResolveOpenCodeGoUpstreamProtocol。
func OpenCodeGoModelProtocol(model string) string {
	return matchOpenCodeGoProtocolRules(model, DefaultOpenCodeGoProtocolRules())
}

func (a *Account) openCodeGoProtocolRules() ([]OpenCodeGoProtocolRule, bool) {
	if a == nil || a.Credentials == nil {
		return nil, false
	}
	raw, ok := a.Credentials[openCodeGoProtocolRulesKey]
	if !ok || raw == nil {
		return nil, false
	}
	rules, err := parseOpenCodeGoProtocolRules(raw)
	if err != nil {
		return nil, false
	}
	return rules, true
}

func parseOpenCodeGoProtocolRules(raw any) ([]OpenCodeGoProtocolRule, error) {
	items, err := openCodeGoProtocolRuleItems(raw)
	if err != nil {
		return nil, err
	}
	if len(items) > maxOpenCodeGoProtocolRules {
		return nil, fmt.Errorf("protocol_rules supports at most %d entries", maxOpenCodeGoProtocolRules)
	}
	rules := make([]OpenCodeGoProtocolRule, 0, len(items))
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("protocol_rules[%d] must be an object", i)
		}
		pattern, _ := entry["pattern"].(string)
		protocol, _ := entry["protocol"].(string)
		pattern, err := normalizeOpenCodeGoProtocolPattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("protocol_rules[%d]: %w", i, err)
		}
		protocol = strings.TrimSpace(protocol)
		if !isNativeOpenCodeGoProtocol(protocol) {
			return nil, fmt.Errorf("protocol_rules[%d]: protocol must be chat_completions, anthropic, or responses", i)
		}
		rules = append(rules, OpenCodeGoProtocolRule{Pattern: pattern, Protocol: protocol})
	}
	return rules, nil
}

func openCodeGoProtocolRuleItems(raw any) ([]any, error) {
	switch items := raw.(type) {
	case []any:
		return items, nil
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("protocol_rules must be an array")
	}
}

func normalizeOpenCodeGoProtocolPattern(pattern string) (string, error) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if len(pattern) > maxOpenCodeGoProtocolPatternLength {
		return "", fmt.Errorf("pattern is too long")
	}
	if strings.ContainsAny(pattern, " \t") {
		return "", fmt.Errorf("pattern must not contain whitespace")
	}
	star := strings.Count(pattern, "*")
	if star > 1 || (star == 1 && !strings.HasSuffix(pattern, "*")) {
		return "", fmt.Errorf("pattern may use a single trailing * wildcard")
	}
	return pattern, nil
}

// NormalizeOpenCodeGoProtocolRulesCredentials 校验并原地规范化 credentials.protocol_rules。
// 未携带该字段时为 no-op，使旧账号继续使用内置默认表。
func NormalizeOpenCodeGoProtocolRulesCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, ok := credentials[openCodeGoProtocolRulesKey]
	if !ok || raw == nil {
		return nil
	}
	rules, err := parseOpenCodeGoProtocolRules(raw)
	if err != nil {
		return infraerrors.New(http.StatusBadRequest, "INVALID_OPENCODE_GO_PROTOCOL_RULES", err.Error())
	}
	encoded := make([]any, 0, len(rules))
	for _, rule := range rules {
		encoded = append(encoded, map[string]any{
			"pattern":  rule.Pattern,
			"protocol": rule.Protocol,
		})
	}
	credentials[openCodeGoProtocolRulesKey] = encoded
	return nil
}

func (a *Account) IsOpenCodeGo() bool {
	return a != nil && a.Platform == PlatformOpenCodeGo
}

// GetOpenCodeAccountMode 返回 OpenCode 账号类型。未设置时按 Go 处理，兼容已有账号。
func (a *Account) GetOpenCodeAccountMode() string {
	if a == nil || !a.IsOpenCodeGo() {
		return ""
	}
	if strings.TrimSpace(a.GetCredential("account_mode")) == AccountModeZen {
		return AccountModeZen
	}
	return AccountModeGo
}

func (a *Account) IsOpenCodeZen() bool {
	return a.GetOpenCodeAccountMode() == AccountModeZen
}

func (a *Account) IsOpenCodeGoPlan() bool {
	return a.GetOpenCodeAccountMode() == AccountModeGo
}

func (a *Account) openCodeDefaultChatBaseURL() string {
	if a.IsOpenCodeZen() {
		return DefaultOpenCodeZenBaseURL
	}
	return DefaultOpenCodeGoBaseURL
}

func (a *Account) openCodeDefaultAnthropicBaseURL() string {
	if a.IsOpenCodeZen() {
		return DefaultOpenCodeZenAnthropicBaseURL
	}
	return DefaultOpenCodeGoAnthropicBaseURL
}

func (a *Account) IsMultiProtocolAPIKey() bool {
	return a != nil && IsMultiProtocolAPIKeyProvider(a.Platform)
}

// openCodeGoNativeProtocol 返回 OpenCode Go 实际上游协议。
// 规则未命中、空值或未知协议一律兜底 Chat Completions，避免落入 Responses 转换链。
func openCodeGoNativeProtocol(account *Account, model string) string {
	if account == nil {
		return APIProtocolChatCompletions
	}
	switch proto := account.ResolveOpenCodeGoUpstreamProtocol(model); proto {
	case APIProtocolAnthropic, APIProtocolResponses:
		return proto
	default:
		return APIProtocolChatCompletions
	}
}

// ResolveOpenCodeGoUpstreamProtocol 按账号协议配置与模型规则决定上游协议。
// 显式 pinned 协议优先；adaptive（默认）先走 credentials.protocol_rules，
// 未配置时回落内置默认表；已配置但未命中则走 Chat Completions。
func (a *Account) ResolveOpenCodeGoUpstreamProtocol(model string) string {
	if a == nil || !a.IsOpenCodeGo() {
		return ""
	}
	switch a.GetAPIProtocol() {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return a.GetAPIProtocol()
	default:
		if rules, present := a.openCodeGoProtocolRules(); present {
			return matchOpenCodeGoProtocolRules(model, rules)
		}
		return matchOpenCodeGoProtocolRules(model, defaultOpenCodeProtocolRules(a.GetOpenCodeAccountMode()))
	}
}

func openCodeGoQuotaURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultOpenCodeGoBaseURL
	}
	return base + openCodeGoUsagePath
}
