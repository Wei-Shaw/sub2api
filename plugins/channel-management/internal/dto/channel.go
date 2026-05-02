// Package dto holds the JSON-wire request / response shapes used by the
// channel-management plugin's HTTP handlers. The package is intentionally
// thin — it owns the public field names, validator tags, and the boundary
// converters between HTTP-shape DTO and service-domain structs.
//
// Single source of truth:
//   - 自定义 validator 注册见 handler/validators.go
//   - 枚举值权威来源：service/channel.go
//     BillingMode* / ChannelStatus* / Platform* / BillingModelSource*
//
// The validator tag literals (`channel_status` / `billing_mode` /
// `billing_model_source` / `platform`) are registered against gin's
// validator engine by handler.RegisterCustomValidators(); the allowed
// value sets come from service.BillingModes() / service.ChannelStatuses()
// / service.BillingModelSources() / service.Platforms(). Adding a new
// enum value only requires editing those service helpers — DTO struct
// tags stay literal-free.
package dto

// CreateChannelRequest is the JSON body for POST /admin/channels.
type CreateChannelRequest struct {
	Name                       string                           `json:"name" binding:"required,max=100"`
	Description                string                           `json:"description"`
	GroupIDs                   []int64                          `json:"group_ids"`
	ModelPricing               []ChannelModelPricingRequest     `json:"model_pricing"`
	ModelMapping               map[string]map[string]string     `json:"model_mapping"`
	BillingModelSource         string                           `json:"billing_model_source" binding:"billing_model_source"`
	RestrictModels             bool                             `json:"restrict_models"`
	Features                   string                           `json:"features"`
	FeaturesConfig             map[string]any                   `json:"features_config"`
	ApplyPricingToAccountStats bool                             `json:"apply_pricing_to_account_stats"`
	AccountStatsPricingRules   []AccountStatsPricingRuleRequest `json:"account_stats_pricing_rules"`
}

// UpdateChannelRequest is the JSON body for PUT /admin/channels/:id.
//
// Pointer-typed fields use the "nil = no change" convention so the handler
// can distinguish "not provided" from "set to zero". This mirrors the
// service.UpdateChannelInput shape on the other side of the boundary.
type UpdateChannelRequest struct {
	Name                       string                        `json:"name" binding:"omitempty,max=100"`
	Description                *string                       `json:"description"`
	Status                     string                        `json:"status" binding:"channel_status"`
	GroupIDs                   *[]int64                      `json:"group_ids"`
	ModelPricing               *[]ChannelModelPricingRequest `json:"model_pricing"`
	ModelMapping               map[string]map[string]string  `json:"model_mapping"`
	BillingModelSource         string                        `json:"billing_model_source" binding:"billing_model_source"`
	RestrictModels             *bool                         `json:"restrict_models"`
	Features                   *string                       `json:"features"`
	FeaturesConfig             map[string]any                `json:"features_config"`
	ApplyPricingToAccountStats *bool                         `json:"apply_pricing_to_account_stats"`
	// AccountStatsPricingRules: nil 表示不修改；非 nil（即使空数组）表示
	// "用提交的列表整体替换"。语义与 ModelPricing 字段一致。
	AccountStatsPricingRules *[]AccountStatsPricingRuleRequest `json:"account_stats_pricing_rules"`
}

// ChannelResponse is the JSON shape returned by channel CRUD endpoints.
type ChannelResponse struct {
	ID                         int64                             `json:"id"`
	Name                       string                            `json:"name"`
	Description                string                            `json:"description"`
	Status                     string                            `json:"status"`
	BillingModelSource         string                            `json:"billing_model_source"`
	RestrictModels             bool                              `json:"restrict_models"`
	Features                   string                            `json:"features"`
	FeaturesConfig             map[string]any                    `json:"features_config"`
	ApplyPricingToAccountStats bool                              `json:"apply_pricing_to_account_stats"`
	GroupIDs                   []int64                           `json:"group_ids"`
	ModelPricing               []ChannelModelPricingResponse     `json:"model_pricing"`
	ModelMapping               map[string]map[string]string      `json:"model_mapping"`
	AccountStatsPricingRules   []AccountStatsPricingRuleResponse `json:"account_stats_pricing_rules"`
	CreatedAt                  string                            `json:"created_at"`
	UpdatedAt                  string                            `json:"updated_at"`
}
