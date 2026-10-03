package service

import (
	"fmt"
	"net/http"
	"slices"
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

	protocolRulesCredentialKey   = "protocol_rules"
	maxProtocolRules             = 64
	maxProtocolRulePatternLength = 128
)

// DefaultOpenCodeGoModelIDs 是官方文档当前公开的模型 ID 目录，
// 供 /v1/models 在尚未同步上游列表时回退，以及账号白名单预填。
func DefaultOpenCodeGoModelIDs() []string {
	return []string{
		"grok-4.7",
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

// ProtocolRule is one model-pattern → native protocol mapping.
// Pattern is an exact ID or a suffix glob (foo* / *). First match wins.
type ProtocolRule struct {
	Pattern  string `json:"pattern"`
	Protocol string `json:"protocol"`
	// Protocols 是命中模型支持的全部原生协议，首项即 Protocol（首选）；为空表示只支持
	// Protocol。入站协议在其中时同协议直通，否则走首选协议（见 resolveModelRoutedProtocolFor）。
	Protocols []string `json:"protocols,omitempty"`
}

// normalizeProtocolSet 返回以 preferred 开头、去重且只含原生协议的协议集合。
func normalizeProtocolSet(preferred string, protocols []string) []string {
	set := []string{preferred}
	for _, protocol := range protocols {
		if !isNativeUpstreamProtocol(protocol) || slices.Contains(set, protocol) {
			continue
		}
		set = append(set, protocol)
	}
	return set
}

// DefaultOpenCodeGoProtocolRules is the built-in adaptive routing table for Go.
func DefaultOpenCodeGoProtocolRules() []ProtocolRule {
	return []ProtocolRule{
		{Pattern: "grok-*", Protocol: APIProtocolResponses},
		{Pattern: "gpt-*", Protocol: APIProtocolResponses},
		{Pattern: "muse-spark-*", Protocol: APIProtocolResponses},
		{Pattern: "minimax-*", Protocol: APIProtocolAnthropic},
		{Pattern: "qwen*", Protocol: APIProtocolAnthropic},
	}
}

// DefaultOpenCodeZenProtocolRules 对齐 https://opencode.ai/docs/zen/ 端点表：
// GPT/Grok/Muse Spark → Responses，Claude/Qwen → Anthropic，其余 Chat Completions。
func DefaultOpenCodeZenProtocolRules() []ProtocolRule {
	return []ProtocolRule{
		{Pattern: "grok-*", Protocol: APIProtocolResponses},
		{Pattern: "gpt-*", Protocol: APIProtocolResponses},
		{Pattern: "muse-spark-*", Protocol: APIProtocolResponses},
		{Pattern: "claude-*", Protocol: APIProtocolAnthropic},
		{Pattern: "qwen*", Protocol: APIProtocolAnthropic},
	}
}

func isNativeUpstreamProtocol(protocol string) bool {
	switch protocol {
	case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return true
	default:
		return false
	}
}

func protocolRulePatternMatches(pattern, model string) bool {
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

// matchProtocolRules 返回首条命中规则的首选协议；未命中时为 Chat Completions。
func matchProtocolRules(model string, rules []ProtocolRule) string {
	if set, ok := matchProtocolRuleSet(model, rules); ok {
		return set[0]
	}
	return APIProtocolChatCompletions
}

// matchProtocolRuleSet 返回首条命中规则支持的协议集合（首项为首选）；未命中时 ok=false。
func matchProtocolRuleSet(model string, rules []ProtocolRule) ([]string, bool) {
	for _, rule := range rules {
		if !isNativeUpstreamProtocol(rule.Protocol) {
			continue
		}
		if protocolRulePatternMatches(rule.Pattern, model) {
			return normalizeProtocolSet(rule.Protocol, rule.Protocols), true
		}
	}
	return nil, false
}

func (a *Account) configuredProtocolRules() ([]ProtocolRule, bool) {
	if a == nil || a.Credentials == nil {
		return nil, false
	}
	raw, ok := a.Credentials[protocolRulesCredentialKey]
	if !ok || raw == nil {
		return nil, false
	}
	rules, err := parseProtocolRules(raw)
	if err != nil {
		return nil, false
	}
	return rules, true
}

func parseProtocolRules(raw any) ([]ProtocolRule, error) {
	items, err := protocolRuleItems(raw)
	if err != nil {
		return nil, err
	}
	if len(items) > maxProtocolRules {
		return nil, fmt.Errorf("protocol_rules supports at most %d entries", maxProtocolRules)
	}
	rules := make([]ProtocolRule, 0, len(items))
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("protocol_rules[%d] must be an object", i)
		}
		pattern, _ := entry["pattern"].(string)
		protocol, _ := entry["protocol"].(string)
		pattern, err := normalizeProtocolRulePattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("protocol_rules[%d]: %w", i, err)
		}
		protocols, err := protocolRuleProtocols(entry["protocols"])
		if err != nil {
			return nil, fmt.Errorf("protocol_rules[%d]: %w", i, err)
		}
		protocol = strings.TrimSpace(protocol)
		if protocol == "" && len(protocols) > 0 {
			protocol = protocols[0]
		}
		if !isNativeUpstreamProtocol(protocol) {
			return nil, fmt.Errorf("protocol_rules[%d]: protocol must be chat_completions, anthropic, or responses", i)
		}
		rule := ProtocolRule{Pattern: pattern, Protocol: protocol}
		if set := normalizeProtocolSet(protocol, protocols); len(set) > 1 {
			rule.Protocols = set
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// protocolRuleProtocols 解析规则的 protocols 列表（可缺省）；其中每一项都须为原生协议。
func protocolRuleProtocols(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var items []any
	switch values := raw.(type) {
	case []any:
		items = values
	case []string:
		for _, value := range values {
			items = append(items, value)
		}
	default:
		return nil, fmt.Errorf("protocols must be an array")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		protocol, _ := item.(string)
		protocol = strings.TrimSpace(protocol)
		if !isNativeUpstreamProtocol(protocol) {
			return nil, fmt.Errorf("protocols must only contain chat_completions, anthropic, or responses")
		}
		out = append(out, protocol)
	}
	return out, nil
}

func protocolRuleItems(raw any) ([]any, error) {
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

func normalizeProtocolRulePattern(pattern string) (string, error) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if len(pattern) > maxProtocolRulePatternLength {
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

// NormalizeProtocolRulesCredentials 校验并原地规范化 credentials.protocol_rules。
// 未携带该字段时为 no-op，使旧账号继续使用内置默认表。
func NormalizeProtocolRulesCredentials(credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	raw, ok := credentials[protocolRulesCredentialKey]
	if !ok || raw == nil {
		return nil
	}
	rules, err := parseProtocolRules(raw)
	if err != nil {
		return infraerrors.New(http.StatusBadRequest, "INVALID_OPENCODE_GO_PROTOCOL_RULES", err.Error())
	}
	encoded := make([]any, 0, len(rules))
	for _, rule := range rules {
		item := map[string]any{
			"pattern":  rule.Pattern,
			"protocol": rule.Protocol,
		}
		if len(rule.Protocols) > 1 {
			protocols := make([]any, 0, len(rule.Protocols))
			for _, protocol := range rule.Protocols {
				protocols = append(protocols, protocol)
			}
			item["protocols"] = protocols
		}
		encoded = append(encoded, item)
	}
	credentials[protocolRulesCredentialKey] = encoded
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

func (a *Account) IsMultiProtocolAPIKey() bool {
	return a != nil && IsMultiProtocolAPIKeyProvider(a.Platform)
}

// openCodeGoQuotaURL 根据 base_url 解析 OpenCode Go 额度端点。
// /zen/go/v1（Chat 协议默认，DefaultOpenCodeGoBaseURL）与 /zen/go
// （Anthropic 协议默认，DefaultOpenCodeGoAnthropicBaseURL）两种 base 统一
// 剥掉尾部 /v1 后拼回 /v1/usage（实测 /zen/go/usage → 404），协议切换不
// 影响额度探测端点。与 kimiQuotaURL 同一惯例。
func openCodeGoQuotaURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultOpenCodeGoBaseURL
	}
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1" + openCodeGoUsagePath
}
