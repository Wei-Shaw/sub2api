package routes

import (
	"net/http"

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
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	cfg *config.Config,
) {
	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	soraMaxBodySize := cfg.Gateway.SoraMaxBodySize
	if soraMaxBodySize <= 0 {
		soraMaxBodySize = cfg.Gateway.MaxBodySize
REDACTED
	soraBodyLimit := middleware.RequestBodyLimit(soraMaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	opsErrorLogger := handler.OpsErrorLoggerMiddleware(opsService)

	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(bodyLimit)
	gateway.Use(clientRequestID)
	gateway.Use(opsErrorLogger)
	gateway.Use(gin.HandlerFunc(apiKeyAuth))
	{
		gateway.POST("/messages", h.Gateway.Messages)
		gateway.POST("/messages/count_tokens", h.Gateway.CountTokens)
		gateway.GET("/models", h.Gateway.Models)
		gateway.GET("/usage", h.Gateway.Usage)
		// OpenAI Responses API
		gateway.POST("/responses", h.OpenAIGateway.Responses)
		gateway.GET("/responses", h.OpenAIGateway.ResponsesWebSocket)
		// 明确阻止旧协议入口：OpenAI 仅支持 Responses API，避免客户端误解为会自动路由到其它平台。
		gateway.POST("/chat/completions", func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": "Unsupported legacy protocol: /v1/chat/completions is not supported. Please use /v1/responses.",
			REDACTED,
		REDACTED)
	REDACTED)
REDACTED

	// Gemini 原生 API 兼容层（Gemini SDK/CLI 直连）
	gemini := r.Group("/v1beta")
	gemini.Use(bodyLimit)
	gemini.Use(clientRequestID)
	gemini.Use(opsErrorLogger)
	gemini.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	{
		gemini.GET("/models", h.Gateway.GeminiV1BetaListModels)
		gemini.GET("/models/:model", h.Gateway.GeminiV1BetaGetModel)
		// Gin treats ":" as a param marker, but Gemini uses "{modelREDACTED:{actionREDACTED" in the same segment.
		gemini.POST("/models/*modelAction", h.Gateway.GeminiV1BetaModels)
REDACTED

	// OpenAI Responses API（不带v1前缀的别名）
	r.POST("/responses", bodyLimit, clientRequestID, opsErrorLogger, gin.HandlerFunc(apiKeyAuth), h.OpenAIGateway.Responses)
	r.GET("/responses", bodyLimit, clientRequestID, opsErrorLogger, gin.HandlerFunc(apiKeyAuth), h.OpenAIGateway.ResponsesWebSocket)

	// Antigravity 模型列表
	r.GET("/antigravity/models", gin.HandlerFunc(apiKeyAuth), h.Gateway.AntigravityModels)

	// Antigravity 专用路由（仅使用 antigravity 账户，不混合调度）
	antigravityV1 := r.Group("/antigravity/v1")
	antigravityV1.Use(bodyLimit)
	antigravityV1.Use(clientRequestID)
	antigravityV1.Use(opsErrorLogger)
	antigravityV1.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1.Use(gin.HandlerFunc(apiKeyAuth))
	{
		antigravityV1.POST("/messages", h.Gateway.Messages)
		antigravityV1.POST("/messages/count_tokens", h.Gateway.CountTokens)
		antigravityV1.GET("/models", h.Gateway.AntigravityModels)
		antigravityV1.GET("/usage", h.Gateway.Usage)
REDACTED

	antigravityV1Beta := r.Group("/antigravity/v1beta")
	antigravityV1Beta.Use(bodyLimit)
	antigravityV1Beta.Use(clientRequestID)
	antigravityV1Beta.Use(opsErrorLogger)
	antigravityV1Beta.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1Beta.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	{
		antigravityV1Beta.GET("/models", h.Gateway.GeminiV1BetaListModels)
		antigravityV1Beta.GET("/models/:model", h.Gateway.GeminiV1BetaGetModel)
		antigravityV1Beta.POST("/models/*modelAction", h.Gateway.GeminiV1BetaModels)
REDACTED

	// Sora 专用路由（强制使用 sora 平台）
	soraV1 := r.Group("/sora/v1")
	soraV1.Use(soraBodyLimit)
	soraV1.Use(clientRequestID)
	soraV1.Use(opsErrorLogger)
	soraV1.Use(middleware.ForcePlatform(service.PlatformSora))
	soraV1.Use(gin.HandlerFunc(apiKeyAuth))
	{
		soraV1.POST("/chat/completions", h.SoraGateway.ChatCompletions)
		soraV1.GET("/models", h.Gateway.Models)
REDACTED

	// Sora 媒体代理（可选 API Key 验证）
	if cfg.Gateway.SoraMediaRequireAPIKey {
		r.GET("/sora/media/*filepath", gin.HandlerFunc(apiKeyAuth), h.SoraGateway.MediaProxy)
REDACTED else {
		r.GET("/sora/media/*filepath", h.SoraGateway.MediaProxy)
REDACTED
	// Sora 媒体代理（签名 URL，无需 API Key）
	r.GET("/sora/media-signed/*filepath", h.SoraGateway.MediaProxySigned)
REDACTED
