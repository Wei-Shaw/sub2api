package service

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// BillingMode 计费模式
type BillingMode string

const (
	BillingModeToken      BillingMode = "token"       // 按 token 区间计费
	BillingModePerRequest BillingMode = "per_request" // 按次计费（支持上下文窗口分层）
	BillingModeImage      BillingMode = "image"       // 图片计费（当前按次，预留 token 计费）
)

// IsValid 检查 BillingMode 是否为合法值
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

// Channel 渠道实体
type Channel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string         // "requested", "upstream", or "channel_mapped"
	RestrictModels     bool           // 是否限制模型（仅允许定价列表中的模型）
	Features           string         // 渠道特性描述（JSON 数组），用于支付页面展示
	FeaturesConfig     map[string]any // 渠道功能配置（如 web search emulation）
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// 关联的分组 ID 列表
	GroupIDs []int64
	// 模型定价列表（每条含 Platform 字段）
	ModelPricing []ChannelModelPricing
	// 渠道级模型映射（按平台分组：platform → {src→dst}）
	ModelMapping map[string]map[string]string

	// 账号统计定价
	ApplyPricingToAccountStats bool                      // 是否应用渠道模型定价到账号统计
	AccountStatsPricingRules   []AccountStatsPricingRule // 自定义账号统计定价规则（按 SortOrder 排序，先命中为准）
}

// AccountStatsPricingRule 账号统计定价规则
// 每条规则包含匹配条件（分组/账号）和独立的模型定价。
// 多条规则按 SortOrder 排序，先命中为准。
type AccountStatsPricingRule struct {
	ID         int64
	ChannelID  int64
	Name       string
	GroupIDs   []int64
	AccountIDs []int64
	SortOrder  int
	Pricing    []ChannelModelPricing // 规则内的模型定价（复用现有定价结构）
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ChannelModelPricing 渠道模型定价条目
type ChannelModelPricing struct {
	ID               int64
	ChannelID        int64
	Platform         string            // 所属平台（anthropic/openai/gemini/...）
	Models           []string          // 绑定的模型列表
	BillingMode      BillingMode       // 计费模式
	InputPrice       *float64          // 每 token 输入价格（USD）— 向后兼容 flat 定价
	OutputPrice      *float64          // 每 token 输出价格（USD）
	CacheWritePrice  *float64          // 缓存写入价格
	CacheReadPrice   *float64          // 缓存读取价格
	ImageOutputPrice *float64          // 图片输出价格（向后兼容）
	PerRequestPrice  *float64          // 默认按次计费价格（USD）
	Intervals        []PricingInterval // 区间定价列表
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PricingInterval 定价区间（token 区间 / 按次分层 / 图片分辨率分层）
type PricingInterval struct {
	ID              int64
	PricingID       int64
	MinTokens       int      // 区间下界（含）
	MaxTokens       *int     // 区间上界（不含），nil = 无上限
	TierLabel       string   // 层级标签（按次/图片模式：1K, 2K, 4K, HD 等）
	InputPrice      *float64 // token 模式：每 token 输入价
	OutputPrice     *float64 // token 模式：每 token 输出价
	CacheWritePrice *float64 // token 模式：缓存写入价
	CacheReadPrice  *float64 // token 模式：缓存读取价
	PerRequestPrice *float64 // 按次/图片模式：每次请求价格
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsActive 判断渠道是否启用
func (c *Channel) IsActive() bool {
	return c.Status == StatusActive
}

// GetModelPricing 根据模型名查找渠道定价，未找到返回 nil。
// 精确匹配，大小写不敏感。返回值拷贝，不污染缓存。
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

// FindMatchingInterval 在区间列表中查找匹配 totalTokens 的区间。
// 区间为左开右闭 (min, max]：min 不含，max 包含。
// 第一个区间 min=0 时，0 token 不匹配任何区间（回退到默认价格）。
func FindMatchingInterval(intervals []PricingInterval, totalTokens int) *PricingInterval {
	for i := range intervals {
		iv := &intervals[i]
		if totalTokens > iv.MinTokens && (iv.MaxTokens == nil || totalTokens <= *iv.MaxTokens) {
			return iv
		}
	}
	return nil
}

// GetIntervalForContext 根据总 context token 数查找匹配的区间。
func (p *ChannelModelPricing) GetIntervalForContext(totalTokens int) *PricingInterval {
	return FindMatchingInterval(p.Intervals, totalTokens)
}

// GetTierByLabel 根据标签查找层级（用于 per_request / image 模式）
func (p *ChannelModelPricing) GetTierByLabel(label string) *PricingInterval {
	labelLower := strings.ToLower(label)
	for i := range p.Intervals {
		if strings.ToLower(p.Intervals[i].TierLabel) == labelLower {
			return &p.Intervals[i]
		}
	}
	return nil
}

// Clone 返回 ChannelModelPricing 的拷贝（切片独立，指针字段共享，调用方只读安全）
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

// Clone 返回 Channel 的深拷贝
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
		for i, rule := range c.AccountStatsPricingRules {
			cp.AccountStatsPricingRules[i] = rule
			if rule.GroupIDs != nil {
				cp.AccountStatsPricingRules[i].GroupIDs = make([]int64, len(rule.GroupIDs))
				copy(cp.AccountStatsPricingRules[i].GroupIDs, rule.GroupIDs)
			}
			if rule.AccountIDs != nil {
				cp.AccountStatsPricingRules[i].AccountIDs = make([]int64, len(rule.AccountIDs))
				copy(cp.AccountStatsPricingRules[i].AccountIDs, rule.AccountIDs)
			}
			if rule.Pricing != nil {
				cp.AccountStatsPricingRules[i].Pricing = make([]ChannelModelPricing, len(rule.Pricing))
				for j := range rule.Pricing {
					cp.AccountStatsPricingRules[i].Pricing[j] = rule.Pricing[j].Clone()
				}
			}
		}
	}
	return &cp
}

// IsWebSearchEmulationEnabled 返回该渠道是否为指定平台启用了 web search 模拟。
func (c *Channel) IsWebSearchEmulationEnabled(platform string) bool {
	if c == nil || c.FeaturesConfig == nil {
		return false
	}
	wse, ok := c.FeaturesConfig[featureKeyWebSearchEmulation].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := wse[platform].(bool)
	return ok && enabled
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

// ValidateIntervals 校验区间列表的合法性。
// 规则：MinTokens >= 0；MaxTokens 若非 nil 则 > 0 且 > MinTokens；
// 所有价格字段 >= 0；区间按 MinTokens 排序后无重叠（(min, max] 语义）；
// 无界区间（MaxTokens=nil）必须是最后一个。间隙允许（回退默认价格）。
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

// validateSingleInterval 校验单个区间的字段合法性
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

// validateIntervalPrices 校验区间内所有价格字段 >= 0
func validateIntervalPrices(iv *PricingInterval, idx int) error {
	prices := []struct {
		name string
		val  *float64
	}{
		{"input_price", iv.InputPrice},
		{"output_price", iv.OutputPrice},
		{"cache_write_price", iv.CacheWritePrice},
		{"cache_read_price", iv.CacheReadPrice},
		{"per_request_price", iv.PerRequestPrice},
	}
	for _, p := range prices {
		if p.val != nil && *p.val < 0 {
			return fmt.Errorf("interval #%d: %s must be >= 0", idx+1, p.name)
		}
	}
	return nil
}

// validateIntervalOverlap 校验排序后的区间列表无重叠，且无界区间在最后
func validateIntervalOverlap(sorted []PricingInterval) error {
	for i, iv := range sorted {
		// 无界区间必须是最后一个
		if iv.MaxTokens == nil && i < len(sorted)-1 {
			return fmt.Errorf("interval #%d: unbounded interval (max_tokens=null) must be the last one",
				i+1)
		}
		if i == 0 {
			continue
		}
		prev := sorted[i-1]
		// 检查重叠：前一个区间的上界 > 当前区间的下界则重叠
		// (min, max] 语义：prev 覆盖 (prev.Min, prev.Max]，cur 覆盖 (cur.Min, cur.Max]
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

// ChannelUsageFields 渠道相关的使用记录字段（嵌入到各平台的 RecordUsageInput 中）
type ChannelUsageFields struct {
	ChannelID          int64  // 渠道 ID（0 = 无渠道）
	OriginalModel      string // 用户原始请求模型（渠道映射前）
	ChannelMappedModel string // 渠道映射后的模型名（无映射时等于 OriginalModel）
	BillingModelSource string // 计费模型来源："requested" / "upstream" / "channel_mapped"
	ModelMappingChain  string // 映射链描述，如 "a→b→c"
}

// SupportedModel 渠道的一个支持模型条目（无通配符、可直接展示给用户）
type SupportedModel struct {
	Name     string               // 用户侧模型名
	Platform string               // 所属平台
	Pricing  *ChannelModelPricing // 定价详情（nil 表示未配置定价）
}

// wildcardSuffix 是模型模式中的通配符后缀标记（仅支持尾部匹配）。
const wildcardSuffix = "*"

// splitWildcardSuffix 将模型模式拆分为 (prefix, isWildcard)。
//
//	"claude-opus-*"  → ("claude-opus-", true)
//	"claude-opus-4"  → ("claude-opus-4", false)
//	"*"              → ("", true)
//
// 注意：返回的 prefix 保持原始大小写，由调用方按需 ToLower。
func splitWildcardSuffix(pattern string) (prefix string, isWildcard bool) {
	if strings.HasSuffix(pattern, wildcardSuffix) {
		return strings.TrimSuffix(pattern, wildcardSuffix), true
	}
	return pattern, false
}

// GetModelPricingByPlatform 在指定平台下查找精确模型的定价，未找到返回 nil。
// 与 GetModelPricing 的区别：按 Platform 隔离，避免跨平台同名模型误匹配。
func (c *Channel) GetModelPricingByPlatform(platform, model string) *ChannelModelPricing {
	if c == nil {
		return nil
	}
	modelLower := strings.ToLower(model)
	for i := range c.ModelPricing {
		if c.ModelPricing[i].Platform != platform {
			continue
		}
		for _, m := range c.ModelPricing[i].Models {
			if strings.ToLower(m) == modelLower {
				cp := c.ModelPricing[i].Clone()
				return &cp
			}
		}
	}
	return nil
}

// platformPricingIndex 是单个平台下定价信息的复合索引。
// 一次扫描即可同时支持精确查找（exact 分支）与有序遍历（wildcard 分支），
// 避免 SupportedModels 对每个平台重复扫描定价列表。
//
// byLower 与 names/originalCase 共享同一套去重规则：以 lower-case 模型名为 key，
// 首个命中保留其原始大小写。names 维持按定价行扫描顺序的稳定迭代。
type platformPricingIndex struct {
	byLower      map[string]*ChannelModelPricing // lowercased model name → pricing (Clone'd)
	originalCase map[string]string               // lowercased model name → original-case model name
	names        []string                        // priced model names in their ORIGINAL case, insertion-ordered, deduped case-insensitively (first wins)
}

// buildPricingIndex 对渠道的定价列表做一次扫描，按 platform 聚合为查找索引。
// 索引值是定价条目的 Clone 指针，调用方可安全按需返回副本而不污染缓存。
// 通配符后缀条目（如 "claude-*"）不被索引（它们是模式，不是具体模型名）。
// 同一平台中以大小写不敏感方式去重，先出现者保留原始大小写。
func buildPricingIndex(pricings []ChannelModelPricing) map[string]*platformPricingIndex {
	idx := make(map[string]*platformPricingIndex)
	for i := range pricings {
		p := pricings[i]
		pidx, ok := idx[p.Platform]
		if !ok {
			pidx = &platformPricingIndex{
				byLower:      make(map[string]*ChannelModelPricing),
				originalCase: make(map[string]string),
				names:        make([]string, 0),
			}
			idx[p.Platform] = pidx
		}
		for _, m := range p.Models {
			if _, wild := splitWildcardSuffix(m); wild {
				continue
			}
			lower := strings.ToLower(m)
			if _, exists := pidx.byLower[lower]; exists {
				continue // 首个命中胜出（case-insensitive 去重后第一个定价 / 第一个原始大小写）
			}
			cp := pricings[i].Clone()
			pidx.byLower[lower] = &cp
			pidx.originalCase[lower] = m
			pidx.names = append(pidx.names, m)
		}
	}
	return idx
}

// SupportedModels 计算渠道的支持模型列表，结果保证不含通配符。
//
// 算法（以渠道自身的 ModelMapping 为唯一入口）：
//   - 遍历 Channel.ModelMapping 的每个 platform 条目；
//   - 映射 key 不带尾部 "*"：直接作为一个支持模型名（即使没有匹配的定价行，也会产出 Pricing=nil 的条目）；
//   - 映射 key 带尾部 "*"：用同 platform 的 ModelPricing.Models 做前缀匹配展开（定价中带 "*" 的条目被忽略，因为它们本身就是模式，不是具体模型名）；
//   - 映射 key 为 `"*"`（单独一个星号）将展开为该平台所有定价模型（前缀为空 → 全匹配）。这是刻意行为，用于"将该平台所有模型透传"的场景；
//   - 未在 ModelMapping 中出现的 platform 不会产出任何条目——这是**刻意设计**（"没配映射就不显示"），即使该平台有定价行。
//
// 当映射 key（exact 或 wildcard 展开后的候选）能命中定价时，结果中的 Name 使用**定价的原始大小写**
// （定价是模型身份的事实来源），否则保留映射 key 的原始大小写。
// 每个结果尝试从 platform 索引查找精确定价，未配置则 Pricing=nil。
// 结果按 (Platform, Name) 稳定排序，并按 (Platform, lowercase(Name)) 去重。
func (c *Channel) SupportedModels() []SupportedModel {
	if c == nil || len(c.ModelMapping) == 0 {
		return nil
	}

	idx := buildPricingIndex(c.ModelPricing)

	type dedupKey struct {
		platform string
		name     string
	}
	seen := make(map[dedupKey]struct{})
	result := make([]SupportedModel, 0)

	add := func(platform, name string, pidx *platformPricingIndex) {
		key := dedupKey{platform: platform, name: strings.ToLower(name)}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		var pricing *ChannelModelPricing
		displayName := name
		if pidx != nil {
			lower := strings.ToLower(name)
			if p, ok := pidx.byLower[lower]; ok {
				pricing = p
				displayName = pidx.originalCase[lower] // 定价大小写胜出
			}
		}
		result = append(result, SupportedModel{
			Name:     displayName,
			Platform: platform,
			Pricing:  pricing,
		})
	}

	for platform, mapping := range c.ModelMapping {
		if len(mapping) == 0 {
			continue
		}
		pidx := idx[platform] // 可能为 nil（该平台无定价行）
		for src := range mapping {
			prefix, isWild := splitWildcardSuffix(src)
			if isWild {
				if pidx == nil {
					continue
				}
				prefixLower := strings.ToLower(prefix)
				for _, candidate := range pidx.names {
					if strings.HasPrefix(strings.ToLower(candidate), prefixLower) {
						add(platform, candidate, pidx)
					}
				}
				continue
			}
			add(platform, src, pidx)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		return result[i].Name < result[j].Name
	})
	return result
}
