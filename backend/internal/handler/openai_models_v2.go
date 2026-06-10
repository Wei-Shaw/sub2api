package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelsV2 returns Grok CLI model metadata for xAI platform groups.
// GET /v1/models-v2
func (h *OpenAIGatewayHandler) ModelsV2(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if service.PlatformFromAPIKey(apiKey) != service.PlatformXAI {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Models V2 API is not supported for this platform",
			},
		})
		return
	}
	c.JSON(http.StatusOK, xai.DefaultModelsV2Response())
}