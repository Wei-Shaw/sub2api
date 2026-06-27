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

	// ImagePricingMatrix 二维定价矩阵：tier_key -> quality_key -> 单价（USD per image）。
	// 见 spec media-prepay-billing：6 档 × 3 quality。nil/空 map 表示分组未启用矩阵定价。
	ImagePricingMatrix domain.ImagePricingMatrix

	// ImagePreferFal 仅在 platform=openai 分组下生效：
	// true 表示该分组发起的图片请求优先使用 fal 账号承载，openai 账号兜底。
	// false（默认）保持现有 openai 优先、fal 兜底的行为。
	ImagePreferFal bool

	// ImageDecodeSizeOnRsp 仅在 platform=openai 分组下生效：
	// true 表示当回包某张图缺失 size 字段或 size="auto" 时，系统在异步记账阶段对该张图的
	// b64_json 内容做最小代价的头部解码，回填真实分辨率用于 6 档归档计费；URL 模式不解码。
	// false（默认）保持现状默认 2K 档兜底语义。
	ImageDecodeSizeOnRsp bool

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

	CreatedAt time.Time
	UpdatedAt time.Time

	AccountGroups           []AccountGroup
	AccountCount            int64
	ActiveAccountCount      int64
	RateLimitedAccountCount int64
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

// BuildImagePriceConfig 把分组持久化的图片计费配置（旧三档 + 新二维矩阵）
// 打包成 BillingService.CalculateImageCost 期望的 ImagePriceConfig。
//
// rawWidth/rawHeight/quality 由调用方从请求/响应中收集；任意一个不可用时
// 传 0/空字符串即可——计费层会按 spec D5 自动回退到旧三档或默认价。
func (g *Group) BuildImagePriceConfig(rawWidth, rawHeight int, quality string) *ImagePriceConfig {
	if g == nil {
		return nil
	}
	cfg := &ImagePriceConfig{
		Price1K:       g.ImagePrice1K,
		Price2K:       g.ImagePrice2K,
		Price4K:       g.ImagePrice4K,
		PricingMatrix: g.ImagePricingMatrix,
		RawWidth:      rawWidth,
		RawHeight:     rawHeight,
		Quality:       quality,
	}
	return cfg
}

// ImagePreferFalEnabled 仅在 platform=openai 分组上为真才生效。
// 集中校验避免上层调度把非 openai 分组的脏数据当作"prefer_fal=true"误用。
func (g *Group) ImagePreferFalEnabled() bool {
	if g == nil {
		return false
	}
	if !g.ImagePreferFal {
		return false
	}
	return g.Platform == PlatformOpenAI
}

// ImageDecodeSizeOnRspEnabled 仅在 platform=openai 分组上为真才生效。
// 集中校验避免上层把非 openai 分组的脏数据当作 decode_size_on_rsp=true 误触发解码副作用。
func (g *Group) ImageDecodeSizeOnRspEnabled() bool {
	if g == nil {
		return false
	}
	if !g.ImageDecodeSizeOnRsp {
		return false
	}
	return g.Platform == PlatformOpenAI
}
