package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"strings"
	"time"
)

type Account struct {
	ID                      int64
	Name                    string
	Notes                   *string
	Platform                string
	Type                    string
	Credentials             map[string]any
	Extra                   map[string]any
	ProxyID                 *int64
	ProxyFallbackOriginID   *int64
	ProxyFallbackOriginName *string // 仅展示用
	Concurrency             int
	Priority                int
	// RateMultiplier 账号计费倍率（>=0，允许 0 表示该账号计费为 0）。
	// 使用指针用于兼容旧版本调度缓存（Redis）中缺字段的情况：nil 表示按 1.0 处理。
	RateMultiplier     *float64
	LoadFactor         *int // 调度负载因子；nil 表示使用 Concurrency
	Status             string
	ErrorMessage       string
	LastUsedAt         *time.Time
	ExpiresAt          *time.Time
	AutoPauseOnExpired bool
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Schedulable bool

	RateLimitedAt    *time.Time
	RateLimitResetAt *time.Time
	OverloadUntil    *time.Time

	TempUnschedulableUntil  *time.Time
	TempUnschedulableReason string

	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string

	ParentAccountID *int64 // non-nil → 影子账号（不持凭据，透传母账号凭据）
	QuotaDimension  string // 用量维度："" / "global" / "spark"

	Proxy         *Proxy
	AccountGroups []AccountGroup
	GroupIDs      []int64
	Groups        []*Group

	// model_mapping 热路径缓存（非持久化字段）
	modelMappingCache               map[string]string
	modelMappingCacheReady          bool
	modelMappingCacheCredentialsPtr uintptr
	modelMappingCacheRawPtr         uintptr
	modelMappingCacheRawLen         int
	modelMappingCacheRawSig         uint64
	modelMappingCacheRuntimeVersion uint64
	// modelMappingCacheShared 为 true 时缓存来自只读共享母本（见 PrepareSharedSnapshot），
	// 指针、长度与运行时版本匹配即直接复用预解析结果，不计算签名。
	modelMappingCacheShared bool

	// header_overrides 热路径缓存（非持久化字段，同 model_mapping 缓存先例）
	headerOverrideCache               map[string]string
	headerOverrideCacheReady          bool
	headerOverrideCacheCredentialsPtr uintptr
	headerOverrideCacheRawPtr         uintptr
	headerOverrideCacheRawLen         int
	headerOverrideCacheRawSig         uint64
}

type OpenAIEndpointCapability string

const openAILongContextBillingEnabledKey = "openai_long_context_billing_enabled"

const (
	OpenAIEndpointCapabilityChatCompletions OpenAIEndpointCapability = "chat_completions"
	OpenAIEndpointCapabilityEmbeddings      OpenAIEndpointCapability = "embeddings"
	OpenAIEndpointCapabilityAlphaSearch     OpenAIEndpointCapability = "alpha_search"
	OpenAIEndpointCapabilityLive            OpenAIEndpointCapability = "live"
	// OpenAIEndpointCapabilityGrokMediaGeneration keeps image/video generation
	// away from Grok accounts that are explicitly disabled or whose billing
	// entitlement probe was forbidden. Video status lookups intentionally do not
	// require this capability so already-submitted requests remain queryable.
	OpenAIEndpointCapabilityGrokMediaGeneration OpenAIEndpointCapability = "grok_media_generation"
	// OpenAIEndpointCapabilityResponses 表示上游确实提供 /v1/responses 端点。
	// 与其他能力不同：支持状态来自 accounts.extra 的自动探测标记
	// （openai_responses_supported / openai_responses_mode），而非
	// credentials["openai_capabilities"] 配置集。仅用于生图意图的 /v1/responses
	// 调度，避免把请求调度到会在 forward 阶段被降级为 Chat Completions 的账号（#4417）。
	OpenAIEndpointCapabilityResponses OpenAIEndpointCapability = "responses"
)

const openAIEndpointCapabilitiesCredentialKey = "openai_capabilities"

// GrokMediaEligibleExtraKey is an optional per-account override stored in
// accounts.extra. true forces media routing on, false disables it, and an
// absent/null value uses provider observations.
const GrokMediaEligibleExtraKey = "grok_media_eligible"

const (
	OpenAIAuthModePersonalAccessToken = "personalAccessToken"
	openAIAuthModeCredentialKey       = "auth_mode"
	openAIAuthModeLegacyCredentialKey = "openai_auth_mode"
)

func isOpenAIPersonalAccessTokenAuthMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "personalaccesstoken", "personal_access_token":
		return true
	default:
		return false
	}
}

// TempUnschedulableRule.Scope 的取值：命中后隔离的粒度。
const (
	TempUnschedScopeAccount = "account"
	TempUnschedScopeModel   = "model"
)

type TempUnschedulableRule struct {
	ErrorCode       int      `json:"error_code"`
	Keywords        []string `json:"keywords"`
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
	// Scope 决定命中后的隔离粒度：account 暂停整个账号，model 只冷却本次请求的模型。
	// 为空时模型已知且非 401 走模型级，否则账号级。
	Scope string `json:"scope,omitempty"`
}

func (a *Account) IsActive() bool {
	return a.Status == StatusActive
}

// IsSyntheticUITest reports whether the account belongs to an isolated UI load-test
// dataset. Production accounts never receive this marker. It lets the dedicated
// test instance exercise interactive quota and connection-test controls without
// sending fake credentials to an upstream provider.
func (a *Account) IsSyntheticUITest() bool {
	if a == nil || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra["synthetic_ui_test"].(bool)
	return ok && enabled
}

// BillingRateMultiplier 返回账号计费倍率。
// - nil 表示未配置/旧缓存缺字段，按 1.0 处理
// - 允许 0，表示该账号计费为 0
// - 负数属于非法数据，出于安全考虑按 1.0 处理
func (a *Account) BillingRateMultiplier() float64 {
	if a == nil || a.RateMultiplier == nil {
		return 1.0
	}
	if *a.RateMultiplier < 0 {
		return 1.0
	}
	return *a.RateMultiplier
}

func (a *Account) EffectiveLoadFactor() int {
	if a == nil {
		return 1
	}
	if a.LoadFactor != nil && *a.LoadFactor > 0 {
		return *a.LoadFactor
	}
	if a.Concurrency > 0 {
		return a.Concurrency
	}
	return 1
}

func (a *Account) IsSchedulable() bool {
	if !a.IsActive() || !a.Schedulable {
		return false
	}
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return false
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return false
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
	}
	if a.SupportsAccountQuota() && a.IsQuotaExceeded() {
		return false
	}
	return true
}

// IsCredentialUsableForShadow 报告本账号(作为某 spark 影子的母账号)的凭据/传输是否可被影子透传使用。
//
// 检查「凭据/账号/传输可用性」:
//   - 账号 active(非禁用/删除);
//   - OAuth token 未过期(AutoPauseOnExpired+ExpiresAt);
//   - 未处于 TempUnschedulableUntil 冷却期 —— 对 OpenAI 账号该字段由 401 鉴权失败 /
//     token 刷新耗尽 / transport·proxy 故障写入(ratelimit/token_refresh/upstream_transport),
//     都代表**共享凭据或传输通道坏死**;影子共享母 token+proxy,故母处于该冷却期时影子也不可用。
//
// **刻意排除** global 维度的限流/过载窗口(RateLimitResetAt / OverloadUntil)与母账号自身的
// 手动 Schedulable 开关:spark 影子拥有独立 spark 配额窗口,母账号 global 429(走 RateLimitResetAt)
// 不应连坐 spark(否则重新耦合影子架构本应解耦的两条 429 道)。nil receiver 返回 false。
func (a *Account) IsCredentialUsableForShadow() bool {
	if a == nil || !a.IsActive() {
		return false
	}
	now := time.Now()
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return false
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return false
	}
	return true
}

func (a *Account) IsRateLimited() bool {
	if a.RateLimitResetAt == nil {
		return false
	}
	return time.Now().Before(*a.RateLimitResetAt)
}

func (a *Account) IsOverloaded() bool {
	if a.OverloadUntil == nil {
		return false
	}
	return time.Now().Before(*a.OverloadUntil)
}

func (a *Account) IsOAuth() bool {
	return a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken
}

// IsPrivacySet 检查账号的 privacy 是否已成功设置。
// OpenAI: privacy_mode == "training_off"
// Antigravity: privacy_mode == "privacy_set"
// 其他平台: 无 privacy 概念，始终返回 true
func (a *Account) IsPrivacySet() bool {
	switch a.Platform {
	case PlatformOpenAI:
		return a.getExtraString("privacy_mode") == PrivacyModeTrainingOff
	case PlatformAntigravity:
		return a.getExtraString("privacy_mode") == AntigravityPrivacySet
	default:
		return true
	}
}

func (a *Account) IsGemini() bool {
	return a.Platform == PlatformGemini
}

func (a *Account) IsGrok() bool {
	return a.Platform == PlatformGrok
}

func (a *Account) IsTypeSafe() bool {
	return a != nil && a.Platform == PlatformTypeSafe
}

func (a *Account) IsGrokOAuth() bool {
	return a.IsGrok() && a.Type == AccountTypeOAuth
}

// IsKimi / IsZhipu / IsDeepseek 标识国产 OpenAI 兼容供应商账号。
func (a *Account) IsKimi() bool {
	return a.Platform == PlatformKimi
}

func (a *Account) IsZhipu() bool {
	return a.Platform == PlatformZhipu
}

func (a *Account) IsDeepseek() bool {
	return a.Platform == PlatformDeepseek
}

func (a *Account) IsMiniMax() bool {
	return a.Platform == PlatformMiniMax
}

// IsCNProvider 报告是否为国产 OpenAI 兼容供应商（kimi/zhipu/deepseek/minimax）。
func (a *Account) IsCNProvider() bool {
	return a != nil && IsCNProvider(a.Platform)
}

// IsOpenAICompatible 报告账号是否走 OpenAI 网关（OpenAI 协议族）。
// openai/grok 原生走 OpenAI 网关；国产供应商同为 OpenAI Chat Completions
// 兼容上游，也经 OpenAI 网关转发。OpenCode 同样经 OpenAI 网关按模型分流。
func (a *Account) IsOpenAICompatible() bool {
	return a != nil && domain.UsesOpenAIGateway(a.Platform)
}

func (a *Account) GeminiOAuthType() string {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return ""
	}
	oauthType := strings.TrimSpace(a.GetCredential("oauth_type"))
	if oauthType == "" && strings.TrimSpace(a.GetCredential("project_id")) != "" {
		return "code_assist"
	}
	return oauthType
}

func (a *Account) GeminiTierID() string {
	tierID := strings.TrimSpace(a.GetCredential("tier_id"))
	return tierID
}

func (a *Account) IsGeminiCodeAssist() bool {
	if a.Platform != PlatformGemini || a.Type != AccountTypeOAuth {
		return false
	}
	oauthType := a.GeminiOAuthType()
	if oauthType == "" {
		return strings.TrimSpace(a.GetCredential("project_id")) != ""
	}
	return oauthType == "code_assist"
}

// IsGeminiGoogleOne reports whether this account uses the legacy consumer
// Gemini CLI / Code Assist OAuth channel.
func (a *Account) IsGeminiGoogleOne() bool {
	return a.Platform == PlatformGemini && a.Type == AccountTypeOAuth && a.GeminiOAuthType() == "google_one"
}

// IsGeminiThirdPartyAPIKey reports whether a Gemini API Key account points at a
// non-Google base URL. Such upstreams carry no AI Studio tier or daily quota.
func (a *Account) IsGeminiThirdPartyAPIKey() bool {
	return a != nil && a.Platform == PlatformGemini && a.Type == AccountTypeAPIKey &&
		!isOfficialGeminiAPIBaseURL(a.GetCredential("base_url"))
}

func (a *Account) CanGetUsage() bool {
	return a.Type == AccountTypeOAuth
}
