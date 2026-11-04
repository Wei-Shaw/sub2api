//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// compatBufferedHandlers 是两条兼容入口的非流式（缓冲）收尾；上游被强制流式。
func compatBufferedHandlers() map[string]compatStreamHandler {
	return map[string]compatStreamHandler{
		"chat_completions": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleCCBufferedFromAnthropic(resp, c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now())
		},
		"responses": func(svc *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return svc.handleResponsesBufferedStreamingResponse(resp, c, "claude-sonnet-4-6", "claude-sonnet-4-6", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
		},
	}
}

// compatBufferedErrorKind 读取端点错误体中的错误类别：Chat Completions 为 error.type，
// Responses 为 error.code。
func compatBufferedErrorKind(t *testing.T, name, body string) string {
	t.Helper()
	require.True(t, gjson.Valid(body), "error body must be a single JSON object: %s", body)
	if name == "responses" {
		return gjson.Get(body, "error.code").String()
	}
	return gjson.Get(body, "error.type").String()
}

func TestAnthropicCompatBuffered_TerminalEventCompletes(t *testing.T) {
	for name, handle := range compatBufferedHandlers() {
		for streamName, stream := range map[string]string{
			"message_stop": anthropicStreamWithMidStreamText(),
			"stop_reason":  anthropicStreamWithoutMessageStop(),
			"done_line":    anthropicStreamTruncatedBeforeMessageDelta() + "data: [DONE]\n\n",
		} {
			t.Run(name+"/"+streamName, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)

				result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, rec.Body.String(), "firstsecond")
				require.Equal(t, 20, result.Usage.InputTokens)
			})
		}
	}
}

// 上游在 message_start 之后、结束事件之前正常关闭：半截内容不得按 200 成功下发，
// 已观测到的 usage 随错误返回由 handler 计费。
func TestAnthropicCompatBuffered_EOFWithoutTerminalEventFails(t *testing.T) {
	messageStartOnly := anthropicStreamWithMidStreamText()[:strings.Index(anthropicStreamWithMidStreamText(), "event: content_block_start")]
	for name, handle := range compatBufferedHandlers() {
		for streamName, stream := range map[string]string{
			"after_text":          anthropicStreamTruncatedBeforeMessageDelta(),
			"after_message_start": messageStartOnly,
		} {
			t.Run(name+"/"+streamName, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)

				result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}, c)

				require.ErrorContains(t, err, "missing terminal event")
				var failoverErr *UpstreamFailoverError
				require.NotErrorAs(t, err, &failoverErr, "metered usage is billed, not retried")
				require.NotNil(t, result)
				require.False(t, result.Stream)
				require.Equal(t, 20, result.Usage.InputTokens)
				require.Equal(t, 11, result.Usage.CacheReadInputTokens)
				require.Equal(t, http.StatusBadGateway, rec.Code)
				require.True(t, IsResponseCommitted(c))
				body := rec.Body.String()
				require.NotContains(t, body, "first", "the half-assembled answer must not reach the client")
				require.Equal(t, "stream_incomplete", compatBufferedErrorKind(t, name, body))
			})
		}
	}
}

func TestAnthropicCompatBuffered_ReadErrorAfterUsageFails(t *testing.T) {
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := io.NopCloser(&errAfterReader{data: strings.NewReader(anthropicStreamTruncatedBeforeMessageDelta()), err: io.ErrUnexpectedEOF})

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: body}, c)

			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr, "metered usage is billed, not retried")
			require.NotNil(t, result)
			require.Equal(t, 20, result.Usage.InputTokens)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.NotContains(t, rec.Body.String(), "first")
			require.Equal(t, "stream_read_error", compatBufferedErrorKind(t, name, rec.Body.String()))
		})
	}
}

// 尚未观测到任何 usage 时读错误：与 /v1/messages 尚未输出时一致，不写任何内容并可同号重试。
func TestAnthropicCompatBuffered_ReadErrorBeforeUsageFailsOver(t *testing.T) {
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := io.NopCloser(&errAfterReader{data: strings.NewReader(""), err: io.ErrUnexpectedEOF})

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: body}, c)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Equal(t, "upstream_disconnected", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
			require.Zero(t, rec.Body.Len())
			require.False(t, IsResponseCommitted(c))
		})
	}
}

func TestAnthropicCompatBuffered_IdleUpstreamFails(t *testing.T) {
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			resp, pw := stallingAnthropicUpstream(t, anthropicStreamTruncatedBeforeMessageDelta())

			got := runCompatStreamWithDeadline(t, handle, newCompatStreamIdleTestService(1), resp, c)

			require.ErrorIs(t, got.err, errAnthropicNativeStreamIdle)
			require.NotNil(t, got.result)
			require.Equal(t, 20, got.result.Usage.InputTokens)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.NotContains(t, rec.Body.String(), "first")
			require.Equal(t, "stream_timeout", compatBufferedErrorKind(t, name, rec.Body.String()))
			_, werr := io.WriteString(pw, "event: ping\n")
			require.Error(t, werr, "upstream body must be closed after idle timeout")
		})
	}
}

func TestAnthropicCompatBuffered_UpstreamErrorEventWritesNothing(t *testing.T) {
	after := strings.Join([]string{
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"LEAKED"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n"
	for name, handle := range compatBufferedHandlers() {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			result, err := handle(&GatewayService{}, &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(anthropicStreamWithInBandError("Overloaded", after)))}, c)

			var sseErr *sseStreamErrorEventError
			require.ErrorAs(t, err, &sseErr)
			require.Nil(t, result, "an upstream error event is not billed")
			require.Zero(t, rec.Body.Len(), "the handler fails over to another account")
			require.False(t, IsResponseCommitted(c))
		})
	}
}

// 非流式请求在流内 error 事件时从未向客户端输出：overloaded_error 一律按 529 换号。
func TestForwardAsCompatBuffered_UpstreamErrorEventFailsOver(t *testing.T) {
	beforeOutput := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	for _, endpoint := range []string{"chat_completions", "responses"} {
		for _, tc := range []struct {
			name   string
			stream string
		}{
			{name: "before_message_start", stream: beforeOutput},
			{name: "after_text", stream: anthropicStreamWithInBandError("Overloaded", "")},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":false}`
				path := "/v1/chat/completions"
				if endpoint == "responses" {
					body = `{"model":"claude-sonnet-4-6","input":"hi","stream":false}`
					path = "/v1/responses"
				}
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.stream))}
				svc := &GatewayService{
					cfg:              rawChatCompletionsTestConfig(),
					httpUpstream:     &anthropicHTTPUpstreamRecorder{resp: resp},
					rateLimitService: &RateLimitService{accountRepo: &errorPolicyRepoStub{}, cfg: &config.Config{}},
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}}

				var result *ForwardResult
				var err error
				if endpoint == "responses" {
					result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
				} else {
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				}

				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, 529, failoverErr.StatusCode)
				require.Nil(t, result)
				require.Zero(t, rec.Body.Len())
			})
		}
	}
}
