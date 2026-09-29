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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A1-13: handleStreamingResponse 自行写出终止 `event: error` 帧后必须标记响应已提交，
// 否则 handler 的 ensureForwardErrorResponse 会再追加一条通用 upstream_error 帧。
func TestA113HandleStreamingResponse_StreamReadErrorAfterOutput_MarksCommitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			payload: []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"),
			err:     io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream read error")
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "event: error\n")
	require.Contains(t, rec.Body.String(), `"stream_read_error"`)
	require.True(t, IsResponseCommitted(c), "写出终止 error 事件后必须 MarkResponseCommitted")
}

func TestA113HandleStreamingResponse_StreamTimeout_MarksCommitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()
	svc.cfg.Gateway.StreamDataIntervalTimeout = 1

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
		_ = pr.Close()
	})
	go func() {
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"))
		// 之后不再写入，模拟上游停滞
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       pr,
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream data interval timeout")
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "stream_timeout")
	require.Equal(t, 1, strings.Count(body, "event: error\n"))
	require.True(t, IsResponseCommitted(c), "写出终止 error 事件后必须 MarkResponseCommitted")
}
