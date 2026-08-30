package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DisplayPricingHandler struct {
	service           *service.DisplayPricingService
	plazaService      *service.ModelPlazaService
	officialPriceSync *service.OfficialPriceSyncService
}

func NewDisplayPricingHandler(displayService *service.DisplayPricingService, plazaService *service.ModelPlazaService) *DisplayPricingHandler {
	return NewDisplayPricingHandlerWithOfficialPriceFetcher(displayService, plazaService, nil)
}

func NewDisplayPricingHandlerWithOfficialPriceFetcher(
	displayService *service.DisplayPricingService,
	plazaService *service.ModelPlazaService,
	fetcher service.OfficialPriceCandidateFetcher,
) *DisplayPricingHandler {
	return &DisplayPricingHandler{
		service: displayService, plazaService: plazaService,
		officialPriceSync: service.NewOfficialPriceSyncService(displayService, fetcher),
	}
}

type updateDisplayPricingSettingsRequest struct {
	GlobalMultiplier float64 `json:"global_multiplier" binding:"required,gt=0"`
}

type createDisplayProviderRequest struct {
	Provider       string   `json:"provider" binding:"required"`
	DisplayName    string   `json:"display_name" binding:"required"`
	ProviderNote   string   `json:"provider_note"`
	PerRequestNote string   `json:"per_request_note"`
	ImageNote      string   `json:"image_note"`
	Currency       string   `json:"currency" binding:"required,oneof=CNY USD"`
	Multiplier     *float64 `json:"multiplier"`
	LogoKey        string   `json:"logo_key"`
	LogoURL        string   `json:"logo_url"`
	SortOrder      int      `json:"sort_order"`
}

type updateDisplayProviderRequest struct {
	DisplayName    string   `json:"display_name" binding:"required"`
	ProviderNote   string   `json:"provider_note"`
	PerRequestNote string   `json:"per_request_note"`
	ImageNote      string   `json:"image_note"`
	Currency       string   `json:"currency" binding:"required,oneof=CNY USD"`
	Multiplier     *float64 `json:"multiplier"`
	LogoKey        string   `json:"logo_key"`
	LogoURL        string   `json:"logo_url"`
	SortOrder      int      `json:"sort_order"`
}

type displayModelPriceRequest struct {
	Platform    string `json:"platform" binding:"required"`
	ModelName   string `json:"model_name" binding:"required"`
	Provider    string `json:"provider" binding:"required"`
	BillingMode string `json:"billing_mode" binding:"required,oneof=token per_request image"`
	Currency    string `json:"currency" binding:"required,oneof=CNY USD"`
	Enabled     *bool  `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	ModelNote   string `json:"model_note"`

	OfficialInputPerMillion      *float64   `json:"official_input_per_million"`
	OfficialOutputPerMillion     *float64   `json:"official_output_per_million"`
	OfficialCacheWritePerMillion *float64   `json:"official_cache_write_per_million"`
	OfficialCacheReadPerMillion  *float64   `json:"official_cache_read_per_million"`
	OfficialPriceSource          string     `json:"official_price_source"`
	OfficialPriceSourceURL       string     `json:"official_price_source_url"`
	OfficialPriceSyncedAt        *time.Time `json:"official_price_synced_at"`
	ModelMultiplier              *float64   `json:"model_multiplier"`

	PerRequestLTE256K          *float64                    `json:"per_request_lte_256k"`
	PerRequest256K512KOverride *float64                    `json:"per_request_256k_512k_override"`
	PerRequestGT512KOverride   *float64                    `json:"per_request_gt_512k_override"`
	ImagePrices                []service.DisplayImagePrice `json:"image_prices"`
}

func (h *DisplayPricingHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"global_multiplier": settings.GlobalMultiplier, "updated_at": settings.UpdatedAt})
}

func (h *DisplayPricingHandler) UpdateSettings(c *gin.Context) {
	var req updateDisplayPricingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), req.GlobalMultiplier)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"global_multiplier": settings.GlobalMultiplier, "updated_at": settings.UpdatedAt})
}

func (h *DisplayPricingHandler) ListProviders(c *gin.Context) {
	items, err := h.service.ListProviders(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *DisplayPricingHandler) CreateProvider(c *gin.Context) {
	var req createDisplayProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	item, err := h.service.CreateProvider(c.Request.Context(), service.DisplayPricingProvider{
		Provider: req.Provider, DisplayName: req.DisplayName, ProviderNote: req.ProviderNote,
		PerRequestNote: req.PerRequestNote, ImageNote: req.ImageNote, Currency: req.Currency,
		Multiplier: req.Multiplier, LogoKey: req.LogoKey, LogoURL: req.LogoURL, SortOrder: req.SortOrder,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, displayProviderResponse(item))
}

func (h *DisplayPricingHandler) UpdateProvider(c *gin.Context) {
	var req updateDisplayProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	item, err := h.service.UpdateProvider(c.Request.Context(), c.Param("provider"), service.DisplayPricingProvider{
		DisplayName: req.DisplayName, ProviderNote: req.ProviderNote,
		PerRequestNote: req.PerRequestNote, ImageNote: req.ImageNote, Currency: req.Currency, Multiplier: req.Multiplier,
		LogoKey: req.LogoKey, LogoURL: req.LogoURL, SortOrder: req.SortOrder,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, displayProviderResponse(item))
}

func (h *DisplayPricingHandler) DeleteProvider(c *gin.Context) {
	provider := c.Param("provider")
	deletedModels, err := h.service.DeleteProvider(c.Request.Context(), provider)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"provider": provider, "deleted_models": deletedModels})
}

func (h *DisplayPricingHandler) ListModels(c *gin.Context) {
	items, err := h.service.ListModels(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for i := range items {
		out = append(out, displayModelAdminResponse(&items[i]))
	}
	response.Success(c, gin.H{"items": out})
}

func (h *DisplayPricingHandler) UpsertModel(c *gin.Context) {
	var req displayModelPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	item, err := h.service.UpsertModel(c.Request.Context(), modelFromDisplayRequest(req))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, displayModelAdminResponse(item))
}

func (h *DisplayPricingHandler) UpdateModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid display price ID")
		return
	}
	var req displayModelPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	item, err := h.service.UpdateModel(c.Request.Context(), id, modelFromDisplayRequest(req))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, displayModelAdminResponse(item))
}

func (h *DisplayPricingHandler) DeleteModel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid display price ID")
		return
	}
	if err := h.service.DeleteModel(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *DisplayPricingHandler) ListDiscoveredModels(c *gin.Context) {
	groups, err := h.plazaService.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items, err := h.service.ListDiscovered(c.Request.Context(), groups)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *DisplayPricingHandler) PreviewOfficialPrices(c *gin.Context) {
	preview, err := h.officialPriceSync.Preview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}

func (h *DisplayPricingHandler) ApplyOfficialPrices(c *gin.Context) {
	var req struct {
		Models []service.OfficialPriceApplySelection `json:"models" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	result, err := h.officialPriceSync.Apply(c.Request.Context(), req.Models)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func modelFromDisplayRequest(req displayModelPriceRequest) service.DisplayModelPrice {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return service.DisplayModelPrice{
		Platform: req.Platform, ModelName: req.ModelName, Provider: req.Provider, BillingMode: req.BillingMode,
		Currency: req.Currency, Enabled: enabled, SortOrder: req.SortOrder, ModelNote: req.ModelNote,
		OfficialInputPerMillion: req.OfficialInputPerMillion, OfficialOutputPerMillion: req.OfficialOutputPerMillion,
		OfficialCacheWritePerMillion: req.OfficialCacheWritePerMillion, OfficialCacheReadPerMillion: req.OfficialCacheReadPerMillion,
		OfficialPriceSource: req.OfficialPriceSource, OfficialPriceSourceURL: req.OfficialPriceSourceURL,
		OfficialPriceSyncedAt: req.OfficialPriceSyncedAt,
		ModelMultiplier:       req.ModelMultiplier, PerRequestLTE256K: req.PerRequestLTE256K,
		PerRequest256K512KOverride: req.PerRequest256K512KOverride, PerRequestGT512KOverride: req.PerRequestGT512KOverride,
		ImagePrices: req.ImagePrices,
	}
}

func displayProviderResponse(p *service.DisplayPricingProvider) gin.H {
	return gin.H{
		"provider": p.Provider, "display_name": p.DisplayName, "provider_note": p.ProviderNote,
		"per_request_note": p.PerRequestNote, "image_note": p.ImageNote, "currency": p.Currency,
		"multiplier": p.Multiplier, "logo_key": p.LogoKey, "logo_url": p.LogoURL,
		"sort_order": p.SortOrder, "updated_at": p.UpdatedAt,
	}
}

func displayModelAdminResponse(p *service.DisplayModelPrice) gin.H {
	return gin.H{
		"id": p.ID, "platform": p.Platform, "model_name": p.ModelName, "provider": p.Provider,
		"billing_mode": p.BillingMode, "currency": p.Currency, "enabled": p.Enabled, "sort_order": p.SortOrder, "model_note": p.ModelNote,
		"official_input_per_million": p.OfficialInputPerMillion, "official_output_per_million": p.OfficialOutputPerMillion,
		"official_cache_write_per_million": p.OfficialCacheWritePerMillion, "official_cache_read_per_million": p.OfficialCacheReadPerMillion,
		"official_price_source": p.OfficialPriceSource, "official_price_source_url": p.OfficialPriceSourceURL,
		"official_price_synced_at": p.OfficialPriceSyncedAt,
		"model_multiplier":         p.ModelMultiplier, "per_request_lte_256k": p.PerRequestLTE256K,
		"per_request_256k_512k_override": p.PerRequest256K512KOverride, "per_request_gt_512k_override": p.PerRequestGT512KOverride,
		"image_prices": p.ImagePrices, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
	}
}
