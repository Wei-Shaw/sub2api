// Package handler — payment plugin user-facing HTTP endpoints.
//
// Adapted from backend/internal/handler. Auth identity is read from the
// V4 X-Plugin-User-* request headers via pluginsdk.RequestMetadata
// instead of the host JWT middleware.
package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// PaymentHandler handles user-facing payment requests.
type PaymentHandler struct {
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
}

// NewPaymentHandler creates a new PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		configService:  configService,
	}
}

// RegisterRoutes attaches the user-facing payment routes onto the
// supplied Gin router group. The plugin host calls this once during
// Init from plugin.go so route names match the manifest declarations.
func (h *PaymentHandler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/config", h.GetPaymentConfig)
	api.GET("/checkout-info", h.GetCheckoutInfo)
	api.GET("/plans", h.GetPlans)
	api.GET("/limits", h.GetLimits)
	api.POST("/orders", h.CreateOrder)
	api.POST("/orders/verify", h.VerifyOrder)
	api.GET("/orders/my", h.GetMyOrders)
	api.GET("/orders/refund-eligible-providers", h.GetRefundEligibleProviders)
	api.GET("/orders/:id", h.GetOrder)
	api.POST("/orders/:id/cancel", h.CancelOrder)
	api.POST("/orders/:id/refund-request", h.RequestRefund)
}

// RegisterPublicRoutes attaches the anonymous public lookup endpoints
// (verify by out_trade_no, resolve by resume token).
func (h *PaymentHandler) RegisterPublicRoutes(public *gin.RouterGroup) {
	public.POST("/orders/verify", h.VerifyOrderPublic)
	public.POST("/orders/resolve", h.ResolveOrderPublicByResumeToken)
}

// GetPaymentConfig returns the payment system configuration.
func (h *PaymentHandler) GetPaymentConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// planWithPlatform mirrors the legacy /plans response shape, enriched
// with the group's platform string used by frontend color coding.
type planWithPlatform struct {
	ID            int64            `json:"id"`
	GroupID       int64            `json:"group_id"`
	GroupPlatform string           `json:"group_platform"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Price         decimal.Decimal  `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price,omitempty"`
	ValidityDays  int              `json:"validity_days"`
	ValidityUnit  string           `json:"validity_unit"`
	Features      string           `json:"features"`
	ProductName   string           `json:"product_name"`
	ForSale       bool             `json:"for_sale"`
	SortOrder     int              `json:"sort_order"`
}

// GetPlans returns subscription plans available for sale.
func (h *PaymentHandler) GetPlans(c *gin.Context) {
	plans, err := h.configService.ListPlansForSale(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	platformMap := h.configService.GetGroupPlatformMap(c.Request.Context(), plans)
	result := make([]planWithPlatform, 0, len(plans))
	for _, p := range plans {
		result = append(result, planWithPlatform{
			ID: int64(p.ID), GroupID: p.GroupID, GroupPlatform: platformMap[p.GroupID],
			Name: p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice,
			ValidityDays: p.ValidityDays, ValidityUnit: p.ValidityUnit, Features: p.Features,
			ProductName: p.ProductName, ForSale: p.ForSale, SortOrder: p.SortOrder,
		})
	}
	response.Success(c, result)
}

// GetCheckoutInfo returns all data the payment page needs in a single call.
func (h *PaymentHandler) GetCheckoutInfo(c *gin.Context) {
	ctx := c.Request.Context()

	limitsResp, err := h.configService.GetAvailableMethodLimits(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	cfg, err := h.configService.GetPaymentConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	plans, _ := h.configService.ListPlansForSale(ctx)
	groupInfo := h.configService.GetGroupInfoMap(ctx, plans)
	planList := make([]checkoutPlan, 0, len(plans))
	for _, p := range plans {
		gi := groupInfo[p.GroupID]
		planList = append(planList, checkoutPlan{
			ID: int64(p.ID), GroupID: p.GroupID,
			GroupPlatform: gi.Platform, GroupName: gi.Name,
			Name: p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice,
			ValidityDays: p.ValidityDays, ValidityUnit: p.ValidityUnit, Features: parseFeatures(p.Features),
			ProductName: p.ProductName,
		})
	}

	response.Success(c, checkoutInfoResponse{
		Methods:                   limitsResp.Methods,
		GlobalMin:                 limitsResp.GlobalMin,
		GlobalMax:                 limitsResp.GlobalMax,
		Plans:                     planList,
		BalanceDisabled:           cfg.BalanceDisabled,
		BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier,
		RechargeFeeRate:           cfg.RechargeFeeRate,
		HelpText:                  cfg.HelpText,
		HelpImageURL:              cfg.HelpImageURL,
		StripePublishableKey:      cfg.StripePublishableKey,
	})
}

type checkoutInfoResponse struct {
	Methods                   map[string]service.MethodLimits `json:"methods"`
	GlobalMin                 decimal.Decimal                 `json:"global_min"`
	GlobalMax                 decimal.Decimal                 `json:"global_max"`
	Plans                     []checkoutPlan                  `json:"plans"`
	BalanceDisabled           bool                            `json:"balance_disabled"`
	BalanceRechargeMultiplier decimal.Decimal                 `json:"balance_recharge_multiplier"`
	RechargeFeeRate           decimal.Decimal                 `json:"recharge_fee_rate"`
	HelpText                  string                          `json:"help_text"`
	HelpImageURL              string                          `json:"help_image_url"`
	StripePublishableKey      string                          `json:"stripe_publishable_key"`
}

type checkoutPlan struct {
	ID            int64            `json:"id"`
	GroupID       int64            `json:"group_id"`
	GroupPlatform string           `json:"group_platform"`
	GroupName     string           `json:"group_name"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Price         decimal.Decimal  `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price,omitempty"`
	ValidityDays  int              `json:"validity_days"`
	ValidityUnit  string           `json:"validity_unit"`
	Features      []string         `json:"features"`
	ProductName   string           `json:"product_name"`
}

// parseFeatures splits a newline-separated features string into a slice.
func parseFeatures(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

// GetLimits returns per-payment-type limits derived from enabled provider instances.
func (h *PaymentHandler) GetLimits(c *gin.Context) {
	resp, err := h.configService.GetAvailableMethodLimits(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, resp)
}
