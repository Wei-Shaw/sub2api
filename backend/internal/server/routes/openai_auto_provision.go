package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterOpenAIAutoProvisionRoutes registers machine-to-machine callbacks.
// Authentication is handled by the callback secret in the handler; these
// routes intentionally do not use the browser/admin JWT middleware.
func RegisterOpenAIAutoProvisionRoutes(v1 *gin.RouterGroup, h *handler.OpenAIAutoProvisionHandler) {
	group := v1.Group("/integrations/openai/auto-provision")
	group.POST("/callback", h.Callback)
	group.POST("/reauthorization/callback", h.ReauthorizationCallback)
	group.POST("/reauthorization/completion", h.ReauthorizationCompletion)
}
