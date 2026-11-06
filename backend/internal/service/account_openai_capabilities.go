package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"strings"
	"time"
)

func (a *Account) SupportsOpenAIEndpointCapability(capability OpenAIEndpointCapability) bool {
	if a == nil {
		return false
	}
	if capability == OpenAIEndpointCapabilitySeedance {
		configured, _ := a.openAIEndpointCapabilitySet()
		return configured["seedance"] && a.Platform == PlatformOpenAI && a.Type == AccountTypeAPIKey &&
			strings.TrimSpace(a.GetCredential("base_url")) != ""
	}
	if capability == "" {
		return true
	}
	if !a.IsOpenAICompatible() {
		return false
	}
	if a.IsGrok() {
		switch capability {
		case OpenAIEndpointCapabilityChatCompletions:
			return true
		case OpenAIEndpointCapabilityGrokMediaGeneration:
			eligible, reason := a.GrokMediaGenerationEligibility()
			// Unobserved OAuth accounts remain scheduler candidates only so the
			// request path can run the billing probe before forwarding. The
			// forwarding gate itself fails closed if that probe is unavailable or
			// cannot produce positive paid-entitlement evidence.
			return eligible || reason == "billing_unobserved"
		default:
			return false
		}
	}
	switch capability {
	case OpenAIEndpointCapabilityChatCompletions:
	case OpenAIEndpointCapabilityLive:
		return a.Platform == PlatformOpenAI &&
			a.Type == AccountTypeOAuth &&
			!a.IsOpenAIPersonalAccessToken() &&
			!a.IsOpenAIAgentIdentity()
	case OpenAIEndpointCapabilityResponses:
		// Responses 支持状态由 accounts.extra 的自动探测标记决定，而非
		// credentials 能力集。已探测确认不支持 /v1/responses 的 APIKey 上游
		// 必须排除——否则会在 forward 阶段被静默降级为 Chat Completions，
		// 无法完成生图（#4417）。未探测/OAuth 账号保留旧行为（不排除）。
		if a.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(a.Extra) {
			return false
		}
		// 支持 Responses 的上游同样需具备 chat 能力：复用下方 chat_completions
		// 配置集校验。
		capability = OpenAIEndpointCapabilityChatCompletions
	case OpenAIEndpointCapabilityAlphaSearch:
		// alpha/search 的转发按账号类型分流：OAuth/PAT 走
		// chatgpt.com/backend-api/codex/alpha/search，API key 走
		// {base_url}/v1/alpha/search（见 openAIAlphaSearchURL），两类账号
		// 都可承接独立搜索请求。上游不支持该端点时由转发层 failover 兜底。
		if a.Type != AccountTypeOAuth && a.Type != AccountTypeAPIKey {
			return false
		}
	case OpenAIEndpointCapabilityEmbeddings:
		if a.Type != AccountTypeAPIKey {
			return false
		}
	default:
		return false
	}

	configured, found := a.openAIEndpointCapabilitySet()
	if !found {
		return true
	}
	if capability == OpenAIEndpointCapabilityAlphaSearch && configured[string(OpenAIEndpointCapabilityChatCompletions)] {
		return true
	}
	return configured[string(capability)]
}

// GrokMediaGenerationEligibility reports whether a Grok account may receive
// new image/video generation requests. Explicit evidence of a forbidden or
// free account blocks media, while an incomplete successful billing response
// remains eligible for backwards compatibility. An explicit operator
// override takes precedence over probe data.
func (a *Account) GrokMediaGenerationEligibility() (bool, string) {
	if a == nil || !a.IsGrok() {
		return false, "not_grok"
	}
	if override, ok := grokMediaEligibilityOverride(a.Extra); ok {
		if override {
			return true, "override_enabled"
		}
		return false, "override_disabled"
	}
	if a.Type != AccountTypeOAuth {
		return true, "non_oauth"
	}

	billing, err := grokBillingSnapshotFromExtra(a.Extra)
	if err != nil || billing == nil {
		return false, "billing_unobserved"
	}
	if billing.StatusCode == 403 || billing.WeeklyStatusCode == 403 || billing.MonthlyStatusCode == 403 {
		return false, "billing_forbidden"
	}
	if isKnownGrokFreeAccount(a) {
		return false, "billing_free_tier"
	}
	if !grokBillingHasAuthoritativeQuota(billing) {
		// Billing endpoints can return 200 with an account-specific schema that
		// omits plan/quota fields (for example, some SuperGrok accounts). An
		// incomplete observation is not proof of ineligibility; keep the account
		// routable and expose the reason for diagnostics. Operators can still
		// quarantine a known-bad account with grok_media_eligible=false.
		return true, "billing_inconclusive"
	}
	return true, "eligible"
}

func grokMediaEligibilityOverride(extra map[string]any) (bool, bool) {
	if extra == nil {
		return false, false
	}
	raw, exists := extra[GrokMediaEligibleExtraKey]
	if !exists || raw == nil {
		return false, false
	}
	value, ok := raw.(bool)
	return value, ok
}

func (a *Account) openAIEndpointCapabilitySet() (map[string]bool, bool) {
	if a == nil || a.Credentials == nil {
		return nil, false
	}
	raw, found := a.Credentials[openAIEndpointCapabilitiesCredentialKey]
	if !found || raw == nil {
		return nil, false
	}

	result := make(map[string]bool)
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		result[value] = true
	}

	// 空容器（{} / []）与未配置一致：不限制任何能力。
	// 避免 OAuth 账号因 API 直写/导入/历史数据遗留的空对象而被调度器静默排除（#5530）。
	// 注意：非空但全 false / 类型异常的数据仍视为「已配置且不含能力」，保持原行为。
	switch capabilities := raw.(type) {
	case []any:
		if len(capabilities) == 0 {
			return nil, false
		}
		for _, item := range capabilities {
			if value, ok := item.(string); ok {
				add(value)
			}
		}
	case []string:
		if len(capabilities) == 0 {
			return nil, false
		}
		for _, value := range capabilities {
			add(value)
		}
	case map[string]any:
		if len(capabilities) == 0 {
			return nil, false
		}
		for key, value := range capabilities {
			enabled, ok := value.(bool)
			if ok && enabled {
				add(key)
			}
		}
	case map[string]bool:
		if len(capabilities) == 0 {
			return nil, false
		}
		for key, enabled := range capabilities {
			if enabled {
				add(key)
			}
		}
	}

	return result, true
}

func (a *Account) SupportsOpenAIImageCapability(capability OpenAIImagesCapability) bool {
	if capability == "" {
		return true
	}
	if !a.IsOpenAI() {
		return false
	}
	switch capability {
	case OpenAIImagesCapabilityAPIKey:
		return a.Type == AccountTypeAPIKey
	case OpenAIImagesCapabilityBasic, OpenAIImagesCapabilityNative:
		return a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken || a.Type == AccountTypeAPIKey
	default:
		return true
	}
}

func (a *Account) GetChatGPTUserID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("chatgpt_user_id")
}

func (a *Account) GetOpenAIOrganizationID() string {
	if !a.IsOpenAIOAuth() {
		return ""
	}
	return a.GetCredential("organization_id")
}

func (a *Account) GetOpenAITokenExpiresAt() *time.Time {
	if !a.IsOpenAIOAuth() {
		return nil
	}
	return a.GetCredentialAsTime("expires_at")
}

func (a *Account) IsOpenAITokenExpired() bool {
	expiresAt := a.GetOpenAITokenExpiresAt()
	if expiresAt == nil {
		return false
	}
	return time.Now().Add(60 * time.Second).After(*expiresAt)
}
