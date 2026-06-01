package service

//go:generate go run ../cmd/gen-frontend-constants -out ../frontend/src/utils/channelConstants.ts

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/decimalx"
	"github.com/shopspring/decimal"
)

// Status values used on Channel.Status. Mirrors backend/internal/domain
// StatusActive/Disabled so the plugin's data layer is wire-compatible with
// the rest of the system.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Platform identifiers shared with the core. Kept as plain strings so the
// JSON wire format stays identical.
const (
	PlatformAnthropic   = "anthropic"
	PlatformOpenAI      = "openai"
	PlatformGemini      = "gemini"
	PlatformAntigravity = "antigravity"
)

// BillingMode distinguishes how a model is billed.
type BillingMode string

const (
	BillingModeToken      BillingMode = "token"       // 按 token 区间计费
	BillingModePerRequest BillingMode = "per_request" // 按次计费（支持上下文窗口分层）
	BillingModeImage      BillingMode = "image"       // 图片计费（当前按次，预留 token 计费）
)

// IsValid reports whether m is one of the recognised modes (the empty string
// is allowed because it represents "use defaults").
func (m BillingMode) IsValid() bool {
	switch m {
	case BillingModeToken, BillingModePerRequest, BillingModeImage, "":
		return true
	}
	return false
}

const (
	BillingModelSourceRequested     = "requested"
	BillingModelSourceUpstream      = "upstream"
	BillingModelSourceChannelMapped = "channel_mapped"
)

// 以下 List 函数把 service 包内权威的枚举常量集中暴露给上层（handler 注册的
// 自定义 validator、未来 codegen 工具等），避免在 struct tag / 前端 / SDK
// 各处再硬编码字符串字面量。新增 / 删除某个枚举值时只需要修改对应常量与
// 它的 List 函数；handler 启动期 validator 注册会自动跟随。
//
// 返回值统一是 string slice，方便 caller 直接做 slices.Contains / map 查找。

// BillingModes 返回所有合法 billing_mode 值（不含空串：空串只在写入侧由
// pricingRequestToService 默认补成 BillingModeToken，校验层不应放它通过）。
func BillingModes() []string {
	return []string{
		string(BillingModeToken),
		string(BillingModePerRequest),
		string(BillingModeImage),
	}
}

// ChannelStatuses 返回所有合法 channel.status 值。
func ChannelStatuses() []string {
	return []string{StatusActive, StatusDisabled}
}

// Platforms 返回所有合法 platform 标识。
//
// 业务上 platform 字段在 ChannelModelPricingRequest / 相关 DTO 是可选的
// （留空时由后端默认补 PlatformAnthropic），所以本列表也用于"平台是否可识别"
// 的轻量校验，让 handler 启动期 validator 拒绝拼写错误。
func Platforms() []string {
	return []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
	}
}

// BillingModelSources 返回所有合法 billing_model_source 值。
func BillingModelSources() []string {
	return []string{
		BillingModelSourceRequested,
		BillingModelSourceUpstream,
		BillingModelSourceChannelMapped,
	}
}

// Channel is the in-memory shape of a channel record. Pricing and group
// associations live as nested fields rather than separate aggregate roots so
// the cache layer can hand out clones cheaply.
type Channel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string         // "requested", "upstream", or "channel_mapped"
	RestrictModels     bool           // 是否限制模型（仅允许定价列表中的模型）
	Features           string         // 渠道特性描述（JSON 数组），用于支付页面展示
	FeaturesConfig     map[string]any // 渠道功能配置（如 web search emulation）
	// ApplyPricingToAccountStats 控制账号统计费用是否启用基于渠道定价的覆写。
	// 关闭时 account_stats_cost 走默认公式 (total_cost * account_rate_multiplier)。
	// 开启后优先级：自定义规则 (AccountStatsPricingRules) → 客户计费 (totalCost) →
	// host LiteLLM 默认价格 → nil（默认公式）。
	ApplyPricingToAccountStats bool
	CreatedAt                  time.Time
	UpdatedAt                  time.Time

	// 关联的分组 ID 列表
	GroupIDs []int64
	// 模型定价列表（每条含 Platform 字段）
	ModelPricing []ChannelModelPricing
	// 渠道级模型映射（按平台分组：platform → {src→dst}）
	ModelMapping map[string]map[string]string
	// 账号统计自定义定价规则（按 SortOrder 升序遍历，先命中即返回）。
	// 不依赖 ApplyPricingToAccountStats 开关：只要规则匹配就生效。
	AccountStatsPricingRules []AccountStatsPricingRule
}

// ChannelModelPricing is one pricing row inside a channel.
//
// Pricing fields use decimal.NullDecimal so the channel-management plugin
// satisfies the project's "金额必须用 shopspring/decimal" rule (CLAUDE.md
// "支付系统专项"). Valid=false means "price not configured" — semantically
// the same as the pre-T4 *float64 nil. Sites that bridge to JSON / proto /
// the vendored domain snapshot translate via internal/decimalx helpers.
type ChannelModelPricing struct {
	ID               int64
	ChannelID        int64
	Platform         string              // 所属平台（anthropic/openai/gemini/...）
	Models           []string            // 绑定的模型列表
	BillingMode      BillingMode         // 计费模式
	InputPrice       decimal.NullDecimal // 每 token 输入价格（USD）— 向后兼容 flat 定价
	OutputPrice      decimal.NullDecimal // 每 token 输出价格（USD）
	CacheWritePrice  decimal.NullDecimal // 缓存写入价格
	CacheReadPrice   decimal.NullDecimal // 缓存读取价格
	ImageOutputPrice decimal.NullDecimal // 图片输出价格（向后兼容）
	PerRequestPrice  decimal.NullDecimal // 默认按次计费价格（USD）
	Intervals        []PricingInterval   // 区间定价列表
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PricingInterval represents one tiered pricing band (token range, per-request
// tier, or image resolution tier).
type PricingInterval struct {
	ID               int64
	PricingID        int64
	MinTokens        int                 // 区间下界（含）
	MaxTokens        *int                // 区间上界（不含），nil = 无上限
	TierLabel        *string             // 层级标签（按次/图片模式：1K, 2K, 4K, HD 等）
	InputPrice       decimal.NullDecimal // token 模式：每 token 输入价
	OutputPrice      decimal.NullDecimal // token 模式：每 token 输出价
	CacheWritePrice  decimal.NullDecimal // token 模式：缓存写入价
	CacheReadPrice   decimal.NullDecimal // token 模式：缓存读取价
	ImageOutputPrice decimal.NullDecimal // image 模式：图片输出价（与 ChannelModelPricing.ImageOutputPrice 对称）
	PerRequestPrice  decimal.NullDecimal // 按次/图片模式：每次请求价格
	SortOrder        int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountStatsPricingRule 是渠道下的一条「账号统计自定义定价规则」。
// 规则按 SortOrder 升序遍历，找到第一条同时满足 (groupID 命中 GroupIDs 或
// accountID 命中 AccountIDs) 且 Pricing 中存在匹配模型的规则即停止。
//
// 规则与 channels.ApplyPricingToAccountStats 开关相互独立：开关只影响
// "无规则匹配时是否回退到客户计费" 的语义；规则本身始终生效。
type AccountStatsPricingRule struct {
	ID         int64
	ChannelID  int64
	Name       string
	GroupIDs   []int64
	AccountIDs []int64
	SortOrder  int
	// Pricing 是规则下的模型定价行，结构与 ChannelModelPricing 对齐但仅使用
	// flat 字段（Intervals 不在该规则范围内，对账号统计没意义）。
	Pricing   []ChannelModelPricing
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Clone returns a deep copy of r so the cache snapshot can hand it out
// without exposing the underlying slice references.
func (r AccountStatsPricingRule) Clone() AccountStatsPricingRule {
	cp := r
	if r.GroupIDs != nil {
		cp.GroupIDs = make([]int64, len(r.GroupIDs))
		copy(cp.GroupIDs, r.GroupIDs)
	}
	if r.AccountIDs != nil {
		cp.AccountIDs = make([]int64, len(r.AccountIDs))
		copy(cp.AccountIDs, r.AccountIDs)
	}
	if r.Pricing != nil {
		cp.Pricing = make([]ChannelModelPricing, len(r.Pricing))
		for i := range r.Pricing {
			cp.Pricing[i] = r.Pricing[i].Clone()
		}
	}
	return cp
}

// IsActive reports whether the channel is in the active state.
func (c *Channel) IsActive() bool {
	return c.Status == StatusActive
}

// GetModelPricing looks up the pricing row that lists model. Match is exact,
// case-insensitive. Returns a clone so the caller cannot mutate the cache.
func (c *Channel) GetModelPricing(model string) *ChannelModelPricing {
	modelLower := strings.ToLower(model)
	for i := range c.ModelPricing {
		for _, m := range c.ModelPricing[i].Models {
			if strings.ToLower(m) == modelLower {
				cp := c.ModelPricing[i].Clone()
				return &cp
			}
		}
	}
	return nil
}

// FindMatchingInterval picks the (min, max] interval that contains
// totalTokens. The first interval's MinTokens=0 means a 0-token request
// falls through to the default price (returns nil).
func FindMatchingInterval(intervals []PricingInterval, totalTokens int) *PricingInterval {
	for i := range intervals {
		iv := &intervals[i]
		if totalTokens > iv.MinTokens && (iv.MaxTokens == nil || totalTokens <= *iv.MaxTokens) {
			return iv
		}
	}
	return nil
}

// GetIntervalForContext picks the interval matching totalTokens.
func (p *ChannelModelPricing) GetIntervalForContext(totalTokens int) *PricingInterval {
	return FindMatchingInterval(p.Intervals, totalTokens)
}

// GetTierByLabel locates a tier by its label (case-insensitive). Used by
// per_request / image billing modes.
func (p *ChannelModelPricing) GetTierByLabel(label string) *PricingInterval {
	labelLower := strings.ToLower(label)
	for i := range p.Intervals {
		if p.Intervals[i].TierLabel != nil && strings.ToLower(*p.Intervals[i].TierLabel) == labelLower {
			return &p.Intervals[i]
		}
	}
	return nil
}

// Clone returns a copy of p where the slices are independent. Pointer fields
// are shared but only read by callers, so this is safe for the cache.
func (p ChannelModelPricing) Clone() ChannelModelPricing {
	cp := p
	if p.Models != nil {
		cp.Models = make([]string, len(p.Models))
		copy(cp.Models, p.Models)
	}
	if p.Intervals != nil {
		cp.Intervals = make([]PricingInterval, len(p.Intervals))
		copy(cp.Intervals, p.Intervals)
	}
	return cp
}

// Clone returns a deep copy of c suitable for handing out from the cache.
func (c *Channel) Clone() *Channel {
	if c == nil {
		return nil
	}
	cp := *c
	if c.GroupIDs != nil {
		cp.GroupIDs = make([]int64, len(c.GroupIDs))
		copy(cp.GroupIDs, c.GroupIDs)
	}
	if c.ModelPricing != nil {
		cp.ModelPricing = make([]ChannelModelPricing, len(c.ModelPricing))
		for i := range c.ModelPricing {
			cp.ModelPricing[i] = c.ModelPricing[i].Clone()
		}
	}
	if c.ModelMapping != nil {
		cp.ModelMapping = make(map[string]map[string]string, len(c.ModelMapping))
		for platform, mapping := range c.ModelMapping {
			inner := make(map[string]string, len(mapping))
			for k, v := range mapping {
				inner[k] = v
			}
			cp.ModelMapping[platform] = inner
		}
	}
	if c.FeaturesConfig != nil {
		cp.FeaturesConfig = deepCopyFeaturesConfig(c.FeaturesConfig)
	}
	if c.AccountStatsPricingRules != nil {
		cp.AccountStatsPricingRules = make([]AccountStatsPricingRule, len(c.AccountStatsPricingRules))
		for i := range c.AccountStatsPricingRules {
			cp.AccountStatsPricingRules[i] = c.AccountStatsPricingRules[i].Clone()
		}
	}
	return &cp
}

// deepCopyFeaturesConfig creates a deep copy of FeaturesConfig to prevent cache pollution.
func deepCopyFeaturesConfig(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if inner, ok := v.(map[string]any); ok {
			dst[k] = deepCopyFeaturesConfig(inner)
		} else {
			dst[k] = v
		}
	}
	return dst
}

// ValidateIntervals enforces the interval-list invariants:
//
//   - MinTokens >= 0
//   - MaxTokens, when set, must be > 0 and > MinTokens
//   - all price fields must be >= 0
//   - intervals (sorted by MinTokens) must not overlap under (min, max]
//   - the unbounded interval (MaxTokens == nil) must come last
//
// Gaps between intervals are allowed and fall through to the default price.
func ValidateIntervals(intervals []PricingInterval) error {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]PricingInterval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MinTokens < sorted[j].MinTokens
	})

	for i := range sorted {
		if err := validateSingleInterval(&sorted[i], i); err != nil {
			return err
		}
	}
	return validateIntervalOverlap(sorted)
}

func validateSingleInterval(iv *PricingInterval, idx int) error {
	if iv.MinTokens < 0 {
		return fmt.Errorf("interval #%d: min_tokens (%d) must be >= 0", idx+1, iv.MinTokens)
	}
	if iv.MaxTokens != nil {
		if *iv.MaxTokens <= 0 {
			return fmt.Errorf("interval #%d: max_tokens (%d) must be > 0", idx+1, *iv.MaxTokens)
		}
		if *iv.MaxTokens <= iv.MinTokens {
			return fmt.Errorf("interval #%d: max_tokens (%d) must be > min_tokens (%d)",
				idx+1, *iv.MaxTokens, iv.MinTokens)
		}
	}
	return validateIntervalPrices(iv, idx)
}

func validateIntervalPrices(iv *PricingInterval, idx int) error {
	prices := []struct {
		name string
		val  decimal.NullDecimal
	}{
		{"input_price", iv.InputPrice},
		{"output_price", iv.OutputPrice},
		{"cache_write_price", iv.CacheWritePrice},
		{"cache_read_price", iv.CacheReadPrice},
		{"image_output_price", iv.ImageOutputPrice},
		{"per_request_price", iv.PerRequestPrice},
	}
	for _, p := range prices {
		if decimalx.IsNegative(p.val) {
			return fmt.Errorf("interval #%d: %s must be >= 0", idx+1, p.name)
		}
	}
	return nil
}

func validateIntervalOverlap(sorted []PricingInterval) error {
	for i, iv := range sorted {
		if iv.MaxTokens == nil && i < len(sorted)-1 {
			return fmt.Errorf("interval #%d: unbounded interval (max_tokens=null) must be the last one",
				i+1)
		}
		if i == 0 {
			continue
		}
		prev := sorted[i-1]
		// (min, max] semantics: prev covers (prev.Min, prev.Max], cur covers
		// (cur.Min, cur.Max]; an overlap means prev's upper bound exceeds
		// cur's lower bound.
		if prev.MaxTokens == nil || *prev.MaxTokens > iv.MinTokens {
			return fmt.Errorf("interval #%d and #%d overlap: prev max=%s > cur min=%d",
				i, i+1, formatMaxTokensLabel(prev.MaxTokens), iv.MinTokens)
		}
	}
	return nil
}

func formatMaxTokensLabel(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

// ChannelUsageFields captures the channel-related columns that platform
// gateways embed in their RecordUsageInput. Kept here so callers can copy
// the struct without dragging in the full channel service.
type ChannelUsageFields struct {
	ChannelID          int64  // 渠道 ID（0 = 无渠道）
	OriginalModel      string // 用户原始请求模型（渠道映射前）
	ChannelMappedModel string // 渠道映射后的模型名（无映射时等于 OriginalModel）
	BillingModelSource string // 计费模型来源："requested" / "upstream" / "channel_mapped"
	ModelMappingChain  string // 映射链描述，如 "a→b→c"
}
