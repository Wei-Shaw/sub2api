package service

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type OpenAIMessagesDispatchModelConfig = domain.OpenAIMessagesDispatchModelConfig
type GroupModelsListConfig = domain.GroupModelsListConfig

type Group struct {
	ID             int64
	Name           string
	Description    string
	Platform       string
	RateMultiplier float64
	IsExclusive    bool
	Status         string
	Hydrated       bool // indicates the group was loaded from a trusted repository source

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int

	// 图片生成计费配置（antigravity 和 gemini 平台使用）
	AllowImageGeneration bool
	ImageRateIndependent bool
	ImageRateMultiplier  float64
	ImagePrice1K         *float64
	ImagePrice2K         *float64
	ImagePrice4K         *float64

	// Claude Code 客户端限制
	ClaudeCodeOnly  bool
	FallbackGroupID *int64
	// 无效请求兜底分组（仅 anthropic 平台使用）
	FallbackGroupIDOnInvalidRequest *int64

	// 模型路由配置
	// key: 模型匹配模式（支持 * 通配符，如 "claude-opus-*"）
	// value: 优先账号 ID 列表
	ModelRouting        map[string][]int64
	ModelRoutingEnabled bool

	// MCP XML 协议注入开关（仅 antigravity 平台使用）
	MCPXMLInject bool

	// 支持的模型系列（仅 antigravity 平台使用）
	// 可选值: claude, gemini_text, gemini_image
	SupportedModelScopes []string

	// 分组排序
	SortOrder int

	// OpenAI Messages 调度配置（仅 openai 平台使用）
	AllowMessagesDispatch       bool
	RequireOAuthOnly            bool // 仅允许非 apikey 类型账号关联（OpenAI/Antigravity/Anthropic/Gemini）
	RequirePrivacySet           bool // 调度时仅允许 privacy 已成功设置的账号（OpenAI/Antigravity/Anthropic/Gemini）
	DefaultMappedModel          string
	MessagesDispatchModelConfig OpenAIMessagesDispatchModelConfig
	ModelsListConfig            GroupModelsListConfig

	// RPMLimit 分组级每分钟请求数上限（0 = 不限制）。
	// 一旦设置即接管该分组用户的限流（覆盖用户级 rpm_limit），可被 user-group rpm_override 进一步覆盖。
	RPMLimit int

	// Kiro 模拟缓存配置（仅 Kiro 平台生效）。
	KiroCacheEmulationEnabled   bool
	KiroAutoStickyEnabled       bool
	KiroStickySessionTTLSeconds int
	KiroCacheEmulationRatio     float64

	// Kiro 推理 endpoint 模式（仅 platform=kiro 生效）。
	// "q"   = AWS Q (q.{region}.amazonaws.com)，默认，与其它工具共用限流池
	// "krs" = Kiro Runtime Service (runtime.us-east-1.kiro.dev)，独立限流池
	KiroEndpointMode string

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups           []AccountGroup
	AccountCount            int64
	ActiveAccountCount      int64
	RateLimitedAccountCount int64
}

func (g *Group) EffectiveKiroCacheEmulationEnabled() bool {
	return g != nil && g.Platform == PlatformKiro && g.KiroCacheEmulationEnabled && g.EffectiveKiroCacheEmulationRatio() > 0
}

func (g *Group) EffectiveKiroAutoStickyEnabled() bool {
	return g != nil && g.Platform == PlatformKiro && g.KiroAutoStickyEnabled
}

const (
	DefaultKiroStickySessionTTLSeconds = 3600
	MinKiroStickySessionTTLSeconds     = 60
	MaxKiroStickySessionTTLSeconds     = 86400
)

func (g *Group) EffectiveKiroStickySessionTTLSeconds() int {
	if g == nil || g.Platform != PlatformKiro {
		return 0
	}
	return normalizeKiroStickySessionTTLSeconds(g.KiroStickySessionTTLSeconds)
}

func (g *Group) EffectiveKiroStickySessionTTL() time.Duration {
	seconds := g.EffectiveKiroStickySessionTTLSeconds()
	if seconds <= 0 {
		return stickySessionTTL
	}
	return time.Duration(seconds) * time.Second
}

func (g *Group) EffectiveKiroCacheEmulationRatio() float64 {
	if g == nil || g.Platform != PlatformKiro || !g.KiroCacheEmulationEnabled {
		return 0
	}
	return normalizeKiroCacheEmulationRatio(g.KiroCacheEmulationRatio)
}

func normalizeKiroStickySessionTTLSeconds(seconds int) int {
	if seconds <= 0 {
		return DefaultKiroStickySessionTTLSeconds
	}
	if seconds < MinKiroStickySessionTTLSeconds {
		return MinKiroStickySessionTTLSeconds
	}
	if seconds > MaxKiroStickySessionTTLSeconds {
		return MaxKiroStickySessionTTLSeconds
	}
	return seconds
}

func normalizeKiroCacheEmulationRatio(ratio float64) float64 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	if ratio == 0 {
		return 0
	}
	return ratio
}

func normalizeKiroCacheEmulationFields(g *Group) {
	if g == nil {
		return
	}
	if g.Platform != PlatformKiro {
		g.KiroAutoStickyEnabled = false
		g.KiroStickySessionTTLSeconds = 0
		g.KiroCacheEmulationEnabled = false
		g.KiroCacheEmulationRatio = 0
		return
	}
	g.KiroStickySessionTTLSeconds = normalizeKiroStickySessionTTLSeconds(g.KiroStickySessionTTLSeconds)
	if g.KiroCacheEmulationRatio == 0 {
		g.KiroCacheEmulationRatio = 1
	}
	g.KiroCacheEmulationRatio = normalizeKiroCacheEmulationRatio(g.KiroCacheEmulationRatio)
}

// Kiro 推理 endpoint 模式取值。
const (
	KiroEndpointModeQ   = "q"
	KiroEndpointModeKRS = "krs"
)

// EffectiveKiroEndpointMode 返回当前 group 实际使用的 Kiro endpoint 模式。
// 仅当 Platform = kiro 时返回 group 配置；非 kiro 平台、空值或未知字符串兜底返回 "q"。
func (g *Group) EffectiveKiroEndpointMode() string {
	if g == nil || g.Platform != PlatformKiro {
		return KiroEndpointModeQ
	}
	switch g.KiroEndpointMode {
	case KiroEndpointModeKRS:
		return KiroEndpointModeKRS
	default:
		return KiroEndpointModeQ
	}
}

// KiroKRSEnabled 报告该 group 是否启用了 Kiro KRS endpoint。
func (g *Group) KiroKRSEnabled() bool {
	return g.EffectiveKiroEndpointMode() == KiroEndpointModeKRS
}

func normalizeKiroEndpointFields(g *Group) {
	if g == nil {
		return
	}
	if g.Platform != PlatformKiro {
		g.KiroEndpointMode = ""
		return
	}
	switch g.KiroEndpointMode {
	case KiroEndpointModeKRS:
		// 合法值保留
	default:
		g.KiroEndpointMode = KiroEndpointModeQ
	}
}

func NormalizeGroupRuntimeFields(g *Group) {
	normalizeKiroCacheEmulationFields(g)
	normalizeKiroEndpointFields(g)
}

func (g *Group) IsActive() bool {
	return g.Status == StatusActive
}

func (g *Group) IsSubscriptionType() bool {
	return g.SubscriptionType == SubscriptionTypeSubscription
}

func (g *Group) HasDailyLimit() bool {
	return g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
}

func (g *Group) HasWeeklyLimit() bool {
	return g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
}

func (g *Group) HasMonthlyLimit() bool {
	return g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
}

// GetImagePrice 根据 image_size 返回对应的图片生成价格
// 如果分组未配置价格，返回 nil（调用方应使用默认值）
func (g *Group) GetImagePrice(imageSize string) *float64 {
	switch imageSize {
	case "1K":
		return g.ImagePrice1K
	case "2K":
		return g.ImagePrice2K
	case "4K":
		return g.ImagePrice4K
	default:
		// 未知尺寸默认按 2K 计费
		return g.ImagePrice2K
	}
}

// IsGroupContextValid reports whether a group from context has the fields required for routing decisions.
func IsGroupContextValid(group *Group) bool {
	if group == nil {
		return false
	}
	if group.ID <= 0 {
		return false
	}
	if !group.Hydrated {
		return false
	}
	if group.Platform == "" || group.Status == "" {
		return false
	}
	return true
}

// GetRoutingAccountIDs 根据请求模型获取路由账号 ID 列表
// 返回匹配的优先账号 ID 列表，如果没有匹配规则则返回 nil
func (g *Group) GetRoutingAccountIDs(requestedModel string) []int64 {
	if !g.ModelRoutingEnabled || len(g.ModelRouting) == 0 || requestedModel == "" {
		return nil
	}

	// 1. 精确匹配优先
	if accountIDs, ok := g.ModelRouting[requestedModel]; ok && len(accountIDs) > 0 {
		return accountIDs
	}

	// 2. 通配符匹配（前缀匹配）
	for pattern, accountIDs := range g.ModelRouting {
		if matchModelPattern(pattern, requestedModel) && len(accountIDs) > 0 {
			return accountIDs
		}
	}

	return nil
}

// matchModelPattern 检查模型是否匹配模式
// 支持 * 通配符，如 "claude-opus-*" 匹配 "claude-opus-4-20250514"
func matchModelPattern(pattern, model string) bool {
	if pattern == model {
		return true
	}

	// 处理 * 通配符（仅支持末尾通配符）
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}

	return false
}
