package dto

// ChannelModelPricingRequest is the JSON shape for one pricing row inside
// CreateChannelRequest / UpdateChannelRequest.
//
// Price fields stay *float64 to keep the JSON wire format identical to
// the pre-T4 shape; the conversion to decimal.NullDecimal happens inside
// the dto package's mapper functions (see mapper.go) so callers in the
// handler do not see decimal types.
type ChannelModelPricingRequest struct {
	Platform         string                   `json:"platform" binding:"omitempty,max=50,platform"`
	Models           []string                 `json:"models" binding:"required,min=1,max=100"`
	BillingMode      string                   `json:"billing_mode" binding:"billing_mode"`
	InputPrice       *float64                 `json:"input_price" binding:"omitempty,min=0"`
	OutputPrice      *float64                 `json:"output_price" binding:"omitempty,min=0"`
	CacheWritePrice  *float64                 `json:"cache_write_price" binding:"omitempty,min=0"`
	CacheReadPrice   *float64                 `json:"cache_read_price" binding:"omitempty,min=0"`
	ImageOutputPrice *float64                 `json:"image_output_price" binding:"omitempty,min=0"`
	PerRequestPrice  *float64                 `json:"per_request_price" binding:"omitempty,min=0"`
	Intervals        []PricingIntervalRequest `json:"intervals"`
}

// PricingIntervalRequest is the JSON shape for one tiered pricing band.
//
// ImageOutputPrice 与 ChannelModelPricingRequest.ImageOutputPrice 对称，
// 让 image-mode 的每一档 tier（例：1K / 2K / 4K）能各自配置图片输出价。
type PricingIntervalRequest struct {
	MinTokens        int      `json:"min_tokens"`
	MaxTokens        *int     `json:"max_tokens"`
	TierLabel        *string  `json:"tier_label"`
	InputPrice       *float64 `json:"input_price"`
	OutputPrice      *float64 `json:"output_price"`
	CacheWritePrice  *float64 `json:"cache_write_price"`
	CacheReadPrice   *float64 `json:"cache_read_price"`
	ImageOutputPrice *float64 `json:"image_output_price"`
	PerRequestPrice  *float64 `json:"per_request_price"`
	SortOrder        int      `json:"sort_order"`
}

// AccountStatsPricingRuleRequest is the input shape used by Create / Update
// to express one custom account-stats pricing rule. Pricing rows reuse the
// existing ChannelModelPricingRequest shape; intervals are not modelled
// because they're not consulted by the account-stats resolver.
type AccountStatsPricingRuleRequest struct {
	Name       string                       `json:"name" binding:"omitempty,max=100"`
	GroupIDs   []int64                      `json:"group_ids"`
	AccountIDs []int64                      `json:"account_ids"`
	SortOrder  int                          `json:"sort_order"`
	Pricing    []ChannelModelPricingRequest `json:"pricing"`
}

// ChannelModelPricingResponse is the JSON shape returned for one pricing
// row. Fields stay as *float64 so existing frontend `Number(value) * scale`
// arithmetic still works after the service layer migrated to
// shopspring/decimal (T4). The dto.PricingToResponse mapper is the single
// boundary that performs the decimal -> *float64 translation.
type ChannelModelPricingResponse struct {
	ID               int64                     `json:"id"`
	Platform         string                    `json:"platform"`
	Models           []string                  `json:"models"`
	BillingMode      string                    `json:"billing_mode"`
	InputPrice       *float64                  `json:"input_price"`
	OutputPrice      *float64                  `json:"output_price"`
	CacheWritePrice  *float64                  `json:"cache_write_price"`
	CacheReadPrice   *float64                  `json:"cache_read_price"`
	ImageOutputPrice *float64                  `json:"image_output_price"`
	PerRequestPrice  *float64                  `json:"per_request_price"`
	Intervals        []PricingIntervalResponse `json:"intervals"`
}

// PricingIntervalResponse is the JSON shape for one tiered pricing band.
type PricingIntervalResponse struct {
	ID               int64    `json:"id"`
	MinTokens        int      `json:"min_tokens"`
	MaxTokens        *int     `json:"max_tokens"`
	TierLabel        *string  `json:"tier_label,omitempty"`
	InputPrice       *float64 `json:"input_price"`
	OutputPrice      *float64 `json:"output_price"`
	CacheWritePrice  *float64 `json:"cache_write_price"`
	CacheReadPrice   *float64 `json:"cache_read_price"`
	ImageOutputPrice *float64 `json:"image_output_price"`
	PerRequestPrice  *float64 `json:"per_request_price"`
	SortOrder        int      `json:"sort_order"`
}

// AccountStatsPricingRuleResponse is the JSON shape returned for one
// custom account-stats pricing rule.
type AccountStatsPricingRuleResponse struct {
	ID         int64                         `json:"id"`
	Name       string                        `json:"name"`
	GroupIDs   []int64                       `json:"group_ids"`
	AccountIDs []int64                       `json:"account_ids"`
	SortOrder  int                           `json:"sort_order"`
	Pricing    []ChannelModelPricingResponse `json:"pricing"`
}
