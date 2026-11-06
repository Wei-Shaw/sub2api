package service

import (
	"net/url"
	"strings"
)

// AccountTypeSupportsQuota 返回账号类型是否支持账号配额（quota_limit 及日/周额度、quota_used 累加、超限停调度）。
// 在 apikey/bedrock 之外额外放开 Vertex service_account；池模式等其它特性仍只看 IsAPIKeyOrBedrock。
func AccountTypeSupportsQuota(accountType string) bool {
	switch strings.ToLower(accountType) {
	case AccountTypeAPIKey, AccountTypeBedrock, AccountTypeServiceAccount:
		return true
	}
	return false
}

// SupportsAccountQuota 返回账号是否支持账号配额，类型集合见 AccountTypeSupportsQuota。
func (a *Account) SupportsAccountQuota() bool {
	return AccountTypeSupportsQuota(a.Type)
}

func (a *Account) IsOpenAI() bool {
	return a.Platform == PlatformOpenAI
}

func (a *Account) IsOpenAILongContextBillingEnabled() bool {
	if a == nil || !a.IsOpenAI() || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra[openAILongContextBillingEnabledKey].(bool)
	return ok && enabled
}

func (a *Account) IsAnthropic() bool {
	return a.Platform == PlatformAnthropic
}

func (a *Account) IsOpenAIOAuth() bool {
	return a.IsOpenAI() && a.Type == AccountTypeOAuth
}

// IsOpenAIOAuthLike reports OpenAI credentials that use the ChatGPT/Codex
// inference protocol. Setup tokens share that forwarding contract but do not
// participate in the refreshable OAuth credential lifecycle.
func (a *Account) IsOpenAIOAuthLike() bool {
	return a != nil && a.IsOpenAI() && (a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken)
}

// UsesOpenAICodexProtocol preserves legacy OpenAI gateway OAuth routing for
// accounts whose platform is implicit, while adding OpenAI SetupToken.
func (a *Account) UsesOpenAICodexProtocol() bool {
	return a != nil && (a.Type == AccountTypeOAuth || a.IsOpenAIOAuthLike())
}

func (a *Account) IsOpenAIChatGPTSubscription() bool {
	if !a.IsOpenAIOAuth() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(a.GetCredential("plan_type"))) {
	case "", "free", "abnormal":
		return false
	default:
		return true
	}
}

func (a *Account) IsOpenAIPersonalAccessToken() bool {
	if !a.IsOpenAIOAuth() {
		return false
	}
	return isOpenAIPersonalAccessTokenAuthMode(a.GetCredential(openAIAuthModeCredentialKey)) ||
		isOpenAIPersonalAccessTokenAuthMode(a.GetCredential(openAIAuthModeLegacyCredentialKey))
}

func (a *Account) IsOpenAIApiKey() bool {
	return a.IsOpenAI() && a.Type == AccountTypeAPIKey
}

// GetOpenAIBaseURL 解析 OpenAI 协议族账号的上游 base_url。
// 适用 openai、国产 OpenAI 兼容供应商（kimi/zhipu/deepseek）与 OpenCode Go；
// grok 走 GetGrokBaseURL，此处对 grok 返回 "" 以保持原有行为。
func (a *Account) GetOpenAIBaseURL() string {
	if !a.IsOpenAI() && !a.IsMultiProtocolAPIKey() {
		return ""
	}
	if a.IsMultiProtocolAPIKey() && a.IsAdaptiveAPIProtocol() {
		if baseURLs, ok := a.Credentials["api_base_urls"].(map[string]any); ok {
			if baseURL, ok := baseURLs[APIProtocolChatCompletions].(string); ok && strings.TrimSpace(baseURL) != "" {
				return strings.TrimSpace(baseURL)
			}
		}
	}
	if a.Type == AccountTypeAPIKey || a.Type == AccountTypeUpstream {
		if baseURL := strings.TrimSpace(a.GetCredential("base_url")); baseURL != "" {
			return baseURL
		}
	}
	// 平台默认 base_url：多协议供应商按 account_mode 查 provider profile。
	if baseURL := a.defaultProviderBaseURL(APIProtocolChatCompletions); baseURL != "" {
		return baseURL
	}
	return "https://api.openai.com"
}

// GetAccountMode 返回国产供应商账号的接入模式（payg / coding）；非国产供应商或未设置时
// 返回空串。存储于 credentials["account_mode"]。
func (a *Account) GetAccountMode() string {
	if a == nil {
		return ""
	}
	mode := strings.TrimSpace(a.GetCredential("account_mode"))
	if mode == AccountModePayG || mode == AccountModeCoding {
		return mode
	}
	return ""
}

// IsCodingPlan 报告账号是否为 Coding Plan 模式（用于滚动用量窗口冷却）。
func (a *Account) IsCodingPlan() bool {
	return a.GetAccountMode() == AccountModeCoding
}

// GetAPIProtocol 返回国产供应商账号的上游 API 协议。存储于
// credentials["api_protocol"]；缺失或与平台不匹配时回退 chat_completions
// （与既有行为完全一致）。responses 协议仅 deepseek / kimi / minimax 支持（官方原生
// Responses 端点，适配 Codex）；zhipu 无此端点。
func (a *Account) GetAPIProtocol() string {
	if a == nil || !a.IsMultiProtocolAPIKey() {
		return APIProtocolChatCompletions
	}
	switch strings.TrimSpace(a.GetCredential("api_protocol")) {
	case APIProtocolAdaptive:
		return APIProtocolAdaptive
	case APIProtocolAnthropic:
		return APIProtocolAnthropic
	case APIProtocolResponses:
		if a.SupportsNativeCNResponses() {
			return APIProtocolResponses
		}
	case APIProtocolChatCompletions:
		return APIProtocolChatCompletions
	}
	// 按模型分流的供应商（多模型聚合平台）未显式配置时默认 adaptive。
	if a.routesByModel() {
		return APIProtocolAdaptive
	}
	return APIProtocolChatCompletions
}

// SupportsNativeCNResponses 报告该国产供应商是否提供原生 Responses 端点。
// DeepSeek 官方为 /responses（无 /v1）；Kimi 按量付费与 Coding Plan 均为
// /v1/responses（moonshot.cn / kimi.com/coding）；MiniMax 为 /v1/responses。
func (a *Account) SupportsNativeCNResponses() bool {
	return a.providerSupportsProtocol(APIProtocolResponses)
}

// UsesNativeCNResponses 报告当前账号是否应按原生 Responses 协议转发
// （显式 responses，或 adaptive 且平台具备原生端点）。
func (a *Account) UsesNativeCNResponses() bool {
	if a == nil || !a.SupportsNativeCNResponses() {
		return false
	}
	switch a.GetAPIProtocol() {
	case APIProtocolResponses, APIProtocolAdaptive:
		return true
	default:
		return false
	}
}

// IsAdaptiveAPIProtocol 报告账号是否按入站协议动态选择供应商原生端点。
func (a *Account) IsAdaptiveAPIProtocol() bool {
	return a.GetAPIProtocol() == APIProtocolAdaptive
}

// GetCNProtocolBaseURL 返回国产供应商指定协议的上游 base URL。
// adaptive 账号优先使用 api_base_urls 中的分协议地址，缺失时按平台和
// account_mode 使用官方默认端点。base_url 继续作为 Chat Completions 地址兼容旧字段。
func (a *Account) GetCNProtocolBaseURL(protocol string) string {
	if a == nil || !a.IsMultiProtocolAPIKey() {
		return ""
	}
	if a.IsAdaptiveAPIProtocol() {
		if baseURLs, ok := a.Credentials["api_base_urls"].(map[string]any); ok {
			if baseURL, ok := baseURLs[protocol].(string); ok && strings.TrimSpace(baseURL) != "" {
				return strings.TrimSpace(baseURL)
			}
		}
		if protocol == APIProtocolChatCompletions {
			if baseURL := strings.TrimSpace(a.GetCredential("base_url")); baseURL != "" {
				return baseURL
			}
		}
	}
	return a.defaultProviderBaseURL(protocol)
}

// IsAnthropicProtocol 报告账号是否以原生 Anthropic 协议接入上游
// （/v1/messages 直通，适配 Claude Code 等客户端）。
func (a *Account) IsAnthropicProtocol() bool {
	return a.GetAPIProtocol() == APIProtocolAnthropic
}

// GetAnthropicProtocolBaseURL 返回 Anthropic 协议账号的上游 base_url
// （上游路径为 {base}/v1/messages）。优先取凭证 base_url，缺失时按
// 供应商 × 接入模式返回默认端点。非 Anthropic 协议账号返回空串。
func (a *Account) GetAnthropicProtocolBaseURL() string {
	if a == nil || (!a.IsAnthropicProtocol() && !a.IsAdaptiveAPIProtocol()) {
		return ""
	}
	if a.IsAdaptiveAPIProtocol() {
		return a.GetCNProtocolBaseURL(APIProtocolAnthropic)
	}
	if a.Type == AccountTypeAPIKey || a.Type == AccountTypeUpstream {
		if baseURL := strings.TrimSpace(a.GetCredential("base_url")); baseURL != "" {
			return baseURL
		}
	}
	return a.defaultProviderBaseURL(APIProtocolAnthropic)
}

// GetOpenAIFormatBaseURL 返回供 OpenAI 格式端点（/v1/models、/v1/chat/completions
// 等）使用的 base。chat_completions / responses 协议下与 GetOpenAIBaseURL
// 一致（凭证 base_url 或平台默认）；anthropic 协议下凭证 base_url 指向 Anthropic
// 端点，不能拿来拼 OpenAI 路径，此时返回该供应商 × 模式的 Chat Completions
// 默认 base（模型同步等协议族共用路径仍可用）。
func (a *Account) GetOpenAIFormatBaseURL() string {
	if a == nil || !a.IsAnthropicProtocol() {
		return a.GetOpenAIBaseURL()
	}
	if baseURL := a.defaultProviderBaseURL(APIProtocolChatCompletions); baseURL != "" {
		return baseURL
	}
	return a.GetOpenAIBaseURL()
}

// GetCNAPIKey 返回国产 OpenAI 兼容供应商账号的 api_key 凭据（kimi/zhipu/deepseek）。
// 与 openai 的 GetOpenAIApiKey 区分：后者仅对 openai 平台返回。
func (a *Account) GetCNAPIKey() string {
	if a == nil || !a.IsMultiProtocolAPIKey() {
		return ""
	}
	return a.GetCredential("api_key")
}

// GetCodingPlanProvider 根据 base_url 识别 Coding Plan 供应商（kimi / zhipu / minimax；
// OpenCode Go 订阅与官方主机上的 Command Code、Cline 账号按平台识别），
// 用于路由到对应的额度查询端点。非 coding 模式或无法识别时返回空串。
// 只认官方域名：自定义中转不得把第三方 Key 发往厂商官方额度端点。
func (a *Account) GetCodingPlanProvider() string {
	if a == nil {
		return ""
	}
	if a.IsOpenCodeGoPlan() {
		return PlatformOpenCodeGo
	}
	if a.IsCommandCode() {
		if a.commandCodeUsageSupported() {
			return PlatformCommandCode
		}
		return ""
	}
	if a.IsCline() {
		if a.clineAccountAPISupported() {
			return PlatformCline
		}
		return ""
	}
	if a.GetAccountMode() != AccountModeCoding {
		return ""
	}
	baseURL := strings.ToLower(a.GetOpenAIBaseURL())
	switch {
	case isKimiCodingBaseURL(a.GetOpenAIBaseURL()):
		return PlatformKimi
	case strings.Contains(baseURL, "bigmodel.cn"), strings.Contains(baseURL, "api.z.ai"):
		return PlatformZhipu
	case strings.Contains(baseURL, "minimax.io"),
		strings.Contains(baseURL, "minimaxi.com"),
		strings.Contains(baseURL, "minimax.com"):
		return PlatformMiniMax
	default:
		return ""
	}
}

func isKimiCodingBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(baseURL, "#") {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if host != "api.kimi.com" && host != "api.kimi.ai" && host != "api.kimi.com:443" && host != "api.kimi.ai:443" {
		return false
	}
	switch parsed.EscapedPath() {
	case "/coding", "/coding/", "/coding/v1", "/coding/v1/":
		return true
	default:
		return false
	}
}
