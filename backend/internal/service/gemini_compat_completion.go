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

func geminiHasDeliverableParts(geminiResp map[string]any) bool {
	for _, part := range extractGeminiParts(geminiResp) {
		if text, ok := part["text"].(string); ok && strings.TrimSuffix(text, "\u0000") != "" {
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

// geminiCompatCompletionTracker observes every event, including terminal events
// that the buffered collector may omit when retaining an earlier content event.
type geminiCompatCompletionTracker struct {
	finishReason string
	deliverable  bool
	signal       geminiResponseSignal
}

func (t *geminiCompatCompletionTracker) observe(rawBytes []byte) {
	if sig, ok := detectGeminiResponseSignal(rawBytes); ok && sig.Kind > t.signal.Kind {
		t.signal = sig
	}
	var resp map[string]any
	if err := json.Unmarshal(rawBytes, &resp); err != nil {
		return
	}
	if fr := extractGeminiFinishReason(resp); fr != "" {
		t.finishReason = fr
	}
	if !t.deliverable && geminiHasDeliverableParts(resp) {
		t.deliverable = true
	}
}

// handleGeminiCompatNoContent runs before client output. Policy blocks remain
// request errors, and MAX_TOKENS remains a valid result when thinking used the budget.
func (s *GeminiMessagesCompatService) handleGeminiCompatNoContent(c *gin.Context, account *Account, finishReason string, sig geminiResponseSignal, usage *ClaudeUsage, stream bool, upstreamRequestID string, writePolicyError func(message string) error) (bool, error) {
	switch sig.Kind {
	case geminiSignalError:
		return true, s.geminiCompatErrorEnvelopeFailover(c, account, sig, upstreamRequestID)
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

func (s *GeminiMessagesCompatService) geminiCompatErrorEnvelopeFailover(c *gin.Context, account *Account, sig geminiResponseSignal, upstreamRequestID string) *UpstreamFailoverError {
	message := sanitizeUpstreamErrorMessage(strings.TrimSpace(sig.Message))
	appendOpsUpstreamError(c, geminiCompatOpsEvent(account, sig.Status, upstreamRequestID, message, sig.Detail))
	c.Writer.Header().Del("x-request-id")
	return &UpstreamFailoverError{
		StatusCode:             sig.Status,
		ResponseBody:           []byte(sig.Detail),
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
