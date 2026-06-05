package dto

import "time"

// PlazaCurrencyMetaDTO carries the data needed to render CNY ⇄ USD toggle on
// the plaza pages. The `balance_recharge_multiplier` is sourced from the
// existing payment configuration: 1 CNY recharge → multiplier USD credited.
//
// model_native is always "USD" (LiteLLM-derived prices are USD); plan_native
// is always "CNY" (operator-priced subscription plans).
type PlazaCurrencyMetaDTO struct {
	BalanceRechargeMultiplier float64 `json:"balance_recharge_multiplier"`
	ModelNative               string  `json:"model_native"`
	PlanNative                string  `json:"plan_native"`
}

// PlazaImagePricesDTO encapsulates the three-tier pricing for image generation
// models (1K / 2K / 4K), in USD per image.
type PlazaImagePricesDTO struct {
	Tier1K float64 `json:"tier_1k"`
	Tier2K float64 `json:"tier_2k"`
	Tier4K float64 `json:"tier_4k"`
}

// PlazaModelRowDTO represents one (group, model) row on the public model plaza.
//
// The `type` field is either "token" or "image". For "token" rows the
// `*_per_mtok` fields are populated and the image-related fields are nil.
// For "image" rows the per-MTok fields are zero and the `*_image_prices`
// fields are populated.
type PlazaModelRowDTO struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
	Model     string `json:"model"`
	Type      string `json:"type"`

	InputPricePerMTok      float64 `json:"input_price_per_mtok,omitempty"`
	OutputPricePerMTok     float64 `json:"output_price_per_mtok,omitempty"`
	SiteInputPricePerMTok  float64 `json:"site_input_price_per_mtok,omitempty"`
	SiteOutputPricePerMTok float64 `json:"site_output_price_per_mtok,omitempty"`

	// Token-only single-tier cache prices (USD per 1M tokens).
	//
	// Pointer semantics intentionally distinguish "absent" (nil → JSON omitted →
	// frontend renders "—") from "explicit zero" (&0 → JSON 0). Models that do
	// not declare `SupportsCacheBreakdown` and report zero cache pricing leave
	// these as nil so clients don't paint a misleading $0.
	CacheWritePricePerMTok     *float64 `json:"cache_write_price_per_mtok,omitempty"`
	CacheReadPricePerMTok      *float64 `json:"cache_read_price_per_mtok,omitempty"`
	SiteCacheWritePricePerMTok *float64 `json:"site_cache_write_price_per_mtok,omitempty"`
	SiteCacheReadPricePerMTok  *float64 `json:"site_cache_read_price_per_mtok,omitempty"`

	BaseImagePrices *PlazaImagePricesDTO `json:"base_image_prices,omitempty"`
	SiteImagePrices *PlazaImagePricesDTO `json:"site_image_prices,omitempty"`

	Multiplier      float64 `json:"multiplier"`
	DiscountPercent float64 `json:"discount_percent"`
}

// PlazaPlanCardDTO represents one subscription-plan card on the public plaza.
type PlazaPlanCardDTO struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Price          float64  `json:"price"`
	OriginalPrice  *float64 `json:"original_price,omitempty"`
	ValidityDays   int      `json:"validity_days"`
	ValidityUnit   string   `json:"validity_unit"`
	Features       string   `json:"features,omitempty"`
	GroupID        int64    `json:"group_id"`
	GroupName      string   `json:"group_name"`
	Platform       string   `json:"platform"`
	Models         []string `json:"models"`
	ModelsOverflow int      `json:"models_overflow"`
}

// PlazaModelsResponseDTO is the response payload for GET /api/v1/plaza/models.
type PlazaModelsResponseDTO struct {
	Rows         []PlazaModelRowDTO   `json:"rows"`
	CurrencyMeta PlazaCurrencyMetaDTO `json:"currency_meta"`
}

// PlazaPlansResponseDTO is the response payload for GET /api/v1/plaza/plans.
type PlazaPlansResponseDTO struct {
	Cards        []PlazaPlanCardDTO   `json:"cards"`
	CurrencyMeta PlazaCurrencyMetaDTO `json:"currency_meta"`
}

// PublicRechargePromoTierDTO 是公开充值赠送活动 DTO 的单个赠送档位。
type PublicRechargePromoTierDTO struct {
	MinAmount float64 `json:"min_amount"`
	BonusRate float64 `json:"bonus_rate"`
}

// PublicRechargePromoDTO 是 GET /api/v1/plaza/recharge-promo 的展示型 promo 体。
//
// 与 service.RechargePromo / checkout-info.recharge_promo 的差异（按 design D2）：
//   - 不含 `enabled`：能出现在响应里就意味着已启用且时间窗内
//   - 不含 `activity_id`：那是后端审计字段，对匿名前端无意义
//   - 多一个 `name`：作为运营文案位（首页 banner 标题），同时也回填到
//     checkout-info.recharge_promo.name 保持一致
type PublicRechargePromoDTO struct {
	Name       string                       `json:"name"`
	ValidFrom  *time.Time                   `json:"valid_from,omitempty"`
	ValidUntil *time.Time                   `json:"valid_until,omitempty"`
	Tiers      []PublicRechargePromoTierDTO `json:"tiers"`
	Version    string                       `json:"version"`
}

// PublicRechargePromoResponseDTO 是 GET /api/v1/plaza/recharge-promo 的整体响应。
//
// 当无生效活动时 `Promo` 为 nil（JSON 序列化为 `"promo": null`），
// 不使用 omitempty —— 前端始终能依靠 `promo` 字段是否为 null 来判断。
type PublicRechargePromoResponseDTO struct {
	Promo *PublicRechargePromoDTO `json:"promo"`
}
