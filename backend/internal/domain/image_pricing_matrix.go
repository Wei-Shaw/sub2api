package domain

// ImagePricingTierKey 是 ImagePricingMatrix 的顶层键，固定为 1K/2K/4K。
// 每档的可配置分辨率阈值存储在 groups.image_resolution_* 字段。
type ImagePricingTierKey = string

// ImageQualityKey 是 ImagePricingMatrix 二级键，取值为归一后的 quality：
//   - "low"
//   - "medium"
//   - "high"
//
// 客户端传入的 "auto"/空串/缺省 在计费层归一为 "high"（详见 spec
// media-prepay-billing「quality 归一规则」）。
type ImageQualityKey = string

// ImagePricingMatrix 是分组持有的图片二维定价矩阵。
//
// 结构：tier_key -> quality_key -> price (USD per image)。
// 例：
//
//	{
//	  "1K": { "low": 0.006, "medium": 0.053, "high": 0.211 },
//	  "4K": { "low": 0.012, "medium": 0.101, "high": 0.401 }
//	}
//
// 缺失某 (tier, quality) 即视为该格未配置，BillingService 将按 spec
// 「计费查找回退顺序」回退到旧 image_price_1k/2k/4k 字段，再回退到
// LiteLLM 默认价。
//
// 该类型对应 groups 表 image_pricing_matrix 列（JSONB, nullable），
// nil/空 map 表示分组未启用矩阵定价（保持兼容现网行为）。
type ImagePricingMatrix map[ImagePricingTierKey]map[ImageQualityKey]float64
