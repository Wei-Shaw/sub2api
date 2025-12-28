package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterGatewayRoutes 注册 API 网关路由（Claude/OpenAI/Gemini 兼容）
func RegisterGatewayRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	apiKeyAuth middleware.ApiKeyAuthMiddleware,
	apiKeyService *service.ApiKeyService,
	subscriptionService *service.SubscriptionService,
	cfg *config.Config,
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

	// Gemini 原生 API 兼容层（Gemini SDK/CLI 直连）
	gemini := r.Group("/v1beta")
	gemini.Use(middleware.ApiKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	{
		gemini.GET("/models", h.Gateway.GeminiV1BetaListModels)
		gemini.GET("/models/:model", h.Gateway.GeminiV1BetaGetModel)
		// Gin treats ":" as a param marker, but Gemini uses "{modelREDACTED:{actionREDACTED" in the same segment.
		gemini.POST("/models/*modelAction", h.Gateway.GeminiV1BetaModels)
REDACTED

	// OpenAI Responses API（不带v1前缀的别名）
	r.POST("/responses", gin.HandlerFunc(apiKeyAuth), h.OpenAIGateway.Responses)
REDACTED
