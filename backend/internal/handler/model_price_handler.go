package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type modelPricePlazaLister interface {
	ListGroups(ctx context.Context) ([]service.PlazaGroup, error)
}

type modelPriceCatalogBuilder interface {
	BuildCatalog(ctx context.Context, groups []service.PlazaGroup) (*service.DisplayPricingCatalog, error)
}

type modelPriceGroupVisibilityReader interface {
	GetUserGroupVisibility(ctx context.Context, userID int64) (map[int64]struct{}, bool, error)
}

type modelPriceRuntimeReader interface {
	GetModelPlazaRuntime(ctx context.Context) service.ModelPlazaRuntime
}

// ModelPriceHandler serves the presentation-only model price catalog for
// authenticated users and, when enabled by settings, anonymous visitors.
type ModelPriceHandler struct {
	plazaService   modelPricePlazaLister
	displayService modelPriceCatalogBuilder
	apiKeyService  modelPriceGroupVisibilityReader
	settingService modelPriceRuntimeReader
}

func NewModelPriceHandler(
	plazaService *service.ModelPlazaService,
	displayService *service.DisplayPricingService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPriceHandler {
	h := &ModelPriceHandler{}
	if plazaService != nil {
		h.plazaService = plazaService
	}
	if displayService != nil {
		h.displayService = displayService
	}
	if apiKeyService != nil {
		h.apiKeyService = apiKeyService
	}
	if settingService != nil {
		h.settingService = settingService
	}
	return h
}

// Get handles GET /api/v1/model-prices.
func (h *ModelPriceHandler) Get(c *gin.Context) {
	// Presentation pricing is edited live by admins; never serve a cached catalog.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	if h.settingService == nil {
		response.NotFound(c, "Model prices are not enabled")
		return
	}
	runtime := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !runtime.Enabled {
		response.NotFound(c, "Model prices are not enabled")
		return
	}
	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if runtime.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}
	groups, err := h.plazaService.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var allowed map[int64]struct{}
	var restrictPublic bool
	if authed {
		allowed, restrictPublic, err = h.apiKeyService.GetUserGroupVisibility(c.Request.Context(), subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	visible := filterPlazaVisibleGroups(groups, allowed, restrictPublic)
	catalog, err := h.displayService.BuildCatalog(c.Request.Context(), visible)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, catalog)
}
