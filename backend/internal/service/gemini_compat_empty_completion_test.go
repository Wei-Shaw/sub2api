//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func geminiEmptyCompletionTestResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func requireGeminiEmptyCompletionFailover(t *testing.T, err error, rec *httptest.ResponseRecorder, c *gin.Context) {
	t.Helper()
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "want UpstreamFailoverError, got %v", err)
	require.True(t, failoverErr.Reason == GatewayFailureReason("gemini_empty_completion"))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount, "empty completions must not retry on the same account")
	require.Equal(t, http.StatusBadGateway, failoverErr.ClientStatusCode)
	require.NotEmpty(t, failoverErr.ClientMessage)
	// Nothing may reach the client: the handler fails over only when the writer is untouched.
	require.False(t, c.Writer.Written())
	require.Equal(t, 0, rec.Body.Len())
	require.Empty(t, c.Writer.Header().Get("Content-Type"))
}

func TestGeminiMessagesStreaming_EmptyCompletionFailsOverWithoutWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":20,"cachedContentTokenCount":10}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, result)
	requireGeminiEmptyCompletionFailover(t, err, rec, c)
	require.Contains(t, string(err.(*UpstreamFailoverError).ResponseBody), "finishReason=STOP")
}

func TestGeminiMessagesStreaming_MalformedFunctionCallWithEmptyTextFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":""}]}}]}` + "\n\n" +
		`data: {"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":2049}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	_, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	requireGeminiEmptyCompletionFailover(t, err, rec, c)
	require.Contains(t, string(err.(*UpstreamFailoverError).ResponseBody), "MALFORMED_FUNCTION_CALL")
}

func TestGeminiMessagesStreaming_EmptyMaxTokensKeepsLegacyStopReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model"},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":128}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 128, result.usage.OutputTokens)
	out := rec.Body.String()
	require.Equal(t, "text/event-stream", c.Writer.Header().Get("Content-Type"))
	require.True(t, strings.Index(out, "event: message_start") == 0, out)
	require.Contains(t, out, `"stop_reason":"max_tokens"`)
	require.Contains(t, out, "event: message_stop")
}

// Inline images are not rendered by the messages bridge; an image-only reply keeps the legacy
// 200 + message_stop behavior instead of being failed over.
func TestGeminiMessagesStreaming_InlineImageOnlyKeepsLegacyReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1290}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-image", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "event: message_stop")
}

func TestGeminiMessagesStreaming_ContentStillStartsWithMessageStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"pong"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result.firstTokenMs)
	out := rec.Body.String()
	require.True(t, strings.Index(out, "event: message_start") == 0, out)
	require.Less(t, strings.Index(out, "event: message_start"), strings.Index(out, "event: content_block_start"))
	require.Contains(t, out, `"text":"pong"`)
	require.Contains(t, out, `"stop_reason":"end_turn"`)
}

func TestGeminiMessagesNonStreaming_EmptyCompletionFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"role":"model"},"finishReason":"OTHER"}],"usageMetadata":{"promptTokenCount":20}}`)),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	usage, err := svc.handleNonStreamingResponse(c, resp, "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, usage)
	requireGeminiEmptyCompletionFailover(t, err, rec, c)
}

func TestGeminiChatCompletionsStreaming_EmptyCompletionFailsOverWithoutWriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"cachedContentTokenCount":10}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", false, true, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, result)
	requireGeminiEmptyCompletionFailover(t, err, rec, c)
}

func TestGeminiChatCompletionsStreaming_EmptyMaxTokensKeepsLegacyFinish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model"},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":128}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", false, true, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	out := rec.Body.String()
	require.Contains(t, out, `"finish_reason":"length"`)
	require.True(t, strings.HasSuffix(out, "data: [DONE]\n\n"), out)
}

func TestGeminiChatCompletionsStreaming_TextStillStreams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"parts":[{"text":"pong"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", false, true, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	out := rec.Body.String()
	require.Equal(t, "text/event-stream", c.Writer.Header().Get("Content-Type"))
	require.Contains(t, out, `"content":"pong"`)
	require.Contains(t, out, `"finish_reason":"stop"`)
}

func TestGeminiChatCompletionsNonStreaming_EmptyCompletionFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"role":"model","parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20}}`)),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	usage, err := svc.handleChatCompletionsNonStreamingResponseFromGemini(c, resp, "gemini-2.5-pro", false, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, usage)
	requireGeminiEmptyCompletionFailover(t, err, rec, c)
}

func TestGeminiMessagesStreaming_ContentFilterWritesPolicyError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"candidates":[{"content":{"role":"model"},"finishReason":"SAFETY","index":0}],"usageMetadata":{"promptTokenCount":10}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "content-policy stops must not fail over")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Contains(t, rec.Body.String(), "invalid_request_error")
	require.Contains(t, rec.Body.String(), "finishReason=SAFETY")
}

func TestGeminiChatCompletionsStreaming_PromptBlockedWritesPolicyError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"},"usageMetadata":{"promptTokenCount":10}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", false, true, &Account{ID: 1, Platform: PlatformGemini}, "")
	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "blockReason=PROHIBITED_CONTENT")
}

func TestGeminiMessagesStreaming_ErrorEnvelopeFailsOverWithItsStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := `data: {"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}` + "\n\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &GeminiMessagesCompatService{}
	_, err := svc.handleStreamingResponse(c, geminiEmptyCompletionTestResp(upstreamBody), time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.False(t, failoverErr.Reason == GatewayFailureReason("gemini_empty_completion"))
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, c.Writer.Written())
	require.Contains(t, string(failoverErr.ResponseBody), "RESOURCE_EXHAUSTED")
}

func TestGeminiCompatEmptyCompletion_StopsAfterSecondAttempt(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat completions"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			svc := &GeminiMessagesCompatService{}
			for attempt := 1; attempt <= 2; attempt++ {
				c.Header("x-request-id", "previous-attempt")
				resp := geminiEmptyCompletionTestResp(`data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10}}` + "\n\n")
				var err error
				if endpoint == "messages" {
					_, err = svc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
				} else {
					_, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-2.5-pro", false, false, &Account{ID: 1, Platform: PlatformGemini}, "")
				}
				requireGeminiEmptyCompletionFailover(t, err, rec, c)
				failover := err.(*UpstreamFailoverError)
				require.Equal(t, attempt == 1, failover.ShouldRetryNextAccount())
				require.True(t, failover.RequestScopedTransient, "an empty completion must not penalize account health")
				require.Empty(t, c.Writer.Header().Get("x-request-id"))
			}
		})
	}
}

func TestGeminiCompatStreaming_PartialContentDoesNotFailOver(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, part := range []string{`{"text":"partial"}`, `{"text":"reasoning","thought":true}`, `{"functionCall":{"name":"read","args":{}}}`} {
			t.Run(endpoint+part, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				svc := &GeminiMessagesCompatService{}
				resp := geminiEmptyCompletionTestResp(`data: {"candidates":[{"content":{"parts":[` + part + `]}}]}` + "\n\n" + `data: {"error":{"code":503,"message":"unavailable","status":"UNAVAILABLE"}}` + "\n\n")
				var result *geminiStreamResult
				var err error
				if endpoint == "messages" {
					result, err = svc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-pro", &Account{ID: 1, Platform: PlatformGemini}, "")
				} else {
					result, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-2.5-pro", false, false, &Account{ID: 1, Platform: PlatformGemini}, "")
				}
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover), "delivered content cannot be replayed on another account")
				require.NotNil(t, result)
				require.NotEmpty(t, rec.Body.String())
			})
		}
	}
}

func TestGeminiCompatBuffered_EmptyCompletionUsesAllEvents(t *testing.T) {
	for _, endpoint := range []string{"messages", "chat completions"} {
		for _, reason := range []string{"STOP", "MAX_TOKENS", "SAFETY"} {
			t.Run(endpoint+reason, func(t *testing.T) {
				svc, upstream := newGeminiVertexAPIKeyTestService()
				svc.tokenProvider = &GeminiTokenProvider{}
				upstream.response = geminiEmptyCompletionTestResp(`data: {"response":{"candidates":[{"content":{"parts":[{"text":""}]}}]}}` + "\n\n" +
					`data: {"response":{"candidates":[{"finishReason":"` + reason + `"}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":2}}}` + "\n\n")
				account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token", "project_id": "test-project"}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := []byte(`{"model":"gemini-2.5-pro","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
				var result *ForwardResult
				var err error
				if endpoint == "messages" {
					result, err = svc.Forward(context.Background(), c, account, body)
				} else {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body)
				}
				switch reason {
				case "STOP":
					require.Nil(t, result)
					requireGeminiEmptyCompletionFailover(t, err, rec, c)
				case "MAX_TOKENS":
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 2, result.Usage.OutputTokens)
					require.Equal(t, http.StatusOK, rec.Code)
				case "SAFETY":
					require.Error(t, err)
					require.Nil(t, result)
					var failover *UpstreamFailoverError
					require.False(t, errors.As(err, &failover))
					require.Equal(t, http.StatusBadRequest, rec.Code)
				}
			})
		}
	}
}
