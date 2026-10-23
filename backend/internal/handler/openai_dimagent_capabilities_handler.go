package handler

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// DimAgentModelCapabilities serves the rich, key-scoped model catalog consumed
// by the DSH Sub2API DimAgent provider plugin. It never exposes upstream OAuth
// credentials or models outside the authenticated key's group.
func (h *OpenAIGatewayHandler) DimAgentModelCapabilities(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformDimAgent {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Model capabilities are only available for DimAgent groups")
		return
	}
	if h == nil || h.gatewayService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "DimAgent capability service is unavailable")
		return
	}
	catalog, err := h.gatewayService.FetchDimAgentGroupCapabilities(c.Request.Context(), apiKey.Group)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
		return
	}
	etag := `"` + catalog.CatalogVersion + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=60")
	if strings.TrimSpace(c.GetHeader("If-None-Match")) == etag {
		c.Status(http.StatusNotModified)
		c.Writer.WriteHeaderNow()
		return
	}
	c.JSON(http.StatusOK, catalog)
}
