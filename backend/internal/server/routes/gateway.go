package routes

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// RegisterGatewayRoutes 注册 API 网关路由（Claude/OpenAI/Gemini 兼容）
func RegisterGatewayRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	cfg *config.Config,
) {
	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	opsErrorLogger := handler.OpsErrorLoggerMiddleware(opsService)
	endpointNorm := handler.InboundEndpointMiddleware()

	// 未分组 Key 拦截中间件（按协议格式区分错误响应）
	requireGroupAnthropic := middleware.RequireGroupAssignment(settingService, middleware.AnthropicErrorWriter)
	requireGroupGoogle := middleware.RequireGroupAssignment(settingService, middleware.GoogleErrorWriter)

	dispatchMessages := func(c *gin.Context) {
		platform := routePlatformForMessagesEndpoint(c)
		middleware.SetRoutePlatformIntent(c, platform)
		if isOpenAICompatibleRoutePlatform(platform) {
			h.OpenAIGateway.Messages(c)
			return
		}
		h.Gateway.Messages(c)
	}
	dispatchOpenAICompatible := func(openAIHandler gin.HandlerFunc, anthropicHandler gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			platform := routePlatformForOpenAICompatibleEndpoint(c)
			middleware.SetRoutePlatformIntent(c, platform)
			if isOpenAICompatibleRoutePlatform(platform) {
				openAIHandler(c)
				return
			}
			anthropicHandler(c)
		}
	}
	countTokensHandler := func(c *gin.Context) {
		platform := routePlatformForCountTokensEndpoint(c)
		middleware.SetRoutePlatformIntent(c, platform)
		if platform == service.PlatformOpenAI {
			h.OpenAIGateway.CountTokens(c)
			return
		}
		if platform == service.PlatformGrok {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.JSON(http.StatusNotFound, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "not_found_error",
					"message": "Token counting is not supported for this platform",
				},
			})
			return
		}
		h.Gateway.CountTokens(c)
	}
	imagesHandler := func(c *gin.Context) {
		platform := routePlatformForOpenAIMediaEndpoint(c)
		middleware.SetRoutePlatformIntent(c, platform)
		if platform == service.PlatformGrok {
			h.OpenAIGateway.GrokImages(c)
			return
		}
		if platform == service.PlatformOpenAI {
			h.OpenAIGateway.Images(c)
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Images API is not supported for this platform",
			},
		})
	}
	videoGenerationHandler := func(c *gin.Context) {
		if hasAPIKeyPlatform(c, service.PlatformGrok) {
			middleware.SetRoutePlatformIntent(c, service.PlatformGrok)
			h.OpenAIGateway.GrokVideoGeneration(c)
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Videos API is not supported for this platform",
			},
		})
	}
	videoStatusHandler := func(c *gin.Context) {
		if hasAPIKeyPlatform(c, service.PlatformGrok) {
			middleware.SetRoutePlatformIntent(c, service.PlatformGrok)
			h.OpenAIGateway.GrokVideoStatus(c)
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"type":    "not_found_error",
				"message": "Videos API is not supported for this platform",
			},
		})
	}
	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(bodyLimit)
	gateway.Use(clientRequestID)
	gateway.Use(opsErrorLogger)
	gateway.Use(endpointNorm)
	gateway.Use(gin.HandlerFunc(apiKeyAuth))
	gateway.Use(requireGroupAnthropic)
	{
		// /v1/messages: auto-route based on group platform
		gateway.POST("/messages", dispatchMessages)
		// /v1/messages/count_tokens: OpenAI uses Anthropic-compat bridge; other
		// OpenAI-compatible platforms keep the prior unsupported response.
		gateway.POST("/messages/count_tokens", countTokensHandler)
		gateway.GET("/models", h.Gateway.Models)
		gateway.GET("/usage", h.Gateway.Usage)
		// OpenAI Responses API: auto-route based on group platform
		gateway.POST("/responses", dispatchOpenAICompatible(h.OpenAIGateway.Responses, h.Gateway.Responses))
		gateway.POST("/responses/*subpath", dispatchOpenAICompatible(h.OpenAIGateway.Responses, h.Gateway.Responses))
		gateway.GET("/responses", func(c *gin.Context) {
			middleware.SetRoutePlatformIntent(c, routePlatformForOpenAICompatibleEndpoint(c))
			h.OpenAIGateway.ResponsesWebSocket(c)
		})
		// OpenAI Chat Completions API: auto-route based on group platform
		gateway.POST("/chat/completions", dispatchOpenAICompatible(h.OpenAIGateway.ChatCompletions, h.Gateway.ChatCompletions))
		gateway.POST("/embeddings", func(c *gin.Context) {
			if !hasAPIKeyPlatform(c, service.PlatformOpenAI) {
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
				c.JSON(http.StatusNotFound, gin.H{
					"error": gin.H{
						"type":    "not_found_error",
						"message": "Embeddings API is not supported for this platform",
					},
				})
				return
			}
			h.OpenAIGateway.Embeddings(c)
		})
		gateway.POST("/images/generations", imagesHandler)
		gateway.POST("/images/edits", imagesHandler)
		gateway.POST("/videos/generations", videoGenerationHandler)
		gateway.GET("/videos/:request_id", videoStatusHandler)
	}

	// Gemini 原生 API 兼容层（Gemini SDK/CLI 直连）
	gemini := r.Group("/v1beta")
	gemini.Use(bodyLimit)
	gemini.Use(clientRequestID)
	gemini.Use(opsErrorLogger)
	gemini.Use(endpointNorm)
	gemini.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	gemini.Use(requireGroupGoogle)
	{
		gemini.GET("/models", h.Gateway.GeminiV1BetaListModels)
		gemini.GET("/models/:model", h.Gateway.GeminiV1BetaGetModel)
		// Gin treats ":" as a param marker, but Gemini uses "{model}:{action}" in the same segment.
		gemini.POST("/models/*modelAction", h.Gateway.GeminiV1BetaModels)
	}

	// OpenAI Responses API（不带v1前缀的别名）— auto-route based on group platform
	responsesHandler := dispatchOpenAICompatible(h.OpenAIGateway.Responses, h.Gateway.Responses)
	r.POST("/responses", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, responsesHandler)
	r.POST("/responses/*subpath", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, responsesHandler)
	r.GET("/responses", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, func(c *gin.Context) {
		middleware.SetRoutePlatformIntent(c, routePlatformForOpenAICompatibleEndpoint(c))
		h.OpenAIGateway.ResponsesWebSocket(c)
	})
	codexDirect := r.Group("/backend-api/codex")
	codexDirect.Use(bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic)
	{
		codexDirect.POST("/responses", responsesHandler)
		codexDirect.POST("/responses/*subpath", responsesHandler)
		codexDirect.GET("/responses", func(c *gin.Context) {
			middleware.SetRoutePlatformIntent(c, routePlatformForOpenAICompatibleEndpoint(c))
			h.OpenAIGateway.ResponsesWebSocket(c)
		})
	}
	// OpenAI Chat Completions API（不带v1前缀的别名）— auto-route based on group platform
	r.POST("/chat/completions", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, dispatchOpenAICompatible(h.OpenAIGateway.ChatCompletions, h.Gateway.ChatCompletions))
	r.POST("/embeddings", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, func(c *gin.Context) {
		if !hasAPIKeyPlatform(c, service.PlatformOpenAI) {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"type":    "not_found_error",
					"message": "Embeddings API is not supported for this platform",
				},
			})
			return
		}
		h.OpenAIGateway.Embeddings(c)
	})
	r.POST("/images/generations", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, imagesHandler)
	r.POST("/images/edits", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, imagesHandler)
	r.POST("/videos/generations", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, videoGenerationHandler)
	r.GET("/videos/:request_id", bodyLimit, clientRequestID, opsErrorLogger, endpointNorm, gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, videoStatusHandler)

	// Antigravity 模型列表
	r.GET("/antigravity/models", gin.HandlerFunc(apiKeyAuth), requireGroupAnthropic, h.Gateway.AntigravityModels)

	// Antigravity 专用路由（仅使用 antigravity 账户，不混合调度）
	antigravityV1 := r.Group("/antigravity/v1")
	antigravityV1.Use(bodyLimit)
	antigravityV1.Use(clientRequestID)
	antigravityV1.Use(opsErrorLogger)
	antigravityV1.Use(endpointNorm)
	antigravityV1.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1.Use(gin.HandlerFunc(apiKeyAuth))
	antigravityV1.Use(requireGroupAnthropic)
	{
		antigravityV1.POST("/messages", h.Gateway.Messages)
		antigravityV1.POST("/messages/count_tokens", h.Gateway.CountTokens)
		antigravityV1.GET("/models", h.Gateway.AntigravityModels)
		antigravityV1.GET("/usage", h.Gateway.Usage)
	}

	antigravityV1Beta := r.Group("/antigravity/v1beta")
	antigravityV1Beta.Use(bodyLimit)
	antigravityV1Beta.Use(clientRequestID)
	antigravityV1Beta.Use(opsErrorLogger)
	antigravityV1Beta.Use(endpointNorm)
	antigravityV1Beta.Use(middleware.ForcePlatform(service.PlatformAntigravity))
	antigravityV1Beta.Use(middleware.APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, cfg))
	antigravityV1Beta.Use(requireGroupGoogle)
	{
		antigravityV1Beta.GET("/models", h.Gateway.GeminiV1BetaListModels)
		antigravityV1Beta.GET("/models/:model", h.Gateway.GeminiV1BetaGetModel)
		antigravityV1Beta.POST("/models/*modelAction", h.Gateway.GeminiV1BetaModels)
	}

}

// getGroupPlatform extracts the group platform from the API Key stored in context.
func getGroupPlatform(c *gin.Context) string {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

func hasAPIKeyPlatform(c *gin.Context, platform string) bool {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok {
		return false
	}
	return service.APIKeyHasCandidateGroup(apiKey, platform)
}

func routePlatformForMessagesEndpoint(c *gin.Context) string {
	model := normalizedRequestModel(c)
	if requestModelLooksGrok(model) && hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	if requestModelLooksOpenAI(model) && hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformAnthropic) {
		return service.PlatformAnthropic
	}
	if hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	return getGroupPlatform(c)
}

func routePlatformForOpenAICompatibleEndpoint(c *gin.Context) string {
	model := normalizedRequestModel(c)
	if requestModelLooksAnthropic(model) && hasAPIKeyPlatform(c, service.PlatformAnthropic) {
		return service.PlatformAnthropic
	}
	if requestModelLooksGrok(model) && hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	if requestModelLooksOpenAI(model) && hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	if hasAPIKeyPlatform(c, service.PlatformAnthropic) {
		return service.PlatformAnthropic
	}
	return getGroupPlatform(c)
}

func routePlatformForCountTokensEndpoint(c *gin.Context) string {
	model := normalizedRequestModel(c)
	if requestModelLooksAnthropic(model) && hasAPIKeyPlatform(c, service.PlatformAnthropic) {
		return service.PlatformAnthropic
	}
	if requestModelLooksOpenAI(model) && hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if requestModelLooksGrok(model) && hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	if hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformAnthropic) {
		return service.PlatformAnthropic
	}
	if hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	return getGroupPlatform(c)
}

func routePlatformForOpenAIMediaEndpoint(c *gin.Context) string {
	model := normalizedRequestModel(c)
	if requestModelLooksGrok(model) && hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	if hasAPIKeyPlatform(c, service.PlatformOpenAI) {
		return service.PlatformOpenAI
	}
	if hasAPIKeyPlatform(c, service.PlatformGrok) {
		return service.PlatformGrok
	}
	return getGroupPlatform(c)
}

func isOpenAICompatibleRoutePlatform(platform string) bool {
	return platform == service.PlatformOpenAI || platform == service.PlatformGrok
}

func requestModelLooksOpenAICompatible(c *gin.Context) bool {
	model := normalizedRequestModel(c)
	return requestModelLooksOpenAI(model) || requestModelLooksGrok(model)
}

func normalizedRequestModel(c *gin.Context) string {
	return strings.ToLower(strings.TrimSpace(peekJSONRequestModel(c)))
}

func requestModelLooksOpenAI(model string) bool {
	return strings.HasPrefix(model, "gpt-") ||
		strings.HasPrefix(model, "chatgpt-") ||
		strings.HasPrefix(model, "codex") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") ||
		strings.HasPrefix(model, "openai/")
}

func requestModelLooksGrok(model string) bool {
	return strings.HasPrefix(model, "grok") ||
		strings.HasPrefix(model, "xai/")
}

func requestModelLooksAnthropic(model string) bool {
	return strings.HasPrefix(model, "claude") ||
		strings.HasPrefix(model, "anthropic/claude") ||
		strings.HasPrefix(model, "sonnet") ||
		strings.HasPrefix(model, "opus") ||
		strings.HasPrefix(model, "haiku")
}

func peekJSONRequestModel(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	return gjson.GetBytes(body, "model").String()
}
