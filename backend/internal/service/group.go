package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// GroupImageConfig 图片生成计费配置（封装在 GroupExtra["image_config"] 中）
type GroupImageConfig struct {
	AllowGeneration bool     `json:"allow_image_generation"`
	RateIndependent bool     `json:"image_rate_independent"`
	RateMultiplier  float64  `json:"image_rate_multiplier"`
	Price1K         *float64 `json:"image_price_1k,omitempty"`
	Price2K         *float64 `json:"image_price_2k,omitempty"`
	Price4K         *float64 `json:"image_price_4k,omitempty"`
}

// GroupAnthropicConfig Anthropic 平台专属配置（封装在 GroupExtra["anthropic_config"] 中）
type GroupAnthropicConfig struct {
	ClaudeCodeOnly                  bool               `json:"claude_code_only"`
	FallbackGroupID                 *int64             `json:"fallback_group_id,omitempty"`
	FallbackGroupIDOnInvalidRequest *int64             `json:"fallback_group_id_on_invalid_request,omitempty"`
	ModelRoutingEnabled             bool               `json:"model_routing_enabled"`
	ModelRouting                    map[string][]int64 `json:"model_routing,omitempty"`
}

// GroupOpenAIConfig OpenAI 平台专属配置（封装在 GroupExtra["openai_config"] 中）
type GroupOpenAIConfig struct {
	AllowMessagesDispatch       bool                               `json:"allow_messages_dispatch"`
	DefaultMappedModel          string                             `json:"default_mapped_model,omitempty"`
	MessagesDispatchModelConfig *OpenAIMessagesDispatchModelConfig `json:"messages_dispatch_model_config,omitempty"`
	RequireOAuthOnly            bool                               `json:"require_oauth_only"`
	RequirePrivacySet           bool                               `json:"require_privacy_set"`
}

// GroupAntigravityConfig Antigravity 平台专属配置（封装在 GroupExtra["antigravity_config"] 中）
type GroupAntigravityConfig struct {
	SupportedModelScopes []string `json:"supported_model_scopes,omitempty"`
	MCPXMLInject         bool     `json:"mcp_xml_inject"`
	RequireOAuthOnly     bool     `json:"require_oauth_only"`
	RequirePrivacySet    bool     `json:"require_privacy_set"`
}

// messagesDispatchModelConfigValue 返回值类型的 MessagesDispatchModelConfig（解引用指针，nil 返回零值）。
func (c GroupOpenAIConfig) messagesDispatchModelConfigValue() OpenAIMessagesDispatchModelConfig {
	if c.MessagesDispatchModelConfig != nil {
		return *c.MessagesDispatchModelConfig
	}
	return OpenAIMessagesDispatchModelConfig{}
}

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

	// GroupExtra 插件平台扩展配置（plugin 平台使用）
	GroupExtra map[string]interface{}

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

// IsRequireOAuthOnly 返回分组是否要求仅 OAuth 账号。
// 优先从平台对应的 config extra 读取，fallback 到一级字段。
func (g *Group) IsRequireOAuthOnly() bool {
	switch g.Platform {
	case PlatformOpenAI:
		return g.OpenAIConfig().RequireOAuthOnly
	case PlatformAntigravity:
		return g.AntigravityConfig().RequireOAuthOnly
	default:
		return g.RequireOAuthOnly
	}
}

// IsRequirePrivacySet 返回分组是否要求账号已设置 privacy。
// 优先从平台对应的 config extra 读取，fallback 到一级字段。
func (g *Group) IsRequirePrivacySet() bool {
	switch g.Platform {
	case PlatformOpenAI:
		return g.OpenAIConfig().RequirePrivacySet
	case PlatformAntigravity:
		return g.AntigravityConfig().RequirePrivacySet
	default:
		return g.RequirePrivacySet
	}
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

// ImageConfig 返回图片生成计费配置。
// 优先从 GroupExtra["image_config"] 读取，fallback 到一级字段（兼容旧数据）。
func (g *Group) ImageConfig() GroupImageConfig {
	if raw, ok := g.GroupExtra["image_config"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			var cfg GroupImageConfig
			if json.Unmarshal(data, &cfg) == nil {
				return cfg
			}
		}
	}
	return GroupImageConfig{
		AllowGeneration: g.AllowImageGeneration,
		RateIndependent: g.ImageRateIndependent,
		RateMultiplier:  g.ImageRateMultiplier,
		Price1K:         g.ImagePrice1K,
		Price2K:         g.ImagePrice2K,
		Price4K:         g.ImagePrice4K,
	}
}

// AnthropicConfig 返回 Anthropic 平台专属配置。
// 优先从 GroupExtra["anthropic_config"] 读取，fallback 到一级字段（兼容旧数据）。
func (g *Group) AnthropicConfig() GroupAnthropicConfig {
	if raw, ok := g.GroupExtra["anthropic_config"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			var cfg GroupAnthropicConfig
			if json.Unmarshal(data, &cfg) == nil {
				return cfg
			}
		}
	}
	return GroupAnthropicConfig{
		ClaudeCodeOnly:                  g.ClaudeCodeOnly,
		FallbackGroupID:                 g.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: g.FallbackGroupIDOnInvalidRequest,
		ModelRoutingEnabled:             g.ModelRoutingEnabled,
		ModelRouting:                    g.ModelRouting,
	}
}

// GetImagePrice 根据 image_size 返回对应的图片生成价格
// 如果分组未配置价格，返回 nil（调用方应使用默认值）
func (g *Group) GetImagePrice(imageSize string) *float64 {
	cfg := g.ImageConfig()
	switch imageSize {
	case "1K":
		return cfg.Price1K
	case "2K":
		return cfg.Price2K
	case "4K":
		return cfg.Price4K
	default:
		// 未知尺寸默认按 2K 计费
		return cfg.Price2K
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
	cfg := g.AnthropicConfig()
	if !cfg.ModelRoutingEnabled || len(cfg.ModelRouting) == 0 || requestedModel == "" {
		return nil
	}

	// 1. 精确匹配优先
	if accountIDs, ok := cfg.ModelRouting[requestedModel]; ok && len(accountIDs) > 0 {
		return accountIDs
	}

	// 2. 通配符匹配（前缀匹配）
	for pattern, accountIDs := range cfg.ModelRouting {
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

// OpenAIConfig 返回 OpenAI 平台专属配置。
// 优先从 GroupExtra["openai_config"] 读取，fallback 到一级字段（兼容旧数据）。
func (g *Group) OpenAIConfig() GroupOpenAIConfig {
	if raw, ok := g.GroupExtra["openai_config"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			var cfg GroupOpenAIConfig
			if json.Unmarshal(data, &cfg) == nil {
				return cfg
			}
		}
	}
	return GroupOpenAIConfig{
		AllowMessagesDispatch:       g.AllowMessagesDispatch,
		DefaultMappedModel:          g.DefaultMappedModel,
		MessagesDispatchModelConfig: &g.MessagesDispatchModelConfig,
		RequireOAuthOnly:            g.RequireOAuthOnly,
		RequirePrivacySet:           g.RequirePrivacySet,
	}
}

// AntigravityConfig 返回 Antigravity 平台专属配置。
// 优先从 GroupExtra["antigravity_config"] 读取，fallback 到一级字段（兼容旧数据）。
func (g *Group) AntigravityConfig() GroupAntigravityConfig {
	if raw, ok := g.GroupExtra["antigravity_config"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			var cfg GroupAntigravityConfig
			if json.Unmarshal(data, &cfg) == nil {
				return cfg
			}
		}
	}
	return GroupAntigravityConfig{
		SupportedModelScopes: g.SupportedModelScopes,
		MCPXMLInject:         g.MCPXMLInject,
		RequireOAuthOnly:     g.RequireOAuthOnly,
		RequirePrivacySet:    g.RequirePrivacySet,
	}
}

// mergeOpenAIConfigIntoExtra 将 OpenAI 配置合并到 GroupExtra 的 "openai_config" 键中。
// 如果 extra 为 nil 则创建新 map。
func mergeOpenAIConfigIntoExtra(extra map[string]interface{}, cfg GroupOpenAIConfig) map[string]interface{} {
	if extra == nil {
		extra = make(map[string]interface{})
	}
	extra["openai_config"] = &cfg
	return extra
}

// mergeAntigravityConfigIntoExtra 将 Antigravity 配置合并到 GroupExtra 的 "antigravity_config" 键中。
// 如果 extra 为 nil 则创建新 map。
func mergeAntigravityConfigIntoExtra(extra map[string]interface{}, cfg GroupAntigravityConfig) map[string]interface{} {
	if extra == nil {
		extra = make(map[string]interface{})
	}
	extra["antigravity_config"] = &cfg
	return extra
}

// mergeImageConfigIntoExtra 将图片配置合并到 GroupExtra 的 "image_config" 键中。
// 如果 extra 为 nil 则创建新 map。
func mergeImageConfigIntoExtra(extra map[string]interface{}, cfg GroupImageConfig) map[string]interface{} {
	if extra == nil {
		extra = make(map[string]interface{})
	}
	extra["image_config"] = &cfg
	return extra
}

// mergeAnthropicConfigIntoExtra 将 Anthropic 配置合并到 GroupExtra 的 "anthropic_config" 键中。
// 如果 extra 为 nil 则创建新 map。
func mergeAnthropicConfigIntoExtra(extra map[string]interface{}, cfg GroupAnthropicConfig) map[string]interface{} {
	if extra == nil {
		extra = make(map[string]interface{})
	}
	extra["anthropic_config"] = &cfg
	return extra
}
