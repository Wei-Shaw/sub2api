package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// GatewayFailureReasonGeminiEmptyCompletion identifies an empty converted completion.
const GatewayFailureReasonGeminiEmptyCompletion GatewayFailureReason = "gemini_empty_completion"

const geminiEmptyCompletionClientMessage = "Gemini upstream returned an empty response; this attempt was not billed. Please retry."

const geminiEmptyCompletionAttemptsKey = "gemini_empty_completion_attempts"

// Empty replies may repeat for the same request. Try one alternative account,
// without retrying the same account or affecting its health.
const geminiEmptyCompletionMaxAttempts = 2

func (e *UpstreamFailoverError) IsGeminiEmptyCompletion() bool {
	return e != nil && e.Reason == GatewayFailureReasonGeminiEmptyCompletion
}

func geminiPartMapIsInlineImage(part map[string]any) bool {
	inlineData, ok := part["inlineData"].(map[string]any)
	if !ok {
		return false
	}
	mimeType, _ := inlineData["mimeType"].(string)
	data, _ := inlineData["data"].(string)
	return isGeminiInlineImageMIMEType(mimeType) && isValidBase64(data)
}

// Non-streaming converters omit thoughts. Valid images retain the existing
// image-only exception even for the Messages format, which omits inline data.
func geminiHasDeliverableParts(geminiResp map[string]any) bool {
	for _, part := range extractGeminiParts(geminiResp) {
		if text, ok := part["text"].(string); ok && !isGeminiThoughtPart(part) && strings.TrimSuffix(text, "\u0000") != "" {
			return true
		}
		if _, ok := part["functionCall"].(map[string]any); ok {
			return true
		}
		if geminiPartMapIsInlineImage(part) {
			return true
		}
	}
	return false
}

// geminiCompatCompletionTracker retains the content and terminal metadata used
// by the non-streaming compatibility converters. The shared native collector
// can select a later empty parts event and does not retain all tools or images.
type geminiCompatCompletionTracker struct {
	finishReason string
	signal       geminiResponseSignal
	errorBody    []byte
	usage        map[string]any
	parts        []any
	textParts    []string
	event        int
	toolCalls    map[string]geminiCompatToolPart
}

type geminiCompatToolPart struct {
	index int
	event int
}

func (t *geminiCompatCompletionTracker) observe(rawBytes []byte) {
	if sig, ok := detectGeminiResponseSignal(rawBytes); ok && sig.Kind > t.signal.Kind {
		t.signal = sig
		if sig.Kind == geminiSignalError {
			t.errorBody = rawBytes
		}
	}
	var resp map[string]any
	if err := json.Unmarshal(rawBytes, &resp); err != nil {
		return
	}
	t.event++
	if fr := extractGeminiFinishReason(resp); fr != "" {
		t.finishReason = fr
	}
	if usage, ok := resp["usageMetadata"].(map[string]any); ok {
		t.usage = usage
	}
	for _, part := range extractGeminiParts(resp) {
		if text, ok := part["text"].(string); ok && text != "" && !isGeminiThoughtPart(part) {
			// Keep the buffered collector's incremental text contract, while
			// excluding thoughts that the non-streaming converter omits.
			t.textParts = append(t.textParts, text)
		}
		if call, ok := part["functionCall"].(map[string]any); ok && call != nil {
			t.flushText()
			// Equal names or arguments do not identify a call. Only an explicit
			// ID can associate an update in a later event with retained content.
			id, _ := call["id"].(string)
			previous, identified := t.toolCalls[id]
			index := len(t.parts)
			if id != "" && identified && previous.event != t.event {
				index = previous.index
				t.parts[index] = map[string]any{"functionCall": call}
			} else {
				t.parts = append(t.parts, map[string]any{"functionCall": call})
			}
			if id != "" {
				if t.toolCalls == nil {
					t.toolCalls = make(map[string]geminiCompatToolPart)
				}
				t.toolCalls[id] = geminiCompatToolPart{index: index, event: t.event}
			}
		}
		if geminiPartMapIsInlineImage(part) {
			t.flushText()
			t.parts = append(t.parts, map[string]any{"inlineData": part["inlineData"]})
		}
	}
}

func (t *geminiCompatCompletionTracker) flushText() {
	if len(t.textParts) > 0 {
		t.parts = append(t.parts, map[string]any{"text": strings.Join(t.textParts, "")})
		t.textParts = nil
	}
}

func (t *geminiCompatCompletionTracker) consolidate(collected map[string]any) map[string]any {
	t.flushText()
	if collected == nil {
		collected = make(map[string]any)
	}
	candidate := make(map[string]any)
	if candidates, ok := collected["candidates"].([]any); ok && len(candidates) > 0 {
		if original, ok := candidates[0].(map[string]any); ok {
			for key, value := range original {
				candidate[key] = value
			}
		}
	}
	parts := t.parts
	if parts == nil {
		parts = []any{}
	}
	candidate["content"] = map[string]any{"role": "model", "parts": parts}
	if t.finishReason != "" {
		candidate["finishReason"] = t.finishReason
	}
	collected["candidates"] = []any{candidate}
	if t.usage != nil {
		collected["usageMetadata"] = t.usage
	}
	return collected
}

// handleGeminiCompatNoContent runs before client output. Policy blocks remain
// request errors, and MAX_TOKENS remains a valid result when thinking used the budget.
func (s *GeminiMessagesCompatService) handleGeminiCompatNoContent(c *gin.Context, account *Account, finishReason string, sig geminiResponseSignal, errorBody []byte, usage *ClaudeUsage, stream bool, upstreamRequestID string, writeUpstreamError func(int, []byte) error, writePolicyError func(message string) error) (bool, error) {
	switch sig.Kind {
	case geminiSignalError:
		if !s.shouldFailoverGeminiUpstreamError(sig.Status) {
			return true, writeUpstreamError(sig.Status, errorBody)
		}
		return true, s.geminiCompatErrorEnvelopeFailover(c, account, sig, errorBody, upstreamRequestID)
	case geminiSignalPromptBlocked, geminiSignalContentFilter:
		logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini] compat no-content policy stop account=%d reason=%s stream=%t", geminiAccountID(account), sig.Reason, stream)
		return true, writePolicyError(sanitizeUpstreamErrorMessage(strings.TrimSpace(sig.Message)))
	}
	if strings.EqualFold(strings.TrimSpace(finishReason), "MAX_TOKENS") {
		return false, nil
	}
	return true, s.geminiCompatEmptyCompletionFailover(c, account, finishReason, usage, stream, upstreamRequestID)
}

func geminiAccountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func geminiCompatOpsEvent(account *Account, status int, upstreamRequestID, message, detail string) OpsUpstreamErrorEvent {
	event := OpsUpstreamErrorEvent{
		Platform:           PlatformGemini,
		UpstreamStatusCode: status,
		UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
		Kind:               "failover",
		Message:            message,
		Detail:             detail,
	}
	if account != nil {
		event.ProxyID = opsUpstreamProxyID(account)
		event.ProxyName = opsUpstreamProxyName(account)
		event.Platform = account.Platform
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	return event
}

func (s *GeminiMessagesCompatService) geminiCompatErrorEnvelopeFailover(c *gin.Context, account *Account, sig geminiResponseSignal, body []byte, upstreamRequestID string) *UpstreamFailoverError {
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(sig.Message))
	appendOpsUpstreamError(c, geminiCompatOpsEvent(account, sig.Status, upstreamRequestID, message, s.upstreamErrorDetail(body)))
	c.Writer.Header().Del("x-request-id")
	return &UpstreamFailoverError{
		StatusCode:             sig.Status,
		ResponseBody:           body,
		RetryableOnSameAccount: account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(sig.Status),
	}
}

func (s *GeminiMessagesCompatService) geminiCompatEmptyCompletionFailover(c *gin.Context, account *Account, finishReason string, usage *ClaudeUsage, stream bool, upstreamRequestID string) *UpstreamFailoverError {
	reason := strings.ToUpper(strings.TrimSpace(finishReason))
	if reason == "" {
		reason = "NONE"
	}
	var inputTokens, cacheReadTokens, outputTokens int
	if usage != nil {
		inputTokens, cacheReadTokens, outputTokens = usage.InputTokens, usage.CacheReadInputTokens, usage.OutputTokens
	}
	attempts := c.GetInt(geminiEmptyCompletionAttemptsKey) + 1
	c.Set(geminiEmptyCompletionAttemptsKey, attempts)

	message := fmt.Sprintf("Gemini upstream returned no content (finishReason=%s)", reason)
	detail := fmt.Sprintf("finishReason=%s input_tokens=%d cache_read_tokens=%d output_tokens=%d stream=%t attempt=%d",
		reason, inputTokens, cacheReadTokens, outputTokens, stream, attempts)
	appendOpsUpstreamError(c, geminiCompatOpsEvent(account, http.StatusBadGateway, upstreamRequestID, message, detail))
	logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini] empty completion failover=true account=%d %s", geminiAccountID(account), detail)
	c.Writer.Header().Del("x-request-id")

	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    http.StatusBadGateway,
			"message": message,
			"status":  geminiSignalEmptyResponseReason,
		},
	})
	failoverErr := &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           body,
		RequestScopedTransient: true,
		Scope:                  GatewayFailureScopeRequest,
		Reason:                 GatewayFailureReasonGeminiEmptyCompletion,
		ClientStatusCode:       http.StatusBadGateway,
		ClientMessage:          geminiEmptyCompletionClientMessage,
	}
	if attempts >= geminiEmptyCompletionMaxAttempts {
		failoverErr.NextAccountAction = NextAccountStop
	}
	return failoverErr
}
