package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBillingErrorDetails_MapsGroupRPMExceededToTooManyRequests(t *testing.T) {
	status, code, msg, retryAfter := billingErrorDetails(service.ErrGroupRPMExceeded)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_exceeded", code)
	require.NotEmpty(t, msg)
	require.Greater(t, retryAfter, 0, "RPM exceeded should return positive Retry-After")
	require.LessOrEqual(t, retryAfter, 60)
}

func TestBillingErrorDetails_MapsUserRPMExceededToTooManyRequests(t *testing.T) {
	status, code, msg, retryAfter := billingErrorDetails(service.ErrUserRPMExceeded)
	require.Equal(t, http.StatusTooManyRequests, status)
	require.Equal(t, "rate_limit_exceeded", code)
	require.NotEmpty(t, msg)
	require.Greater(t, retryAfter, 0, "RPM exceeded should return positive Retry-After")
	require.LessOrEqual(t, retryAfter, 60)
}

func TestBillingErrorDetails_APIKeyRateLimitStillMaps(t *testing.T) {
	// 回归保护：加 RPM 分支后不应影响已有 APIKey rate limit 的映射。
	for _, err := range []error{
		service.ErrAPIKeyRateLimit5hExceeded,
		service.ErrAPIKeyRateLimit1dExceeded,
		service.ErrAPIKeyRateLimit7dExceeded,
	} {
		status, code, _, _ := billingErrorDetails(err)
		require.Equal(t, http.StatusTooManyRequests, status, "status for %v", err)
		require.Equal(t, "rate_limit_exceeded", code)
	}
}

func TestBillingErrorDetails_BillingServiceUnavailableMapsTo503(t *testing.T) {
	status, code, _, retryAfter := billingErrorDetails(service.ErrBillingServiceUnavailable)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "billing_service_error", code)
	require.Equal(t, 0, retryAfter, "non-RPM errors should not set Retry-After")
}

func TestBillingErrorDetails_UnknownErrorFallsBackTo403(t *testing.T) {
	status, code, msg, _ := billingErrorDetails(errors.New("unknown billing failure"))
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "billing_error", code)
	require.NotEmpty(t, msg)
}

func TestExtractQuotaResetSeconds_T19_HappyPath(t *testing.T) {
	err := service.ErrUserPlatformDailyQuotaExhausted.WithMetadata(map[string]string{
		"window_resets_at": time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339),
	})
	got := extractQuotaResetSeconds(err)
	if got < 10 || got > 11 {
		t.Errorf("T19: got %d, want 10 or 11 (math.Ceil boundary)", got)
	}
}

func TestExtractQuotaResetSeconds_T20_NoMetadataFallback(t *testing.T) {
	if got := extractQuotaResetSeconds(errors.New("naked error")); got != 60 {
		t.Errorf("T20: got %d, want 60 fallback", got)
	}
}

func TestExtractQuotaResetSeconds_T21_BadFormatFallback(t *testing.T) {
	err := service.ErrUserPlatformDailyQuotaExhausted.WithMetadata(map[string]string{
		"window_resets_at": "not-a-time",
	})
	if got := extractQuotaResetSeconds(err); got != 60 {
		t.Errorf("T21: got %d, want 60 fallback", got)
	}
}

func TestExtractQuotaResetSeconds_T22_PastResetFallsBackToDefault(t *testing.T) {
	// 当 window_resets_at 已过去时返回 fallback (60s) 而非 1s：
	// 1 秒会导致客户端立即重试仍触发限额的退避循环；
	// 60s 让客户端按常规节奏退避，cache/DB 自愈期间不会反复打抖。
	err := service.ErrUserPlatformDailyQuotaExhausted.WithMetadata(map[string]string{
		"window_resets_at": time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339),
	})
	if got := extractQuotaResetSeconds(err); got != 60 {
		t.Errorf("T22: got %d, want 60 (fallback on past reset)", got)
	}
}

func TestBillingErrorDetails_T10_QuotaExhaustedReturns429WithRetryAfter(t *testing.T) {
	// quota 超限映射 429 + Retry-After（RFC 6585 / 与 RPM 一致），
	// 让 SDK（OpenAI 兼容客户端等）能按 Retry-After 自动退避。
	// 旧实现用 403 导致客户端不退避直接报错。
	// 三个窗口共用同一映射分支，循环覆盖避免漏测某个窗口的 status/code。
	cases := []struct {
		name string
		err  error
	}{
		{"daily", service.ErrUserPlatformDailyQuotaExhausted.WithMetadata(map[string]string{
			"window_resets_at": time.Now().Add(60 * time.Minute).UTC().Format(time.RFC3339),
		})},
		{"weekly", service.ErrUserPlatformWeeklyQuotaExhausted.WithMetadata(map[string]string{
			"window_resets_at": time.Now().Add(60 * time.Minute).UTC().Format(time.RFC3339),
		})},
		{"monthly", service.ErrUserPlatformMonthlyQuotaExhausted.WithMetadata(map[string]string{
			"window_resets_at": time.Now().Add(60 * time.Minute).UTC().Format(time.RFC3339),
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code, _, retryAfter := billingErrorDetails(tc.err)
			if status != http.StatusTooManyRequests {
				t.Errorf("status = %d, want 429", status)
			}
			if code != "rate_limit_exceeded" {
				t.Errorf("code = %q, want rate_limit_exceeded", code)
			}
			if retryAfter < 3599 || retryAfter > 3601 {
				t.Errorf("retryAfter = %d, want ~3600", retryAfter)
			}
		})
	}
}

func TestBillingErrorDetails_MapsSubscriptionLimitsToSwitchableQuotaError(t *testing.T) {
	for _, err := range []error{
		service.ErrDailyLimitExceeded,
		service.ErrWeeklyLimitExceeded,
		service.ErrMonthlyLimitExceeded,
	} {
		status, code, msg, retryAfter := billingErrorDetails(err)
		require.Equal(t, http.StatusTooManyRequests, status, "status for %v", err)
		require.Equal(t, "subscription_limit_exceeded", code)
		require.NotEmpty(t, msg)
		require.Equal(t, 0, retryAfter)
	}
}

func TestBillingErrorDetails_MapsInsufficientBalance(t *testing.T) {
	status, code, msg, retryAfter := billingErrorDetails(service.ErrInsufficientBalance)
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "insufficient_balance", code)
	require.NotEmpty(t, msg)
	require.Equal(t, 0, retryAfter)
}

func TestRespondBillingAsAssistantMessageIncludesStructuredAutoSwitchMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrDailyLimitExceeded, billingProtocolAnthropic, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "subscription_limit_exceeded", rec.Header().Get("X-Sub2API-Billing-Code"))
	require.Equal(t, "true", rec.Header().Get("X-Sub2API-Auto-Switchable"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok, "assistant message should expose billing metadata for clients that inspect 200 responses")
	require.Equal(t, "subscription_limit_exceeded", metadata["billing_code"])
	require.Equal(t, true, metadata["auto_switchable"])
}

func TestRespondBillingAsAssistantMessageMarksInsufficientBalanceAsNotSwitchable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrInsufficientBalance, billingProtocolOpenAIChat, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "insufficient_balance", rec.Header().Get("X-Sub2API-Billing-Code"))
	require.Equal(t, "false", rec.Header().Get("X-Sub2API-Auto-Switchable"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
	require.Equal(t, false, metadata["auto_switchable"])
}

func TestRespondBillingAsAssistantMessageIncludesCodeInAssistantText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrAPIKeyRateLimit1dExceeded, billingProtocolOpenAIChat, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "rate_limit_exceeded", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	choices, ok := body["choices"].([]any)
	require.True(t, ok)
	require.Len(t, choices, 1)
	choice, ok := choices[0].(map[string]any)
	require.True(t, ok)
	message, ok := choice["message"].(map[string]any)
	require.True(t, ok)
	content, ok := message["content"].(string)
	require.True(t, ok)
	require.Contains(t, content, "Current API key has reached its daily usage limit")
	require.Contains(t, content, "Code: rate_limit_exceeded")
}

func TestRespondBillingAsAssistantMessageUsesResponsesFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrDailyLimitExceeded, billingProtocolOpenAIResponses, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "subscription_limit_exceeded", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "response", body["object"])
	require.Equal(t, "completed", body["status"])
	output, ok := body["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 1)
	message, ok := output[0].(map[string]any)
	require.True(t, ok)
	content, ok := message["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "output_text", part["type"])
	require.Contains(t, part["text"], "Code: subscription_limit_exceeded")
}

func TestRespondBillingAsAssistantMessageUsesStreamingChatProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrUserRPMExceeded, billingProtocolOpenAIChat, true))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "rate_limit_exceeded", rec.Header().Get("X-Sub2API-Billing-Code"))
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
	body := rec.Body.String()
	require.Contains(t, body, "chat.completion.chunk")
	require.Contains(t, body, "Code: rate_limit_exceeded")
	require.Contains(t, body, "data: [DONE]")
}

func TestRespondBillingAsAssistantMessageUsesStreamingResponsesProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrDailyLimitExceeded, billingProtocolOpenAIResponses, true))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, "Code: subscription_limit_exceeded")
	require.Contains(t, body, "event: response.completed")
}

func TestRespondBillingAsAssistantMessageUsesGeminiGenerateContentFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrInsufficientBalance, billingProtocolGemini, false))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "insufficient_balance", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	candidates, ok := body["candidates"].([]any)
	require.True(t, ok)
	require.Len(t, candidates, 1)
	candidate, ok := candidates[0].(map[string]any)
	require.True(t, ok)
	content, ok := candidate["content"].(map[string]any)
	require.True(t, ok)
	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	part, ok := parts[0].(map[string]any)
	require.True(t, ok)
	text, ok := part["text"].(string)
	require.True(t, ok)
	require.Contains(t, text, "Code: insufficient_balance")
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
}

func TestRespondBillingAsAssistantMessageUsesGeminiStreamingFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrDailyLimitExceeded, billingProtocolGemini, true))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	require.Contains(t, body, "data: {")
	require.Contains(t, body, `"candidates"`)
	require.Contains(t, body, "Code: subscription_limit_exceeded")
	require.Contains(t, body, `"finishReason":"STOP"`)
}

func TestRespondBillingAsAssistantMessageUsesGeminiErrorFormatForCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrInsufficientBalance, billingProtocolGeminiError, false))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "insufficient_balance", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(http.StatusForbidden), errorBody["code"])
	require.Equal(t, "PERMISSION_DENIED", errorBody["status"])
	message, ok := errorBody["message"].(string)
	require.True(t, ok)
	require.Contains(t, message, "Code: insufficient_balance")
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
}

func TestRespondBillingAsAssistantMessageUsesOpenAIImagesErrorFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrInsufficientBalance, billingProtocolOpenAIImages, false))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "insufficient_balance", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", errorBody["code"])
	message, ok := errorBody["message"].(string)
	require.True(t, ok)
	require.Contains(t, message, "Code: insufficient_balance")
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
}

func TestRespondBillingAsAssistantMessageUsesOpenAIImagesStreamingErrorFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrUserRPMExceeded, billingProtocolOpenAIImages, true))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "rate_limit_exceeded", rec.Header().Get("X-Sub2API-Billing-Code"))
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
	body := rec.Body.String()
	require.Contains(t, body, "event: error")
	require.Contains(t, body, `"type":"error"`)
	require.Contains(t, body, "Code: rate_limit_exceeded")
}

func TestRespondBillingAsAssistantMessageUsesAnthropicErrorFormatForCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	require.True(t, respondBillingAsAssistantMessage(ctx, service.ErrInsufficientBalance, billingProtocolAnthropicError, false))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "insufficient_balance", rec.Header().Get("X-Sub2API-Billing-Code"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "error", body["type"])
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", errorBody["type"])
	message, ok := errorBody["message"].(string)
	require.True(t, ok)
	require.Contains(t, message, "Code: insufficient_balance")
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
}

func TestOpenAIWSBillingErrorPayloadIncludesDebuggableBillingDetails(t *testing.T) {
	payload := openAIWSBillingErrorPayload(service.ErrInsufficientBalance)

	var body map[string]any
	require.NoError(t, json.Unmarshal(payload, &body))
	require.Equal(t, "error", body["type"])
	errorBody, ok := body["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", errorBody["code"])
	message, ok := errorBody["message"].(string)
	require.True(t, ok)
	require.Contains(t, message, "Code: insufficient_balance")
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "insufficient_balance", metadata["billing_code"])
}
