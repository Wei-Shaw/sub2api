//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-01: 上游 200 后、已向客户端输出内容之后再下发 `event: error`，
// 已计量的 usage 不得随错误丢弃：Forward 必须返回部分结果 + 非 failover 错误
// （已写出内容无法 failover，handler 据此照常入账）。未输出前仍保持
// result=nil + UpstreamFailoverError，避免 failover 重试成功后双重计费。
func TestA101GatewayForward_SSEErrorEventAfterOutputKeepsPartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	const errorJSON = `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
	upstreamSSE := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_a101","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest","content":[],"usage":{"input_tokens":11,"cache_read_input_tokens":7}}}`,
		"",
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		"",
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":5}}`,
		"",
		`event: error`,
		`data: ` + errorJSON,
		"",
		"",
	}, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-a101"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamSSE)),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)

	result, err := svc.Forward(context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
	require.Error(t, err)
	require.NotNil(t, result, "已输出内容后的流内 error 事件必须带回已计量的部分 usage")
	require.True(t, result.Stream)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, "rid-a101", result.RequestID)
	require.NotNil(t, result.FirstTokenMs)

	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "已输出内容后不可 failover，错误不得是 UpstreamFailoverError（否则 handler 不入账）")
	var afterOutputErr *StreamErrorEventAfterOutputError
	require.ErrorAs(t, err, &afterOutputErr)
	require.Equal(t, 529, afterOutputErr.StatusCode, "语义状态码沿用 A1-10 的 error.type 推导")
	require.JSONEq(t, errorJSON, string(afterOutputErr.ResponseBody))
	require.Contains(t, rec.Body.String(), "message_start")
}

func TestA101GatewayForward_SSEErrorEventBeforeOutputStaysFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	require.NoError(t, err)

	const errorJSON = `{"type":"error","error":{"type":"api_error","message":"boom"}}`
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: error\ndata: " + errorJSON + "\n\n")),
	}}
	svc := newForwardPartialUsageServiceForTest(upstream)

	result, err := svc.Forward(context.Background(), c, newAnthropicOAuthAccountForPartialUsageTest(), parsed)
	require.Error(t, err)
	require.Nil(t, result, "未输出前必须保持 result=nil，failover 重试成功后才不会双重计费")
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusInternalServerError, failoverErr.StatusCode)
	require.Empty(t, rec.Body.String())
}
