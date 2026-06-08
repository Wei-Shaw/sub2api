package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// chatCompletionsPipeline handles ChatCompletions requests through the
// GatewayPipeline. This is the pipeline-based replacement for the legacy
// inline handler. This is the sole request path (legacy handler code has been removed).
func (h *GatewayHandler) chatCompletionsPipeline(c *gin.Context) {
	if h.pipeline == nil {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "Gateway pipeline not initialized")
		return
	}
	// Pre-pipeline setup: error passthrough + Claude Code only check
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok {
		if apiKey.Group != nil && apiKey.Group.AnthropicConfig().ClaudeCodeOnly {
			h.chatCompletionsErrorResponse(c, http.StatusForbidden,
				"permission_error",
				"This group is restricted to Claude Code clients (/v1/messages only)")
			return
		}
	}

	// Share ForwardRequest pointer between parse and record closures so
	// record can read pipeline-mutated fields (e.g. ForceCacheBilling).
	var fwdReq *gateway.ForwardRequest
	parse := h.buildCCParseFunc(c, &fwdReq)
	record := h.buildCCRecordFunc(c, &fwdReq)

	err := h.pipeline.Execute(c, gateway.ProtocolChatCompletions, "", parse, record)
	if err != nil {
		h.handleCCPipelineError(c, err)
	}
}

// buildCCParseFunc builds the ParseRequestFunc for ChatCompletions.
// It validates JSON, extracts model/stream, and returns a ForwardRequest.
func (h *GatewayHandler) buildCCParseFunc(c *gin.Context, fwdReqOut **gateway.ForwardRequest) gateway.ParseRequestFunc {
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

		req := &gateway.ForwardRequest{
			Model:      model,
			Stream:     stream,
			RawBody:    body,
			GinContext: c,
		}
		*fwdReqOut = req
		return req, nil
	}
}

// buildCCRecordFunc builds the RecordUsageFunc for ChatCompletions.
// It maps the pipeline's ForwardResult to service.RecordUsageInput and
// submits the recording task asynchronously via the worker pool.
func (h *GatewayHandler) buildCCRecordFunc(c *gin.Context, fwdReq **gateway.ForwardRequest) gateway.RecordUsageFunc {
	return func(ctx context.Context, account *service.Account, result *gateway.ForwardResult) error {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)
		subscription, _ := middleware2.GetSubscriptionFromContext(c)

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		// Reconstruct request model from the original body
		reqModel := result.Model
		body := []byte(nil)
		if raw, ok := c.Get("ops_request_body"); ok {
			if b, ok := raw.([]byte); ok {
				body = b
				if m := gjson.GetBytes(b, "model"); m.Exists() {
					reqModel = m.String()
				}
			}
		}
		requestPayloadHash := service.HashUsageRequestPayload(body)

		channelMapping := h.resolveChannelMappingFromContext(c, apiKey, reqModel)

		h.submitUsageRecordTask(func(recordCtx context.Context) {
			if err := h.gatewayService.RecordUsage(recordCtx, &service.RecordUsageInput{
				Result:              toServiceForwardResult(result),
				APIKey:              apiKey,
				User:                apiKey.User,
				Account:             account,
				Subscription:        subscription,
				InboundEndpoint:     inboundEndpoint,
				UpstreamEndpoint:    upstreamEndpoint,
				UserAgent:           userAgent,
				IPAddress:           clientIP,
				RequestPayloadHash:  requestPayloadHash,
				ForceCacheBilling:   *fwdReq != nil && (*fwdReq).ForceCacheBilling,
				APIKeyService:       h.apiKeyService,
				ChannelUsageFields:  channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				ServiceQuotaRequest: service.ServiceQuotaCheckRequest{Model: reqModel, AccountID: account.ID, ChannelID: channelMapping.ChannelID},
			}); err != nil {
				requestLogger(c, "handler.gateway.chat_completions.pipeline").
					Error("gateway.cc.pipeline.record_usage_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(err),
					)
			}
		})
		return nil
	}
}

// handleCCPipelineError translates pipeline errors into ChatCompletions
// error responses. It inspects the error chain to produce the appropriate
// HTTP status code and error type.
func (h *GatewayHandler) handleCCPipelineError(c *gin.Context, err error) {
	status, code, message, metadata := classifyPipelineErrorWithContext(c, service.PlatformAnthropic, err)

	// If streaming already started, write an SSE error event instead of an HTTP error
	if c.Writer.Size() > 0 {
		writeSSEErrorEvent(c, code, message, metadata)
		return
	}

	if len(metadata) > 0 {
		h.chatCompletionsErrorResponseWithMetadata(c, status, code, message, metadata)
	} else {
		h.chatCompletionsErrorResponse(c, status, code, message)
	}
}
