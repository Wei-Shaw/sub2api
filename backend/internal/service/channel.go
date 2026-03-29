package service

import (
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

// Channel 渠道实体
type Channel struct {
	ID          int64
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// 关联的分组 ID 列表
	GroupIDs []int64
	// 模型定价列表
	ModelPricing []ChannelModelPricing
	// 渠道级模型映射
	ModelMapping map[string]string
}

// ChannelModelPricing 渠道模型定价条目
type ChannelModelPricing struct {
	ID               int64
	ChannelID        int64
	Models           []string          // 绑定的模型列表
	BillingMode      BillingMode       // 计费模式
	InputPrice       *float64          // 每 token 输入价格（USD）— 向后兼容 flat 定价
	OutputPrice      *float64          // 每 token 输出价格（USD）
	CacheWritePrice  *float64          // 缓存写入价格
	CacheReadPrice   *float64          // 缓存读取价格
	ImageOutputPrice *float64          // 图片输出价格（向后兼容）
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

// ResolveMappedModel 解析渠道级模型映射，返回映射后的模型名。
// 支持通配符（如 "claude-*" → "claude-sonnet-4"）。
// 如果没有匹配的映射规则，返回原始模型名。
func (c *Channel) ResolveMappedModel(requestedModel string) string {
	if len(c.ModelMapping) == 0 {
		return requestedModel
	}
	lower := strings.ToLower(requestedModel)
	// 精确匹配优先
	for src, dst := range c.ModelMapping {
		if strings.ToLower(src) == lower {
			return dst
		}
	}
	// 通配符匹配
	for src, dst := range c.ModelMapping {
		srcLower := strings.ToLower(src)
		if strings.HasSuffix(srcLower, "*") {
			prefix := strings.TrimSuffix(srcLower, "*")
			if strings.HasPrefix(lower, prefix) {
				return dst
			}
		}
	}
	return requestedModel
}

// IsActive 判断渠道是否启用
func (c *Channel) IsActive() bool {
	return c.Status == StatusActive
}

// GetModelPricing 根据模型名查找渠道定价，未找到返回 nil。
// 优先精确匹配，然后通配符匹配（如 claude-opus-*）。大小写不敏感。
// 返回值拷贝，不污染缓存。
func (c *Channel) GetModelPricing(model string) *ChannelModelPricing {
	modelLower := strings.ToLower(model)

	// 第一轮：精确匹配
	for i := range c.ModelPricing {
		for _, m := range c.ModelPricing[i].Models {
			if strings.ToLower(m) == modelLower {
				cp := c.ModelPricing[i].Clone()
				return &cp
			}
		}
	}

	// 第二轮：通配符匹配（仅支持末尾 *）
	for i := range c.ModelPricing {
		for _, m := range c.ModelPricing[i].Models {
			mLower := strings.ToLower(m)
			if strings.HasSuffix(mLower, "*") {
				prefix := strings.TrimSuffix(mLower, "*")
				if strings.HasPrefix(modelLower, prefix) {
					cp := c.ModelPricing[i].Clone()
					return &cp
				}
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
		cp.ModelMapping = make(map[string]string, len(c.ModelMapping))
		for k, v := range c.ModelMapping {
			cp.ModelMapping[k] = v
		}
	}
	return &cp
}
