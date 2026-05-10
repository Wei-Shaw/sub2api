package handler

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// openaiChatCompletionsPipeline handles OpenAI ChatCompletions requests
// through the GatewayPipeline. Activated by PipelineEnabled().
func (h *OpenAIGatewayHandler) openaiChatCompletionsPipeline(c *gin.Context) {
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	parse := h.buildOpenAICCParseFunc(c)
	record := h.buildOpenAICCRecordFunc(c)

	err := h.pipeline.Execute(c, gateway.ProtocolChatCompletions, "", parse, record)
	if err != nil {
		h.handleOpenAIPipelineError(c, err)
	}
}

// openaiResponsesPipeline handles OpenAI Responses requests through
// the GatewayPipeline. Activated by PipelineEnabled().
func (h *OpenAIGatewayHandler) openaiResponsesPipeline(c *gin.Context) {
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	parse := h.buildOpenAIResponsesParseFunc(c)
	record := h.buildOpenAIResponsesRecordFunc(c)

	err := h.pipeline.Execute(c, gateway.ProtocolOpenAI, "", parse, record)
	if err != nil {
		h.handleOpenAIPipelineError(c, err)
	}
}

// buildOpenAICCParseFunc builds the ParseRequestFunc for OpenAI
// ChatCompletions. It validates JSON, extracts model/stream, and
// resolves the prompt cache key and default mapped model.
func (h *OpenAIGatewayHandler) buildOpenAICCParseFunc(c *gin.Context) gateway.ParseRequestFunc {
	return func(body []byte) (*gateway.ForwardRequest, error) {
		if !gjson.ValidBytes(body) {
			return nil, errors.New("invalid JSON")
		}

		modelResult := gjson.GetBytes(body, "model")
		if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
			return nil, errors.New("model is required")
		}
		model := modelResult.String()
		stream := gjson.GetBytes(body, "stream").Bool()

		// Extract OpenAI-specific fields for the provider
		promptCacheKey := h.gatewayService.ExtractSessionID(c, body)
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)
		defaultMappedModel := resolveOpenAIForwardDefaultMappedModel(apiKey, "")

		return &gateway.ForwardRequest{
			Model:              model,
			Stream:             stream,
			RawBody:            body,
			GinContext:         c,
			PromptCacheKey:     promptCacheKey,
			DefaultMappedModel: defaultMappedModel,
		}, nil
	}
}

// buildOpenAIResponsesParseFunc builds the ParseRequestFunc for OpenAI
// Responses. It validates JSON and extracts model/stream.
func (h *OpenAIGatewayHandler) buildOpenAIResponsesParseFunc(c *gin.Context) gateway.ParseRequestFunc {
	return func(body []byte) (*gateway.ForwardRequest, error) {
		if !gjson.ValidBytes(body) {
			return nil, errors.New("invalid JSON")
		}

		modelResult := gjson.GetBytes(body, "model")
		if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
			return nil, errors.New("model is required")
		}
		model := modelResult.String()
		stream := gjson.GetBytes(body, "stream").Bool()

		return &gateway.ForwardRequest{
			Model:      model,
			Stream:     stream,
			RawBody:    body,
			GinContext: c,
		}, nil
	}
}

// buildOpenAICCRecordFunc builds the RecordUsageFunc for OpenAI
// ChatCompletions. It maps the ForwardResult to OpenAIRecordUsageInput
// and submits the recording task asynchronously.
func (h *OpenAIGatewayHandler) buildOpenAICCRecordFunc(c *gin.Context) gateway.RecordUsageFunc {
	return func(ctx context.Context, account *service.Account, result *gateway.ForwardResult) error {
		return h.openaiRecordUsage(c, account, result)
	}
}

// buildOpenAIResponsesRecordFunc builds the RecordUsageFunc for OpenAI
// Responses.
func (h *OpenAIGatewayHandler) buildOpenAIResponsesRecordFunc(c *gin.Context) gateway.RecordUsageFunc {
	return func(ctx context.Context, account *service.Account, result *gateway.ForwardResult) error {
		return h.openaiRecordUsage(c, account, result)
	}
}

// openaiRecordUsage is the shared usage recording logic for both OpenAI
// ChatCompletions and Responses pipeline paths.
func (h *OpenAIGatewayHandler) openaiRecordUsage(
	c *gin.Context,
	account *service.Account,
	result *gateway.ForwardResult,
) error {
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

	reqModel := result.Model
	if raw, ok := c.Get("ops_request_body"); ok {
		if b, ok := raw.([]byte); ok {
			if m := gjson.GetBytes(b, "model"); m.Exists() {
				reqModel = m.String()
			}
		}
	}

	channelMapping := h.resolveOpenAIChannelMapping(c, apiKey, reqModel)

	h.submitUsageRecordTask(func(recordCtx context.Context) {
		if err := h.gatewayService.RecordUsage(recordCtx, &service.OpenAIRecordUsageInput{
			Result:              toOpenAIForwardResult(result),
			APIKey:              apiKey,
			User:                apiKey.User,
			Account:             account,
			Subscription:        subscription,
			InboundEndpoint:     inboundEndpoint,
			UpstreamEndpoint:    upstreamEndpoint,
			UserAgent:           userAgent,
			IPAddress:           clientIP,
			APIKeyService:       h.apiKeyService,
			ChannelUsageFields:  channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
			ServiceQuotaRequest: service.ServiceQuotaCheckRequest{Model: reqModel, AccountID: account.ID, ChannelID: channelMapping.ChannelID},
		}); err != nil {
			requestLogger(c, "handler.openai_gateway.pipeline").
				Error("openai.pipeline.record_usage_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
		}
	})
	return nil
}

// resolveOpenAIChannelMapping resolves the channel mapping for OpenAI
// usage recording via the OpenAIGatewayService.
func (h *OpenAIGatewayHandler) resolveOpenAIChannelMapping(
	c *gin.Context,
	apiKey *service.APIKey,
	reqModel string,
) service.ChannelMappingResult {
	if apiKey == nil {
		return service.ChannelMappingResult{}
	}
	mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(
		c.Request.Context(), apiKey.GroupID, apiKey.GroupPlatform(), reqModel,
	)
	return mapping
}

// handleOpenAIPipelineError translates pipeline errors into OpenAI-format
// error responses. Uses the shared classifyPipelineError from
// gateway_pipeline_helpers.go.
func (h *OpenAIGatewayHandler) handleOpenAIPipelineError(c *gin.Context, err error) {
	if c.Writer.Size() > 0 {
		return // bytes already written to client, cannot send error
	}

	status, code, message, metadata := classifyPipelineError(err)
	if len(metadata) > 0 {
		h.errorResponseWithMetadata(c, status, code, message, metadata)
	} else {
		h.errorResponse(c, status, code, message)
	}
}
