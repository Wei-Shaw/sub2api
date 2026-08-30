package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelPriceHandler serves the logged-in, presentation-only model price catalog.
type ModelPriceHandler struct {
	plazaService   *service.ModelPlazaService
	displayService *service.DisplayPricingService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

func NewModelPriceHandler(
	plazaService *service.ModelPlazaService,
	displayService *service.DisplayPricingService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPriceHandler {
	return &ModelPriceHandler{plazaService: plazaService, displayService: displayService, apiKeyService: apiKeyService, settingService: settingService}
}

// Get handles GET /api/v1/model-prices.
func (h *ModelPriceHandler) Get(c *gin.Context) {
	// Presentation pricing is edited live by admins; never serve a cached catalog.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	if h.settingService == nil || !h.settingService.GetModelPlazaRuntime(c.Request.Context()).Enabled {
		response.NotFound(c, "Model prices are not enabled")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return
	}
	groups, err := h.plazaService.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	allowed, restrictPublic, err := h.apiKeyService.GetUserGroupVisibility(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	visible := filterPlazaVisibleGroups(groups, allowed, restrictPublic)
	catalog, err := h.displayService.BuildCatalog(c.Request.Context(), visible)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, catalog)
}
