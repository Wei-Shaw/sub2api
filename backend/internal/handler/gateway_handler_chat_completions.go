package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ChatCompletions handles OpenAI Chat Completions API endpoint.
// POST /v1/chat/completions
// For Anthropic platform groups: converts CC → Anthropic Messages → forwards to Anthropic → converts back to CC.
// For Gemini platform groups: converts CC → Anthropic Messages → forwards to Gemini (via geminiCompatService) → converts back to CC.
func (h *GatewayHandler) ChatCompletions(c *gin.Context) {
	streamStarted := false

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	subjectPtr := &subject
	reqLog := requestLogger(
		c,
		"handler.gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Determine platform
	platform := ""
	forcePlatform, hasForcePlatform := middleware2.GetForcePlatformFromContext(c)
	if hasForcePlatform {
		platform = forcePlatform
	} else if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}

	// Read request body
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.chatCompletionsErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false, body)

	// Validate JSON
	if !gjson.ValidBytes(body) {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model and stream
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()
	reqStream := gjson.GetBytes(body, "stream").Bool()
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream), zap.String("platform", platform))

	setOpsRequestContext(c, reqModel, reqStream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// Claude Code only restriction
	if apiKey.Group != nil && apiKey.Group.ClaudeCodeOnly {
		h.chatCompletionsErrorResponse(c, http.StatusForbidden, "permission_error",
			"This group is restricted to Claude Code clients (/v1/messages only)")
		return
	}

	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, reqModel, body); decision != nil && decision.Blocked {
		h.chatCompletionsErrorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
	}

	// Error passthrough binding
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	// 1. Acquire user concurrency slot
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("gateway.cc.user_wait_counter_increment_failed", zap.Error(err))
	} else if !canWait {
		h.chatCompletionsErrorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	if err == nil && canWait {
		waitCounted = true
	}
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		}
	}()

	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		reqLog.Warn("gateway.cc.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. Re-check billing
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("gateway.cc.billing_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.chatCompletionsErrorResponse(c, status, code, message)
		return
	}

	// Parse request for session hash
	parsedReq, _ := service.ParseGatewayRequest(body, "chat_completions")
	if parsedReq == nil {
		parsedReq = &service.ParsedRequest{Model: reqModel, Stream: reqStream, Body: body}
	}
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// === Gemini platform: dedicated forwarding path ===
	if platform == service.PlatformGemini {
		h.chatCompletionsGemini(c, reqLog, apiKey, subjectPtr, subscription, body, reqModel, reqStream, parsedReq, sessionHash, &channelMapping, &streamStarted, requestStart)
		return
	}

	// === Non-Gemini platform: original ForwardAsChatCompletions path ===
	h.chatCompletionsAnthropic(c, reqLog, apiKey, subjectPtr, subscription, body, reqModel, reqStream, parsedReq, sessionHash, &channelMapping, &streamStarted, requestStart)
}

// chatCompletionsAnthropic is the original ChatCompletions path for Anthropic/OpenAI platforms.
// It converts CC → Anthropic Messages, forwards to Anthropic upstream, and converts back to CC.
func (h *GatewayHandler) chatCompletionsAnthropic(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject *middleware2.AuthSubject, subscription *service.UserSubscription, body []byte, reqModel string, reqStream bool, parsedReq *service.ParsedRequest, sessionHash string, channelMapping *service.ChannelMappingResult, streamStarted *bool, requestStart time.Time) {
	// 3. Account selection + failover loop
	fs := NewFailoverState(h.maxAccountSwitches, false)

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionHash, reqModel, fs.FailedAccountIDs, "", int64(0))
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error())
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, *streamStarted)
				} else {
					h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
				}
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// 4. Acquire account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				reqStream,
				streamStarted,
			)
			if err != nil {
				reqLog.Warn("gateway.cc.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", *streamStarted)
				return
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 5. Forward request
		writerSizeBeforeForward := c.Writer.Size()
		forwardBody := body
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
		}
		result, err := h.gatewayService.ForwardAsChatCompletions(c.Request.Context(), c, account, forwardBody, parsedReq)

		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleCCFailoverExhausted(c, failoverErr, true)
					return
				}
				action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
				switch action {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, *streamStarted)
					return
				case FailoverCanceled:
					return
				}
			}
			h.ensureForwardErrorResponse(c, *streamStarted)
			reqLog.Error("gateway.cc.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			return
		}

		// 6. Record usage
		h.recordChatCompletionsUsage(c, result, apiKey, account, subscription, body, reqModel, channelMapping)
		return
	}
}

// chatCompletionsGemini handles CC requests for Gemini platform groups.
// It converts CC → Anthropic Messages, forwards via geminiCompatService.Forward(),
// captures the Claude response, and converts it back to Chat Completions format.
// For non-streaming: buffers the full response, then converts Claude JSON → CC JSON.
// For streaming: currently converts via SSE event re-writing (Claude SSE → CC SSE).
func (h *GatewayHandler) chatCompletionsGemini(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject *middleware2.AuthSubject, subscription *service.UserSubscription, body []byte, reqModel string, reqStream bool, parsedReq *service.ParsedRequest, sessionHash string, channelMapping *service.ChannelMappingResult, streamStarted *bool, requestStart time.Time) {
	// Convert CC → Anthropic Messages format
	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &ccReq); err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse chat completions request")
		return
	}
	originalModel := ccReq.Model

	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert chat completions to responses: "+err.Error())
		return
	}
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to convert responses to anthropic: "+err.Error())
		return
	}

	// Force non-streaming for Gemini Forward (we buffer and convert)
	anthropicReq.Stream = false
	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "Failed to marshal anthropic request")
		return
	}

	// Account selection + failover
	fs := NewFailoverState(h.maxAccountSwitchesGemini, false)

	sessionKey := sessionHash
	if sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
	}

	for {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, reqModel, fs.FailedAccountIDs, "", int64(0))
		if err != nil {
			if len(fs.FailedAccountIDs) == 0 {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error())
				return
			}
			action := fs.HandleSelectionExhausted(c.Request.Context())
			switch action {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				if fs.LastFailoverErr != nil {
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, *streamStarted)
				} else {
					h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "All available accounts exhausted")
				}
				return
			}
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// Acquire account concurrency slot
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				h.chatCompletionsErrorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c, account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, reqStream, streamStarted,
			)
			if err != nil {
				reqLog.Warn("gateway.cc.gemini.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", *streamStarted)
				return
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// Buffer geminiCompatService.Forward output by swapping c.Writer
		forwardBody := anthropicBody
		if channelMapping.Mapped {
			forwardBody = h.gatewayService.ReplaceModelInBody(anthropicBody, channelMapping.MappedModel)
		}

		// Swap in a buffer to capture Forward's Claude-format output
		buf := newResponseBuffer()
		origWriter := c.Writer
		c.Writer = buf

		result, err := h.geminiCompatService.Forward(c.Request.Context(), c, account, forwardBody)

		// Restore original writer
		c.Writer = origWriter

		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
				switch action {
				case FailoverContinue:
					continue
				case FailoverExhausted:
					h.handleCCFailoverExhausted(c, fs.LastFailoverErr, *streamStarted)
					return
				case FailoverCanceled:
					return
				}
			}
			// Forward already wrote an error to the buffer; pass it through as CC error
			if buf.Written() {
				c.Data(buf.status, buf.Header().Get("Content-Type"), buf.body.Bytes())
				return
			}
			h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "Upstream request failed")
			return
		}

		// Convert Claude response → Chat Completions response
		claudeBody := buf.body.Bytes()
		if len(claudeBody) == 0 {
			h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "Empty upstream response")
			return
		}

		if reqStream {
			// Streaming CC: convert buffered Claude response to CC chunked SSE
			h.writeCCStreamFromClaudeMessage(c, claudeBody, originalModel, result)
		} else {
			// Non-streaming CC: convert Claude JSON → CC JSON
			h.writeCCFromClaudeMessage(c, claudeBody, originalModel, result)
		}

		// Record usage
		h.recordChatCompletionsUsage(c, result, apiKey, account, subscription, body, originalModel, channelMapping)
		return
	}
}

// writeCCFromClaudeMessage converts a buffered Claude Messages response to OpenAI Chat Completions format.
func (h *GatewayHandler) writeCCFromClaudeMessage(c *gin.Context, claudeBody []byte, originalModel string, result *service.ForwardResult) {
	var claudeResp apicompat.AnthropicResponse
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "Failed to convert upstream response")
		return
	}

	// Chain: Claude → Responses → Chat Completions
	responsesResp := apicompat.AnthropicToResponsesResponse(&claudeResp)
	ccResp := apicompat.ResponsesToChatCompletions(responsesResp, originalModel)

	c.JSON(http.StatusOK, ccResp)
}

// writeCCStreamFromClaudeMessage converts a buffered Claude Messages response to OpenAI Chat Completions SSE stream.
func (h *GatewayHandler) writeCCStreamFromClaudeMessage(c *gin.Context, claudeBody []byte, originalModel string, result *service.ForwardResult) {
	var claudeResp apicompat.AnthropicResponse
	if err := json.Unmarshal(claudeBody, &claudeResp); err != nil {
		h.chatCompletionsErrorResponse(c, http.StatusBadGateway, "server_error", "Failed to convert upstream response")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.chatCompletionsErrorResponse(c, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	// Build CC SSE from Claude response
	// Claude response has content blocks; CC stream has chat.completion.chunk events
	id := "chatcmpl-" + randomHex(12)

	// Initial chunk with role
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   originalModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{"role": "assistant", "content": ""},
				"finish_reason": nil,
			},
		},
	}
	writeCCSSE(c.Writer, chunk)
	flusher.Flush()

	// Content chunks
	var fullContent strings.Builder
	for _, block := range claudeResp.Content {
		if block.Type == "text" && block.Text != "" {
			contentChunk := map[string]any{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   originalModel,
				"choices": []map[string]any{
					{
						"index":         0,
						"delta":         map[string]any{"content": block.Text},
						"finish_reason": nil,
					},
				},
			}
			writeCCSSE(c.Writer, contentChunk)
			flusher.Flush()
			fullContent.WriteString(block.Text)
		}
	}

	// Final chunk with finish_reason
	finishReason := "stop"
	if claudeResp.StopReason == "max_tokens" {
		finishReason = "length"
	}
	finalChunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   originalModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": finishReason,
			},
			},
	}
	writeCCSSE(c.Writer, finalChunk)
	flusher.Flush()

	_ = result
	_ = fullContent
}

// writeCCSSE writes a single SSE event in Chat Completions format.
func writeCCSSE(w gin.ResponseWriter, data any) {
	b, _ := json.Marshal(data)
	w.Write([]byte("data: "))
	w.Write(b)
	w.Write([]byte("\n\n"))
}

// randomHex generates a random hex string of the given byte length.
func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// recordChatCompletionsUsage records usage for a chat completions request.
func (h *GatewayHandler) recordChatCompletionsUsage(c *gin.Context, result *service.ForwardResult, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, body []byte, reqModel string, channelMapping *service.ChannelMappingResult) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

	h.submitUsageRecordTask(func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
		}); err != nil {
			// Usage recording is best-effort; don't fail the request.
		}
	})
}

// chatCompletionsErrorResponse writes an error in OpenAI Chat Completions format.
func (h *GatewayHandler) chatCompletionsErrorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// handleCCFailoverExhausted writes a failover-exhausted error in CC format.
func (h *GatewayHandler) handleCCFailoverExhausted(c *gin.Context, lastErr *service.UpstreamFailoverError, streamStarted bool) {
	if streamStarted {
		return
	}
	statusCode := http.StatusBadGateway
	if lastErr != nil && lastErr.StatusCode > 0 {
		statusCode = lastErr.StatusCode
	}
	h.chatCompletionsErrorResponse(c, statusCode, "server_error", "All available accounts exhausted")
}
