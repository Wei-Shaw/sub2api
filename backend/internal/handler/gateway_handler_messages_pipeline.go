package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// messagesPipeline handles Messages requests through the GatewayPipeline.
// This is the pipeline-based replacement for the legacy inline Messages
// handler. This is the sole request path (legacy handler code has been removed).
//
// Differences from chatCompletionsPipeline/responsesPipeline:
//   - Pre-pipeline intercept detection (warmup / suggestion / maxTokens1)
//   - Claude Code client detection and version check
//   - Thinking-enabled context propagation
//   - Single-account retry context for Antigravity groups
//   - Fallback group retry on PromptTooLongError
func (h *GatewayHandler) messagesPipeline(c *gin.Context) {
	if h.pipeline == nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Gateway pipeline not initialized")
		return
	}
	// Pre-pipeline setup: error passthrough
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	// Parse body early for intercept detection and Claude Code checks
	body, parsedReq, err := h.readAndParseMessagesBody(c)
	if err != nil {
		return // error already written
	}

	// Claude Code detection: set context before pipeline
	SetClaudeCodeClientContext(c, body, parsedReq)
	isClaudeCodeClient := service.IsClaudeCodeClient(c.Request.Context())

	// Version check: reject outdated Claude Code clients
	if !h.checkClaudeCodeVersion(c) {
		return
	}

	// Set max_tokens=1 + haiku detection context
	if isMaxTokensOneHaikuRequest(parsedReq.Model, parsedReq.MaxTokens, parsedReq.Stream) {
		ctx := service.WithIsMaxTokensOneHaikuRequest(c.Request.Context(), true, h.metadataBridgeEnabled())
		c.Request = c.Request.WithContext(ctx)
	}

	// Propagate thinking-enabled state for model-dimension rate limiting
	c.Request = c.Request.WithContext(
		service.WithThinkingEnabled(c.Request.Context(), parsedReq.ThinkingEnabled, h.metadataBridgeEnabled()),
	)

	// Single-account retry context for mixed-scheduling groups:
	// prevents 29s rate-limit flag on first 503 in single-account groups.
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	if apiKey != nil && h.gatewayService.IsSingleCompatibleAccountGroup(c.Request.Context(), apiKey.GroupID) {
		ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
		c.Request = c.Request.WithContext(ctx)
	}

	// Intercept detection: warmup / suggestion / maxTokens1
	if h.shouldInterceptMessages(c, body, parsedReq, isClaudeCodeClient) {
		return
	}

	// Share ForwardRequest pointer between parse and record closures so
	// record can read pipeline-mutated fields (e.g. ForceCacheBilling).
	var fwdReq *gateway.ForwardRequest
	parse := h.buildMessagesParseFunc(c, body, parsedReq, &fwdReq)
	record := h.buildMessagesRecordFunc(c, parsedReq, &fwdReq)
	forcePlatform := h.resolveMessagesForcePlatform(c)

	err = h.pipeline.Execute(c, gateway.ProtocolAnthropic, forcePlatform, parse, record)
	if err == nil {
		return
	}

	// Fallback group retry on PromptTooLongError
	if h.retryWithFallbackGroup(c, err, body, parsedReq, record) {
		return
	}

	h.handleMessagesPipelineError(c, err)
}

// readAndParseMessagesBody reads and parses the request body for Messages.
// On failure, writes an error response and returns a non-nil error.
func (h *GatewayHandler) readAndParseMessagesBody(c *gin.Context) ([]byte, *service.ParsedRequest, error) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		} else {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		}
		return nil, nil, err
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return nil, nil, errors.New("empty body")
	}

	parsedReq, err := service.ParseGatewayRequest(service.NewRequestBodyRef(body), domain.PlatformAnthropic)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, nil, err
	}
	if parsedReq.Model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, nil, errors.New("model is required")
	}
	return body, parsedReq, nil
}

// shouldInterceptMessages checks for warmup/suggestion/maxTokens1 intercept
// conditions. If an intercept is detected, sends the mock response and
// returns true.
func (h *GatewayHandler) shouldInterceptMessages(
	c *gin.Context,
	body []byte,
	parsed *service.ParsedRequest,
	isClaudeCodeClient bool,
) bool {
	interceptType := detectInterceptType(body, parsed.Model, parsed.MaxTokens, parsed.Stream, isClaudeCodeClient)
	if interceptType == InterceptTypeNone {
		return false
	}
	if parsed.Stream {
		sendMockInterceptStream(c, parsed.Model, interceptType)
	} else {
		sendMockInterceptResponse(c, parsed.Model, interceptType)
	}
	return true
}

// buildMessagesParseFunc builds the ParseRequestFunc for Messages.
// The body and parsed request are pre-computed so the pipeline does not
// re-read or re-parse them.
func (h *GatewayHandler) buildMessagesParseFunc(
	c *gin.Context,
	body []byte,
	parsed *service.ParsedRequest,
	fwdReqOut **gateway.ForwardRequest,
) gateway.ParseRequestFunc {
	return func(_ []byte) (*gateway.ForwardRequest, error) {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)

		parsed.GroupID = nil
		if apiKey != nil {
			parsed.GroupID = apiKey.GroupID
		}
		parsed.SessionContext = &service.SessionContext{
			ClientIP:  ip.GetClientIP(c),
			UserAgent: c.GetHeader("User-Agent"),
		}
		if apiKey != nil {
			parsed.SessionContext.APIKeyID = apiKey.ID
		}
		sessionHash := h.gatewayService.GenerateSessionHash(parsed)

		req := &gateway.ForwardRequest{
			Model:              parsed.Model,
			Stream:             parsed.Stream,
			RawBody:            body,
			GinContext:         c,
			MetadataUserID:     parsed.MetadataUserID,
			SessionHash:        sessionHash,
			IsClaudeCodeClient: service.IsClaudeCodeClient(c.Request.Context()),
			ThinkingEnabled:    parsed.ThinkingEnabled,
		}
		*fwdReqOut = req
		return req, nil
	}
}

// buildMessagesRecordFunc builds the RecordUsageFunc for Messages.
func (h *GatewayHandler) buildMessagesRecordFunc(
	c *gin.Context,
	parsed *service.ParsedRequest,
	fwdReq **gateway.ForwardRequest,
) gateway.RecordUsageFunc {
	return func(ctx context.Context, account *service.Account, result *gateway.ForwardResult) error {
		apiKey, _ := middleware2.GetAPIKeyFromContext(c)
		subscription, _ := middleware2.GetSubscriptionFromContext(c)

		userAgent := c.GetHeader("User-Agent")
		clientIP := ip.GetClientIP(c)
		inboundEndpoint := GetInboundEndpoint(c)
		upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

		reqModel := parsed.Model
		body := []byte(nil)
		if raw, ok := c.Get("ops_request_body"); ok {
			if b, ok := raw.([]byte); ok {
				body = b
			}
		}
		requestPayloadHash := service.HashUsageRequestPayload(body)

		channelMapping := h.resolveChannelMappingFromContext(c, apiKey, reqModel)

		// RPM increment (Anthropic OAuth/SetupToken accounts with base RPM > 0)
		if account.IsAnthropicOAuthOrSetupToken() && account.GetBaseRPM() > 0 {
			if err := h.gatewayService.IncrementAccountRPM(c.Request.Context(), account.ID); err != nil {
				requestLogger(c, "handler.gateway.messages.pipeline").
					Warn("gateway.messages.pipeline.rpm_increment_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(err),
					)
			}
		}

		svcResult := toServiceForwardResult(result)
		if svcResult.ReasoningEffort == nil {
			svcResult.ReasoningEffort = service.NormalizeClaudeOutputEffort(parsed.OutputEffort)
		}

		h.submitUsageRecordTask(func(recordCtx context.Context) {
			if err := h.gatewayService.RecordUsage(recordCtx, &service.RecordUsageInput{
				Result:              svcResult,
				ParsedRequest:       parsed,
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
				requestLogger(c, "handler.gateway.messages.pipeline").
					Error("gateway.messages.pipeline.record_usage_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(err),
					)
			}
		})
		return nil
	}
}

// resolveMessagesForcePlatform resolves the force-platform for Messages.
func (h *GatewayHandler) resolveMessagesForcePlatform(c *gin.Context) string {
	if fp, ok := middleware2.GetForcePlatformFromContext(c); ok {
		return fp
	}
	return ""
}

// retryWithFallbackGroup handles PromptTooLongError by re-executing the
// pipeline with a fallback group. Returns true if handled (caller should
// not write additional error responses).
func (h *GatewayHandler) retryWithFallbackGroup(
	c *gin.Context,
	pipelineErr error,
	body []byte,
	parsed *service.ParsedRequest,
	record gateway.RecordUsageFunc,
) bool {
	var promptTooLongErr *service.PromptTooLongError
	if !errors.As(pipelineErr, &promptTooLongErr) {
		return false
	}

	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	if apiKey == nil || apiKey.Group == nil {
		return false
	}
	fallbackGroupID := apiKey.Group.AnthropicConfig().FallbackGroupIDOnInvalidRequest
	if fallbackGroupID == nil || *fallbackGroupID <= 0 {
		return false
	}

	reqLog := requestLogger(c, "handler.gateway.messages.pipeline")
	reqLog.Warn("gateway.messages.pipeline.prompt_too_long_fallback",
		zap.Any("current_group_id", apiKey.GroupID),
		zap.Int64("fallback_group_id", *fallbackGroupID),
	)

	fallbackGroup, err := h.gatewayService.ResolveGroupByID(c.Request.Context(), *fallbackGroupID)
	if err != nil {
		reqLog.Warn("gateway.messages.pipeline.resolve_fallback_group_failed",
			zap.Int64("fallback_group_id", *fallbackGroupID),
			zap.Error(err),
		)
		writePromptTooLongClaudeError(c, promptTooLongErr)
		return true
	}

	// Validate fallback group constraints
	if fallbackGroup.Platform != service.PlatformAnthropic ||
		fallbackGroup.SubscriptionType == service.SubscriptionTypeSubscription ||
		fallbackGroup.AnthropicConfig().FallbackGroupIDOnInvalidRequest != nil {
		reqLog.Warn("gateway.messages.pipeline.fallback_group_invalid",
			zap.Int64("fallback_group_id", fallbackGroup.ID),
			zap.String("fallback_platform", fallbackGroup.Platform),
			zap.String("fallback_subscription_type", fallbackGroup.SubscriptionType),
		)
		writePromptTooLongClaudeError(c, promptTooLongErr)
		return true
	}

	// Re-execute pipeline with fallback group
	var fallbackFwdReq *gateway.ForwardRequest
	fallbackParse := h.buildMessagesParseFunc(c, body, parsed, &fallbackFwdReq)
	fallbackErr := h.pipeline.Execute(c, gateway.ProtocolAnthropic, "", fallbackParse, record)
	if fallbackErr != nil {
		h.handleMessagesPipelineError(c, fallbackErr)
	}
	return true
}

// handleMessagesPipelineError translates pipeline errors into Anthropic
// Messages error responses.
func (h *GatewayHandler) handleMessagesPipelineError(c *gin.Context, err error) {
	status, code, message, metadata := classifyPipelineErrorWithContext(c, service.PlatformAnthropic, err)

	// If streaming already started, write an SSE error event instead of an HTTP error
	if c.Writer.Size() > 0 {
		writeSSEErrorEvent(c, code, message, metadata)
		return
	}

	if len(metadata) > 0 {
		h.errorResponseWithMetadata(c, status, code, message, metadata)
	} else {
		h.errorResponse(c, status, code, message)
	}
}

// writePromptTooLongClaudeError writes a Claude API error response for a
// PromptTooLongError. This is a local helper that replaces the previous
// dependency on AntigravityGatewayService.WriteMappedClaudeError, which
// required a non-nil Account and pulled in Antigravity-specific ops logging.
//
// The upstream body typically contains a "prompt is too long" message.
// We extract the upstream message, map the status code to the appropriate
// Claude error type, and write the response in Claude API format.
func writePromptTooLongClaudeError(c *gin.Context, ptErr *service.PromptTooLongError) {
	upstreamMsg := strings.TrimSpace(service.ExtractUpstreamErrorMessage(ptErr.Body))
	errType, errMsg := mapUpstreamStatusToClaudeError(ptErr.StatusCode, upstreamMsg)
	c.JSON(ptErr.StatusCode, gin.H{
		"type":  "error",
		"error": gin.H{"type": errType, "message": errMsg},
	})
}

// mapUpstreamStatusToClaudeError maps an upstream HTTP status code to the
// corresponding Claude API error type and message. If the upstream message
// is non-empty it is used as-is (passthrough); otherwise a generic default
// is returned.
func mapUpstreamStatusToClaudeError(status int, upstreamMsg string) (errType, errMsg string) {
	switch status {
	case http.StatusBadRequest:
		errType = "invalid_request_error"
		if upstreamMsg != "" {
			errMsg = upstreamMsg
		} else {
			errMsg = "Invalid request"
		}
	case http.StatusUnauthorized:
		errType = "authentication_error"
		errMsg = "Upstream authentication failed"
	case http.StatusForbidden:
		errType = "permission_error"
		errMsg = "Upstream access forbidden"
	case http.StatusTooManyRequests:
		errType = "rate_limit_error"
		errMsg = "Upstream rate limit exceeded"
	case 529:
		errType = "overloaded_error"
		errMsg = "Upstream service overloaded"
	default:
		errType = "upstream_error"
		errMsg = "Upstream request failed"
	}
	return errType, errMsg
}
