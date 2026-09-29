//go:build unit

package service

import (
	"errors"
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
)

// A1-02: CC / Responses 桥接在客户端写失败后必须继续读取上游直到流结束，
// 合并 message_delta 中的 output_tokens（Anthropic 只在最终 message_delta 报告），
// 否则只计 ~1 个 output token；断开后不再写客户端，结果标记 ClientDisconnect。

const a102Fixture = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_a102","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","usage":{"input_tokens":7,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

// a102DisconnectingWriter 前 allowed 次写成功，之后模拟客户端断开（写失败），并统计断开后的写尝试。
type a102DisconnectingWriter struct {
	gin.ResponseWriter
	allowed        int
	writes         int
	writesAfterErr int
}

func (w *a102DisconnectingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.allowed {
		if w.writes > w.allowed+1 {
			w.writesAfterErr++
		}
		return 0, errors.New("client disconnected")
	}
	return w.ResponseWriter.Write(p)
}

func (w *a102DisconnectingWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestA102BridgeStreamingDrainsUpstreamAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Now()
	cases := []struct {
		name string
		call func(*GatewayService, *http.Response, *gin.Context) (*ForwardResult, error)
	}{
		{name: "cc_streaming", call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleCCStreamingFromAnthropic(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start, true)
		}},
		{name: "responses_streaming", call: func(s *GatewayService, resp *http.Response, c *gin.Context) (*ForwardResult, error) {
			return s.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4-5", "claude-sonnet-4-5", nil, start, apicompat.ResponsesClientToolMapping{})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			w := &a102DisconnectingWriter{ResponseWriter: c.Writer, allowed: 1}
			c.Writer = w
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(a102Fixture)),
			}

			result, err := tc.call(svc, resp, c)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 42, result.Usage.OutputTokens, "must keep draining upstream after client disconnect to bill final output_tokens")
			require.True(t, result.ClientDisconnect)
			require.Zero(t, w.writesAfterErr, "must stop writing to the client after the first write failure")
			require.NotContains(t, rec.Body.String(), "[DONE]")
		})
	}
}
