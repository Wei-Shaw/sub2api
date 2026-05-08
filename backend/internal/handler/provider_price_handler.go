package handler

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const providerPricingSchemaVersion = "1.0"

type ProviderPriceHandler struct {
	groupService         *service.GroupService
	paymentConfigService *service.PaymentConfigService
	billingService       *service.BillingService
	channelService       *service.ChannelService
	settingService       *service.SettingService
	gatewayService       *service.GatewayService
	resolver             *service.ModelPricingResolver
	modelLister          func(context.Context, *service.Group) []string
}

func NewProviderPriceHandler(
	groupService *service.GroupService,
	paymentConfigService *service.PaymentConfigService,
	billingService *service.BillingService,
	channelService *service.ChannelService,
	settingService *service.SettingService,
	gatewayService *service.GatewayService,
	resolver *service.ModelPricingResolver,
) *ProviderPriceHandler {
	return &ProviderPriceHandler{
		groupService:         groupService,
		paymentConfigService: paymentConfigService,
		billingService:       billingService,
		channelService:       channelService,
		settingService:       settingService,
		gatewayService:       gatewayService,
		resolver:             resolver,
	}
}

type providerPricingResponse struct {
	SchemaVersion string                 `json:"schema_version"`
	Success       bool                   `json:"success"`
	Message       string                 `json:"message,omitempty"`
	Data          *providerPricingData   `json:"data,omitempty"`
}

type providerPricingData struct {
	Currency  string                 `json:"currency"`
	PriceUnit string                 `json:"price_unit"`
	SiteName  string                 `json:"site_name,omitempty"`
	SiteDomain string                `json:"site_domain,omitempty"`
	UpdatedAt string                 `json:"updated_at"`
	Models    []providerPricingModel `json:"models"`
}

type providerPricingModel struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	InputPrice         *float64 `json:"input_price"`
	OutputPrice        *float64 `json:"output_price"`
	CacheInputPrice    *float64 `json:"cache_input_price"`
	CacheCreatePrice   *float64 `json:"cache_create_price"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note,omitempty"`
}

type providerPriceEntry struct {
	groupName string
	modelName string
	model     providerPricingModel
}

type modelExportPrice struct {
	input          float64
	output         float64
	cacheInput     float64
	cacheCreate5m  float64
	cacheCreate1h  float64
}

func (h *ProviderPriceHandler) GetProviderPricing(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Accept")
	if c.Request.Method == http.MethodOptions {
		c.Status(http.StatusNoContent)
		return
	}

	groups, err := h.groupService.ListActive(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, providerPricingResponse{
			SchemaVersion: providerPricingSchemaVersion,
			Success:       false,
			Message:       "failed to load groups",
		})
		return
	}

	paymentCfg, err := h.paymentConfigService.GetPaymentConfig(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, providerPricingResponse{
			SchemaVersion: providerPricingSchemaVersion,
			Success:       false,
			Message:       "failed to load payment config",
		})
		return
	}

	rechargeMultiplier := paymentCfg.BalanceRechargeMultiplier
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}

	entries := h.buildProviderPriceEntries(ctx, groups, rechargeMultiplier)
	models := make([]providerPricingModel, 0, len(entries))
	for _, entry := range entries {
		models = append(models, entry.model)
	}

	c.JSON(http.StatusOK, providerPricingResponse{
		SchemaVersion: providerPricingSchemaVersion,
		Success:       true,
		Data: &providerPricingData{
			Currency:   "CNY",
			PriceUnit:  "per_1m_tokens",
			SiteName:   h.siteName(ctx),
			SiteDomain: h.siteDomain(ctx),
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
			Models:     models,
		},
	})
}

func (h *ProviderPriceHandler) buildProviderPriceEntries(
	ctx context.Context,
	groups []service.Group,
	rechargeMultiplier float64,
) []providerPriceEntry {
	entries := make([]providerPriceEntry, 0)
	seen := make(map[string]struct{})

	for i := range groups {
		group := groups[i]
		if !group.IsActive() || group.IsSubscriptionType() {
			continue
		}

		modelIDs := h.availableModelsForGroup(ctx, &group)
		for _, modelID := range modelIDs {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				continue
			}

			key := group.Name + "\x00" + strings.ToLower(modelID)
			if _, ok := seen[key]; ok {
				continue
			}

			pricingInput := service.PricingInput{Model: modelID}
			if h.channelService != nil {
				pricingInput.GroupID = &group.ID
			}
			resolved := h.resolver.Resolve(ctx, pricingInput)
			if resolved == nil {
				continue
			}

			exportPrice, ok := h.exportPriceForResolved(resolved)
			if !ok {
				continue
			}

			model := providerPricingModel{
				ModelName:          modelID,
				GroupName:          group.Name,
				InputPrice:         usdPerTokenToCnyPerMTokPtr(exportPrice.input, group.RateMultiplier, rechargeMultiplier),
				OutputPrice:        usdPerTokenToCnyPerMTokPtr(exportPrice.output, group.RateMultiplier, rechargeMultiplier),
				CacheInputPrice:    usdPerTokenToCnyPerMTokPtr(exportPrice.cacheInput, group.RateMultiplier, rechargeMultiplier),
				CacheCreatePrice:   usdPerTokenToCnyPerMTokPtr(exportPrice.cacheCreate5m, group.RateMultiplier, rechargeMultiplier),
				CacheCreatePrice1h: usdPerTokenToCnyPerMTokPtr(exportPrice.cacheCreate1h, group.RateMultiplier, rechargeMultiplier),
				Enabled:            true,
			}
			if strings.EqualFold(group.Platform, service.PlatformOpenAI) {
				model.CacheCreatePrice = nil
				model.CacheCreatePrice1h = nil
			}

			if model.InputPrice == nil {
				continue
			}

			entries = append(entries, providerPriceEntry{
				groupName: group.Name,
				modelName: modelID,
				model:     model,
			})
			seen[key] = struct{}{}
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].groupName == entries[j].groupName {
			return strings.ToLower(entries[i].modelName) < strings.ToLower(entries[j].modelName)
		}
		return strings.ToLower(entries[i].groupName) < strings.ToLower(entries[j].groupName)
	})

	return entries
}

func (h *ProviderPriceHandler) availableModelsForGroup(ctx context.Context, group *service.Group) []string {
	if h.modelLister != nil {
		return dedupeModelIDs(filterModelsByGroupScope(group, h.modelLister(ctx, group)))
	}

	models := make([]string, 0)
	if h.gatewayService != nil {
		models = append(models, h.gatewayService.GetAvailableModels(ctx, &group.ID, group.Platform)...)
	}

	if h.channelService != nil {
		channel, err := h.channelService.GetChannelForGroup(ctx, group.ID)
		if err == nil && channel != nil {
			for _, supported := range channel.SupportedModels() {
				if supported.Platform != group.Platform {
					continue
				}
				models = append(models, supported.Name)
			}
		}
	}

	return dedupeModelIDs(filterModelsByGroupScope(group, models))
}

func dedupeModelIDs(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		key := strings.ToLower(strings.TrimSpace(model))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func (h *ProviderPriceHandler) exportPriceForResolved(resolved *service.ResolvedPricing) (modelExportPrice, bool) {
	if resolved == nil || resolved.Mode != service.BillingModeToken {
		return modelExportPrice{}, false
	}

	// Public provider pricing should expose a stable model price.
	// If token intervals are present, we currently avoid exporting the model
	// instead of publishing an arbitrary tier or silently falling back to the
	// global baseline.
	if len(resolved.Intervals) > 0 || resolved.BasePricing == nil {
		return modelExportPrice{}, false
	}

	return modelExportPrice{
		input:         resolved.BasePricing.InputPricePerToken,
		output:        resolved.BasePricing.OutputPricePerToken,
		cacheInput:    resolved.BasePricing.CacheReadPricePerToken,
		cacheCreate5m: cacheWrite5m(resolved.BasePricing),
		cacheCreate1h: cacheWrite1h(resolved.BasePricing),
	}, resolved.BasePricing.InputPricePerToken > 0
}

func usdPerTokenToCnyPerMTokPtr(usdPerToken, rateMultiplier, rechargeMultiplier float64) *float64 {
	if usdPerToken <= 0 || rechargeMultiplier <= 0 {
		return nil
	}
	if rateMultiplier <= 0 {
		rateMultiplier = 1
	}
	v := (usdPerToken * rateMultiplier / rechargeMultiplier) * 1_000_000
	v = roundPrice(v)
	return &v
}

func cacheWrite5m(pricing *service.ModelPricing) float64 {
	if pricing == nil {
		return 0
	}
	if pricing.CacheCreation5mPrice > 0 {
		return pricing.CacheCreation5mPrice
	}
	return pricing.CacheCreationPricePerToken
}

func cacheWrite1h(pricing *service.ModelPricing) float64 {
	if pricing == nil {
		return 0
	}
	if pricing.CacheCreation1hPrice > 0 {
		return pricing.CacheCreation1hPrice
	}
	return 0
}

func roundPrice(v float64) float64 {
	return float64(int(v*1000000+0.5)) / 1000000
}

func (h *ProviderPriceHandler) siteName(ctx context.Context) string {
	if h.settingService == nil {
		return "Sub2API"
	}
	return h.settingService.GetSiteName(ctx)
}

func (h *ProviderPriceHandler) siteDomain(ctx context.Context) string {
	if h.settingService == nil {
		return ""
	}
	raw := strings.TrimSpace(h.settingService.GetFrontendURL(ctx))
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}

func filterModelsByGroupScope(group *service.Group, models []string) []string {
	if group == nil {
		return models
	}

	applyImageGate := group.Platform == service.PlatformGemini || group.Platform == service.PlatformAntigravity

	allowedScopes := make(map[string]struct{}, len(group.SupportedModelScopes))
	for _, scope := range group.SupportedModelScopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope != "" {
			allowedScopes[scope] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if modelAllowedByScope(group, allowedScopes, model, applyImageGate) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func modelAllowedByScope(group *service.Group, allowedScopes map[string]struct{}, model string, applyImageGate bool) bool {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if modelLower == "" {
		return false
	}

	if applyImageGate && isImageLikeModel(modelLower) && !group.AllowImageGeneration {
		return false
	}

	if len(allowedScopes) == 0 {
		return true
	}

	switch group.Platform {
	case service.PlatformAnthropic:
		_, ok := allowedScopes["claude"]
		return ok
	case service.PlatformOpenAI:
		return true
	case service.PlatformGemini:
		if isImageLikeModel(modelLower) {
			_, ok := allowedScopes["gemini_image"]
			return ok
		}
		_, ok := allowedScopes["gemini_text"]
		return ok
	case service.PlatformAntigravity:
		if strings.HasPrefix(modelLower, "claude") {
			_, ok := allowedScopes["claude"]
			return ok
		}
		if isImageLikeModel(modelLower) {
			_, ok := allowedScopes["gemini_image"]
			return ok
		}
		_, ok := allowedScopes["gemini_text"]
		return ok
	default:
		return true
	}
}

func isImageLikeModel(model string) bool {
	return strings.Contains(model, "image")
}
