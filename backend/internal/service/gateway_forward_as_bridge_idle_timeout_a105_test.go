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

// A1-05: 桥接读取器必须受 gateway.stream_data_interval_timeout 约束：上游发出
// message_start 后停滞（不发数据也不断连）时，读取应在超时后关闭 resp.Body 并按
// 既有"上游读错误"路径收尾，而不是永久阻塞。

const a105MessageStart = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_a105","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":3}}}` + "\n\n"

type a105Handler struct {
	name     string
	buffered bool
	call     func(*GatewayService, *http.Response, *gin.Context) (*ForwardResult, error)
}

func a105Handlers() []a105Handler {
	start := time.Now()
	return []a105Handler{
		{name: "cc_streaming", call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start, false)
		}},
		{name: "cc_buffered", buffered: true, call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
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

type a105Outcome struct {
	result *ForwardResult
	err    error
}

func a105RunStalled(t *testing.T, h a105Handler, prefix string) (a105Outcome, *httptest.ResponseRecorder, *io.PipeWriter) {
	t.Helper()
	svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		StreamDataIntervalTimeout: 1,
		MaxLineSize:               defaultMaxLineSize,
	}}}
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	if prefix != "" {
		go func() { _, _ = pw.Write([]byte(prefix)) }()
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	done := make(chan a105Outcome, 1)
	go func() {
		result, err := h.call(svc, resp, c)
		done <- a105Outcome{result: result, err: err}
	}()
	select {
	case out := <-done:
		return out, rec, pw
	case <-time.After(5 * time.Second):
		t.Fatal("bridge reader did not time out on stalled upstream")
	}
	return a105Outcome{}, nil, nil
}

func TestA105BridgeReaderIdleTimeoutAfterMessageStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, h := range a105Handlers() {
		t.Run(h.name, func(t *testing.T) {
			out, rec, pw := a105RunStalled(t, h, a105MessageStart)
			// A1-04: 读间隔超时按失败收尾（不 failover），返回错误并携带已累计 usage
			require.Error(t, out.err)
			require.ErrorIs(t, out.err, errAnthropicNativeStreamIdle)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(out.err, &failoverErr))
			require.NotNil(t, out.result)
			require.Equal(t, 3, out.result.Usage.InputTokens)
			if h.buffered {
				require.Equal(t, http.StatusBadGateway, rec.Code)
			} else {
				require.Contains(t, rec.Body.String(), "stream_timeout")
				require.NotContains(t, rec.Body.String(), "[DONE]")
				require.NotContains(t, rec.Body.String(), "response.completed")
			}
			// 超时后必须关闭 resp.Body，释放上游连接
			_, werr := pw.Write([]byte("x"))
			require.ErrorIs(t, werr, io.ErrClosedPipe)
		})
	}
}

func TestA105BridgeBufferedReaderIdleTimeoutBeforeAnyEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, h := range a105Handlers() {
		if !h.buffered {
			continue
		}
		t.Run(h.name, func(t *testing.T) {
			out, rec, _ := a105RunStalled(t, h, "")
			require.Error(t, out.err)
			require.Nil(t, out.result)
			require.Equal(t, http.StatusBadGateway, rec.Code)
		})
	}
}
