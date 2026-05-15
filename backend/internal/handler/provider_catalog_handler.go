package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProviderCatalogHandler handles the public provider-catalog endpoint
// used by v-claw desktop app to dynamically discover available providers and models.
type ProviderCatalogHandler struct {
	catalogService *service.ProviderCatalogService
}

// NewProviderCatalogHandler creates a new ProviderCatalogHandler.
func NewProviderCatalogHandler(catalogService *service.ProviderCatalogService) *ProviderCatalogHandler {
	return &ProviderCatalogHandler{
		catalogService: catalogService,
	}
}

// GetCatalog returns the provider catalog.
// GET /provider-catalog
//
// This endpoint is public (no auth required) — the v-claw desktop app calls it
// during onboarding before the user has an API key. The response contains only
// provider/model metadata (no secrets, no user data).
func (h *ProviderCatalogHandler) GetCatalog(c *gin.Context) {
	catalog, err := h.catalogService.BuildCatalog(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.JSON(200, catalog)
}
