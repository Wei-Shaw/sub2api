package service

import (
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Channel is a minimal stub retained for code that references channel-level
// feature overrides (e.g. CodexImageGenerationBridgeOverride). The full channel
// management has been migrated to plugins/channel-management; this struct is
// only used by the host for feature-flag resolution on the hot path.
type Channel struct {
	ID             int64
	Status         string
	FeaturesConfig map[string]any
}

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

// ChannelModelPricing 渠道模型定价条目。
// 渠道管理逻辑已迁移到 plugins/channel-management/；核心只保留这份定价结构作为
// PricingOverrideCache 翻译层的本地 owner（见 channel_cache_reader.go），
// 以便 Gateway 热路径读取缓存而无需 import plugin 代码。
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

// ChannelUsageFields 渠道相关的使用记录字段（嵌入到各平台的 RecordUsageInput 中）
type ChannelUsageFields struct {
	ChannelID          int64  // 渠道 ID（0 = 无渠道）
	OriginalModel      string // 用户原始请求模型（渠道映射前）
	ChannelMappedModel string // 渠道映射后的模型名（无映射时等于 OriginalModel）
	BillingModelSource string // 计费模型来源："requested" / "upstream" / "channel_mapped"
	ModelMappingChain  string // 映射链描述，如 "a→b→c"
}

// ChannelMappingResult 渠道映射查找结果
type ChannelMappingResult struct {
	MappedModel        string // 映射后的模型名（无映射时等于原始模型名）
	ChannelID          int64  // 渠道 ID（0 = 无渠道关联）
	Mapped             bool   // 是否发生了映射
	BillingModelSource string // 计费模型来源
}

// BuildModelMappingChain 根据映射结果和上游实际模型构建映射链描述
func (r ChannelMappingResult) BuildModelMappingChain(reqModel, upstreamModel string) string {
	if !r.Mapped {
		if upstreamModel != "" && upstreamModel != reqModel {
			return reqModel + "→" + upstreamModel
		}
		return ""
	}
	if upstreamModel != "" && upstreamModel != r.MappedModel {
		return reqModel + "→" + r.MappedModel + "→" + upstreamModel
	}
	return reqModel + "→" + r.MappedModel
}

// ToUsageFields 将渠道映射结果转为使用记录字段
func (r ChannelMappingResult) ToUsageFields(reqModel, upstreamModel string) ChannelUsageFields {
	channelMappedModel := reqModel
	if r.Mapped {
		channelMappedModel = r.MappedModel
	}
	return ChannelUsageFields{
		ChannelID:          r.ChannelID,
		OriginalModel:      reqModel,
		ChannelMappedModel: channelMappedModel,
		BillingModelSource: r.BillingModelSource,
		ModelMappingChain:  r.BuildModelMappingChain(reqModel, upstreamModel),
	}
}

// ReplaceModelInBody 替换请求体 JSON 中的 model 字段。
// 这是一个通用工具函数，gateway 在执行渠道映射后用它改写上游请求。
func ReplaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 {
		return body
	}
	if current := gjson.GetBytes(body, "model"); current.Exists() && current.String() == newModel {
		return body
	}
	newBody, err := sjson.SetBytes(body, "model", newModel)
	if err != nil {
		return body
	}
	return newBody
}
