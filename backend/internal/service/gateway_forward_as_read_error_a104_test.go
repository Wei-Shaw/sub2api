//go:build unit

package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-04: CC / Responses 桥接上游中途读错误（非客户端取消）不得按成功完成收尾：
// 未输出且无 usage 时交给 handler failover（同账号可重试）；已输出/已计量时写协议内
// 失败终止帧（Responses: response.failed；CC: error chunk 且无 [DONE]），返回错误并
// 携带部分 usage 供计费；缓冲路径不得返回截断的 200。

const a104Prefix = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_a104","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":9,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n"

type a104Handler struct {
	name     string
	buffered bool
	cc       bool
	call     func(*GatewayService, *http.Response, *gin.Context) (*ForwardResult, error)
}

func a104Handlers() []a104Handler {
	start := time.Now()
	return []a104Handler{
		{name: "cc_streaming", cc: true, call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start)
		}},
		{name: "cc_buffered", cc: true, buffered: true, call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleCCBufferedFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start)
		}},
		{name: "responses_streaming", call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start, apicompat.ResponsesClientToolMapping{})
		}},
		{name: "responses_buffered", buffered: true, call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleResponsesBufferedStreamingResponse(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start, apicompat.ResponsesClientToolMapping{})
		}},
	}
}

func a104Run(t *testing.T, h a104Handler, payload string) (*ForwardResult, error, *httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &streamReadCloser{payload: []byte(payload), err: io.ErrUnexpectedEOF},
	}
	result, err := h.call(svc, resp, c)
	return result, err, rec, c
}

func TestA104BridgeReadErrorAfterOutputEndsAsFailureWithPartialUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, h := range a104Handlers() {
		t.Run(h.name, func(t *testing.T) {
			result, err, rec, c := a104Run(t, h, a104Prefix)
			require.Error(t, err, "upstream read error must not be finalized as a successful completion")
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "already metered/written: must not fail over")
			require.NotNil(t, result, "partial usage must be returned for billing")
			require.Equal(t, 9, result.Usage.InputTokens)
			require.True(t, IsResponseCommitted(c), "terminal failure must be marked committed so the handler does not append another frame")

			body := rec.Body.String()
			switch {
			case h.buffered:
				require.Equal(t, http.StatusBadGateway, rec.Code, "buffered path must not return a truncated 200")
				require.NotContains(t, body, `"partial"`)
			case h.cc:
				require.NotContains(t, body, "[DONE]")
				require.Contains(t, body, `"error"`)
				require.Contains(t, body, "stream_read_error")
			default:
				require.NotContains(t, body, "response.completed")
				require.Contains(t, body, "response.failed")
				require.Contains(t, body, "stream_read_error")
			}
		})
	}
}

func TestA104BridgeReadErrorBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, h := range a104Handlers() {
		t.Run(h.name, func(t *testing.T) {
			result, err, rec, _ := a104Run(t, h, "")
			require.Nil(t, result, "failover must keep result=nil to avoid double billing")
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.True(t, failoverErr.RetryableOnSameAccount)
			require.Empty(t, rec.Body.String())
		})
	}
}
