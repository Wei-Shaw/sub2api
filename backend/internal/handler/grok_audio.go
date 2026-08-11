package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GrokRealtime exposes xAI's native Voice Realtime WebSocket.
// Only Grok-platform API keys may use this endpoint.
func (h *OpenAIGatewayHandler) GrokRealtime(c *gin.Context) {
	if c == nil || c.Request == nil || !isOpenAIWSUpgradeRequest(c.Request) {
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGrok {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Realtime API is not supported for this platform")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		c.Request.Context(),
		apiKey.GroupID,
		"",
		"",
		"grok-4.5",
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		// Grok only advertises chat_completions + media capabilities on HEAD.
		service.OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		false,
		service.PlatformGrok,
	)
	if err != nil || selection == nil || selection.Account == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Grok accounts")
		return
	}

	var streamStarted bool
	reqLog := requestLogger(c, "handler.openai_gateway.grok_realtime")
	release, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, true, &streamStarted, reqLog)
	if slotStatus != openAISlotAcquireOK {
		return
	}
	defer release()

	token, _, err := h.gatewayService.GetRequestCredential(c.Request.Context(), c, selection.Account)
	if err != nil {
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Grok credential unavailable")
		return
	}

	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	model := c.Query("model")
	if strings.TrimSpace(model) == "" {
		model = "grok-voice-latest"
	}
	started := time.Now()
	proxyErr := h.gatewayService.ProxyGrokRealtime(c.Request.Context(), c, conn, selection.Account, token, model)
	elapsed := time.Since(started)
	if proxyErr != nil {
		reqLog.Info("grok_realtime.proxy_failed", zap.Error(proxyErr))
		if !isExpectedGrokRealtimeClose(proxyErr) {
			_ = conn.Close(coderws.StatusInternalError, "upstream realtime websocket failed")
			return
		}
	}
	// A relay normally returns a close error when either side closes normally.
	// Those sessions still consumed upstream audio time and must be billed.
	if elapsed > 0 {
		result := &service.OpenAIForwardResult{
			// One durable id per WS session so retries cannot collapse or double under client ids.
			RequestID:  service.StableGrokRealtimeBillingRequestID(""),
			Model:      model,
			Duration:   elapsed,
			AudioUsage: &service.AudioUsage{Mode: "realtime", DurationOrUnits: elapsed.Minutes()},
		}
		h.recordGrokVoiceUsage(c, apiKey, selection.Account, subscription, "realtime", nil, result)
	}
}

func isExpectedGrokRealtimeClose(err error) bool {
	if err == nil {
		return true
	}
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway,
		coderws.StatusNoStatusRcvd, coderws.StatusAbnormalClosure:
		return true
	default:
		return false
	}
}

// GrokVoice handles xAI Voice HTTP endpoints. endpoint is "tts", "stt", or "custom-voices".
func (h *OpenAIGatewayHandler) GrokVoice(c *gin.Context, endpoint string) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGrok {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Voice API is not supported for this platform")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	body, err := readGrokVoiceGatewayBody(c)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if endpoint == "tts" {
		subject, _ := middleware2.GetAuthSubjectFromContext(c)
		reqLog := requestLogger(c, "handler.openai_gateway.grok_voice", zap.String("endpoint", endpoint))
		// TTS bodies use {"input":"..."} (and variants). Normalize to chat messages so
		// content moderation extractors see the spoken text.
		auditBody := body
		if input := extractGrokTTSInputText(body); input != "" {
			if b, err := json.Marshal(map[string]any{
				"messages": []map[string]any{{"role": "user", "content": input}},
			}); err == nil {
				auditBody = b
			}
		}
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, "grok-4.5", auditBody); decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
			return
		}
	}
	contentType := c.GetHeader("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}

	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	reqLog := requestLogger(c, "handler.openai_gateway.grok_voice", zap.String("endpoint", endpoint))
	selectionModel := "grok-4.5"

	for attempts := 0; attempts < 4; attempts++ {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			"",
			"",
			selectionModel,
			failed,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityChatCompletions,
			false,
			false,
			false,
			service.PlatformGrok,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if last != nil {
				h.handleFailoverExhausted(c, last, false)
			} else {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Grok accounts")
			}
			return
		}
		account := selection.Account
		var started bool
		release, status := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &started, reqLog)
		if status == openAISlotAcquireProfitVetoed {
			failed[account.ID] = struct{}{}
			continue
		}
		if status != openAISlotAcquireOK {
			// Failed already wrote error response (or transient reject).
			if status == openAISlotAcquireFailed && len(failed) == 0 {
				// Slot path wrote the response; stop.
				return
			}
			failed[account.ID] = struct{}{}
			continue
		}
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			defer release()
			return h.gatewayService.ForwardGrokVoice(c.Request.Context(), c, account, endpoint, body, contentType)
		}()
		if forwardErr == nil {
			h.recordGrokVoiceUsage(c, apiKey, account, subscription, endpoint, body, result)
			return
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(forwardErr, &failoverErr) && failoverErr.ShouldRetryNextAccount() {
			failed[account.ID] = struct{}{}
			last = failoverErr
			continue
		}
		// Non-failover errors: handleGrokMediaErrorResponse / transport already wrote response.
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

// recordGrokVoiceUsage bills TTS/STT/realtime via group audio prices when AudioUsage is set.
func (h *OpenAIGatewayHandler) recordGrokVoiceUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	endpoint string,
	body []byte,
	result *service.OpenAIForwardResult,
) {
	if h == nil || c == nil || apiKey == nil || account == nil || result == nil {
		return
	}
	if result.AudioUsage == nil {
		return
	}
	// Ensure forced durable request ids even if callers forget (realtime/tts/stt money path).
	if mode := strings.TrimSpace(result.AudioUsage.Mode); mode == "realtime" {
		result.RequestID = service.StableGrokRealtimeBillingRequestID(result.RequestID)
	} else {
		result.RequestID = service.StableGrokAudioBillingRequestID(result.RequestID)
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	if requestPayloadHash == "" {
		requestPayloadHash = service.HashUsageRequestPayload([]byte(endpoint))
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	model := strings.TrimSpace(result.Model)
	if model == "" {
		model = endpoint
	}

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
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
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: clientRequestedUsageFields(c, service.ChannelMappingResult{}, model, result.UpstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.grok_voice"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("endpoint", endpoint),
				zap.Int64("account_id", account.ID),
			).Error("grok_voice.record_usage_failed", zap.Error(err))
		}
	})
}

func readGrokVoiceGatewayBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("request body is required")
	}
	if c.Request.Body == nil {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodDelete {
			return nil, nil
		}
		return nil, errors.New("request body is required")
	}
	return io.ReadAll(c.Request.Body)
}

// extractGrokTTSInputText pulls the primary spoken text from a TTS JSON body.
func extractGrokTTSInputText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"input", "text", "prompt"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
