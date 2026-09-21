package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const malformedFunctionCallSSE = `data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"test-signature"}]},"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":3}}}` + "\n\n"

func TestAntigravityMalformedFunctionCallStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	handlers := map[string]func(*gin.Context, *http.Response, time.Time, string) (*antigravityStreamResult, error){
		"messages":  svc.handleClaudeStreamingResponse,
		"responses": svc.handleResponsesStreamingFromAntigravity,
		"chat": func(c *gin.Context, r *http.Response, start time.Time, model string) (*antigravityStreamResult, error) {
			return svc.handleChatCompletionsStreamingFromAntigravity(c, r, start, model, true)
		},
	}
	for name, handle := range handlers {
		for _, prefix := range []string{"", `{"thoughtSignature":"prior-signature"}`, `{"text":"partial"}`, `{"thought":true,"text":"thinking"}`, `{"functionCall":{"name":"lookup","args":{}}}`} {
			t.Run(name+prefix, func(t *testing.T) {
				c, rec := newAntigravityCompatContext(http.MethodPost, "/", nil)
				body := malformedFunctionCallSSE
				if prefix != "" {
					body = `data: {"response":{"candidates":[{"content":{"parts":[` + prefix + `]}}]}}` + "\n\n" + body
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				result, err := handle(c, resp, time.Now(), "test-model")
				require.Error(t, err)
				var failover *UpstreamFailoverError
				if prefix == "" || (prefix == `{"thoughtSignature":"prior-signature"}` && name != "messages") {
					require.ErrorAs(t, err, &failover)
					require.True(t, failover.RetryableOnSameAccount)
					require.Equal(t, 1, failover.SameAccountRetryMax)
					require.True(t, failover.RequestScopedTransient)
					require.Equal(t, http.StatusBadGateway, failover.StatusCode)
					require.Contains(t, string(failover.ResponseBody), "MALFORMED_FUNCTION_CALL")
					require.False(t, c.Writer.Written(), "uncommitted response must remain retryable")
					require.Empty(t, rec.Header().Get("Content-Type"), "exhausted retries must still be able to return JSON")
					require.Empty(t, rec.Body.String())
				} else {
					require.NotErrorAs(t, err, &failover, "never replay an already emitted tool call")
					require.NotNil(t, result)
					require.Equal(t, 3, result.usage.OutputTokens)
					require.True(t, IsResponseCommitted(c))
					require.Equal(t, 1, strings.Count(rec.Body.String(), "MALFORMED_FUNCTION_CALL"))
					require.Contains(t, rec.Body.String(), `"error"`)
					if name == "responses" {
						require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
						require.Contains(t, rec.Body.String(), `"status":"failed"`)
						require.Contains(t, rec.Body.String(), `"code":"server_error"`)
					}
				}
				require.NotContains(t, rec.Body.String(), "message_stop")
				require.NotContains(t, rec.Body.String(), "response.completed")
				require.NotContains(t, rec.Body.String(), `"finish_reason":"stop"`)
				require.NotContains(t, rec.Body.String(), "[DONE]")
				require.NotContains(t, rec.Body.String(), "test-signature")
			})
		}
	}
}

func TestAntigravityMalformedFunctionCallNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
	handlers := map[string]func(*gin.Context, *http.Response, time.Time, string) (*antigravityStreamResult, error){
		"messages":  svc.handleClaudeStreamToNonStreaming,
		"responses": svc.handleResponsesNonStreamingFromAntigravity,
		"chat":      svc.handleChatCompletionsNonStreamingFromAntigravity,
	}
	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			c, rec := newAntigravityCompatContext(http.MethodPost, "/", nil)
			// A later usage-only chunk must not erase the earlier failure.
			body := malformedFunctionCallSSE + `data: {"response":{"usageMetadata":{"promptTokenCount":12}}}` + "\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
			_, err := handle(c, resp, time.Now(), "test-model")
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.True(t, failover.RetryableOnSameAccount)
			require.Equal(t, 1, failover.SameAccountRetryMax)
			require.True(t, failover.RequestScopedTransient)
			require.Contains(t, string(failover.ResponseBody), "MALFORMED_FUNCTION_CALL")
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestAntigravityMalformedFunctionCallDoesNotCoolDownAccount(t *testing.T) {
	repo := &capacityShedAccountRepoStub{}
	svc := &GatewayService{accountRepo: repo}
	svc.TempUnscheduleRetryableError(context.Background(), 1, antigravityMalformedFunctionCallError())
	require.Zero(t, repo.tempUnschedCalls)
}
