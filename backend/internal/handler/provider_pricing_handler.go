package handler

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	providerPricingSchemaVersion = "1.0"
	providerPricingCurrency      = "CNY"
	providerPricingUnit          = "per_1m_tokens"
	providerPricingTokenFactor   = 1_000_000
	providerPricingRoundFactor   = 1_000_000
)

type providerPricingChannelService interface {
	ListAvailable(ctx context.Context) ([]service.AvailableChannel, error)
}

type providerPricingPaymentConfigService interface {
	GetPaymentConfig(ctx context.Context) (*service.PaymentConfig, error)
}

// ProviderPricingHandler exposes the Hvoy-compatible provider pricing endpoint.
type ProviderPricingHandler struct {
	channelService providerPricingChannelService
	paymentConfig  providerPricingPaymentConfigService
	cfg            *config.Config
}

func NewProviderPricingHandler(
	channelService *service.ChannelService,
	paymentConfig *service.PaymentConfigService,
	cfg *config.Config,
) *ProviderPricingHandler {
	return &ProviderPricingHandler{
		channelService: channelService,
		paymentConfig:  paymentConfig,
		cfg:            cfg,
	}
}

type providerPricingResponse struct {
	SchemaVersion string                       `json:"schema_version"`
	Success       bool                         `json:"success"`
	Message       string                       `json:"message,omitempty"`
	Data          *providerPricingDataResponse `json:"data,omitempty"`
}

type providerPricingDataResponse struct {
	Currency   string                         `json:"currency"`
	PriceUnit  string                         `json:"price_unit"`
	SiteName   string                         `json:"site_name,omitempty"`
	SiteDomain string                         `json:"site_domain,omitempty"`
	UpdatedAt  string                         `json:"updated_at"`
	Models     []providerPricingModelResponse `json:"models"`
}

type providerPricingModelResponse struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	InputPrice         float64  `json:"input_price"`
	OutputPrice        *float64 `json:"output_price"`
	CacheInputPrice    *float64 `json:"cache_input_price"`
	CacheCreatePrice   *float64 `json:"cache_create_price"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note"`
}

// Get returns GET /api/provider/pricing in the Hvoy Provider Pricing API shape.
func (h *ProviderPricingHandler) Get(c *gin.Context) {
	if h == nil || h.channelService == nil {
		writeProviderPricingError(c, http.StatusServiceUnavailable, "provider pricing unavailable")
		return
	}

	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		writeProviderPricingError(c, http.StatusServiceUnavailable, "service temporarily unavailable")
		return
	}

	balanceRechargeMultiplier := h.balanceRechargeMultiplier(c.Request.Context())
	siteName, siteDomain := h.siteInfo()
	models := buildProviderPricingModels(channels, balanceRechargeMultiplier)

	c.JSON(http.StatusOK, providerPricingResponse{
		SchemaVersion: providerPricingSchemaVersion,
		Success:       true,
		Message:       "",
		Data: &providerPricingDataResponse{
			Currency:   providerPricingCurrency,
			PriceUnit:  providerPricingUnit,
			SiteName:   siteName,
			SiteDomain: siteDomain,
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
			Models:     models,
		},
	})
}

func (h *ProviderPricingHandler) balanceRechargeMultiplier(ctx context.Context) float64 {
	if h == nil || h.paymentConfig == nil {
		return 1
	}
	cfg, err := h.paymentConfig.GetPaymentConfig(ctx)
	if err != nil || cfg == nil || cfg.BalanceRechargeMultiplier <= 0 {
		return 1
	}
	return cfg.BalanceRechargeMultiplier
}

func (h *ProviderPricingHandler) siteInfo() (string, string) {
	siteName := "sub2api"
	siteDomain := ""
	if h == nil || h.cfg == nil {
		return siteName, siteDomain
	}
	if name := strings.TrimSpace(h.cfg.Log.ServiceName); name != "" {
		siteName = name
	}
	if raw := strings.TrimSpace(h.cfg.Server.FrontendURL); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			siteDomain = u.Hostname()
		}
	}
	return siteName, siteDomain
}

func writeProviderPricingError(c *gin.Context, status int, message string) {
	c.JSON(status, providerPricingResponse{
		SchemaVersion: providerPricingSchemaVersion,
		Success:       false,
		Message:       message,
	})
}

func buildProviderPricingModels(channels []service.AvailableChannel, balanceRechargeMultiplier float64) []providerPricingModelResponse {
	if balanceRechargeMultiplier <= 0 {
		balanceRechargeMultiplier = 1
	}

	out := make([]providerPricingModelResponse, 0)
	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}
		for _, group := range ch.Groups {
			if group.Platform == "" {
				continue
			}
			rateMultiplier := group.RateMultiplier
			if rateMultiplier < 0 {
				rateMultiplier = 0
			}
			for _, model := range ch.SupportedModels {
				if model.Platform != group.Platform || model.Pricing == nil {
					continue
				}
				row, ok := providerPricingModelFromPricing(model.Name, group.Name, model.Pricing, rateMultiplier, balanceRechargeMultiplier)
				if ok {
					out = append(out, row)
				}
			}
		}
	}
	return out
}

func providerPricingModelFromPricing(
	modelName string,
	groupName string,
	pricing *service.ChannelModelPricing,
	rateMultiplier float64,
	balanceRechargeMultiplier float64,
) (providerPricingModelResponse, bool) {
	if pricing == nil {
		return providerPricingModelResponse{}, false
	}
	mode := pricing.BillingMode
	if mode == "" {
		mode = service.BillingModeToken
	}
	if mode != service.BillingModeToken || pricing.InputPrice == nil {
		return providerPricingModelResponse{}, false
	}

	convert := func(v *float64) *float64 {
		if v == nil || *v < 0 {
			return nil
		}
		converted := *v * rateMultiplier * providerPricingTokenFactor / balanceRechargeMultiplier
		converted = math.Round(converted*providerPricingRoundFactor) / providerPricingRoundFactor
		return &converted
	}

	input := convert(pricing.InputPrice)
	if input == nil {
		return providerPricingModelResponse{}, false
	}
	return providerPricingModelResponse{
		ModelName:          modelName,
		GroupName:          groupName,
		InputPrice:         *input,
		OutputPrice:        convert(pricing.OutputPrice),
		CacheInputPrice:    convert(pricing.CacheReadPrice),
		CacheCreatePrice:   convert(pricing.CacheWritePrice),
		CacheCreatePrice1h: nil,
		Enabled:            true,
		Note:               "",
	}, true
}
