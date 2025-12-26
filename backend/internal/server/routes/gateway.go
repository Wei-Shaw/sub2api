package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterGatewayRoutes 注册 API 网关路由（Claude/OpenAI 兼容）
func RegisterGatewayRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	apiKeyAuth middleware.ApiKeyAuthMiddleware,
) {
	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(gin.HandlerFunc(apiKeyAuth))
	{
		gateway.POST("/messages", h.Gateway.Messages)
		gateway.POST("/messages/count_tokens", h.Gateway.CountTokens)
		gateway.GET("/models", h.Gateway.Models)
		gateway.GET("/usage", h.Gateway.Usage)
		// OpenAI Responses API
		gateway.POST("/responses", h.OpenAIGateway.Responses)
REDACTED

	// OpenAI Responses API（不带v1前缀的别名）
	r.POST("/responses", gin.HandlerFunc(apiKeyAuth), h.OpenAIGateway.Responses)
REDACTED
