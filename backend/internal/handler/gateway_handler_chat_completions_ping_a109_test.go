//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// /v1/chat/completions 的槽位等待心跳不能是 Claude 的 data ping 帧（OpenAI SDK 会把它当 chunk 解析）。
func TestA109GatewayChatCompletionsSlotWaitUsesCommentPing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &helperConcurrencyCacheStub{waitAllowed: true} // userSeq 为空：用户槽位始终拿不到
	h := &GatewayHandler{
		gatewayService:    &service.GatewayService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatClaude, 5*time.Millisecond),
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
	setImageChatTestAuth(c)

	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	h.ChatCompletions(c)

	out := recorder.Body.String()
	require.True(t, strings.HasPrefix(out, ":\n\n"), "slot wait should emit SSE comment heartbeats, got %q", out)
	require.NotContains(t, out, `"type": "ping"`)
}

// 仅写过心跳时 failover 耗尽必须补发错误帧，而不是静默 EOF。
func TestA109GatewayChatFailoverExhaustedAfterHeartbeatWritesErrorFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer.WriteHeader(http.StatusOK)
	written, err := c.Writer.WriteString(string(SSEPingFormatComment))
	require.NoError(t, err)
	recordGatewayStreamHeartbeat(c, written)

	(&GatewayHandler{}).handleCCFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}, true)

	out := recorder.Body.String()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, strings.HasPrefix(out, string(SSEPingFormatComment)), "heartbeat prefix must be kept, got %q", out)
	frames := strings.Split(strings.TrimSpace(strings.TrimPrefix(out, string(SSEPingFormatComment))), "\n\n")
	require.Len(t, frames, 1, "expected exactly one terminal frame, got %q", out)
	require.True(t, strings.HasPrefix(frames[0], "data: "), "got %q", frames[0])
	payload := strings.TrimPrefix(frames[0], "data: ")
	require.True(t, gjson.Valid(payload), "got %q", payload)
	require.NotEmpty(t, gjson.Get(payload, "error.message").String())
	require.Equal(t, 1, strings.Count(out, `"error":`))

	_, marked := service.GetOpsStreamError(c)
	require.True(t, marked, "stream error must be recorded for ops")
}

// Forward 已写出实际内容后 failover 耗尽：保持已写内容，不追加错误帧。
func TestA109GatewayChatFailoverExhaustedAfterForwardOutputStaysSilent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer.WriteHeader(http.StatusOK)
	written, err := c.Writer.WriteString(string(SSEPingFormatComment))
	require.NoError(t, err)
	recordGatewayStreamHeartbeat(c, written)
	const a109Chunk = "data: {\"choices\":[]}\n\n"
	_, err = c.Writer.WriteString(a109Chunk)
	require.NoError(t, err)

	(&GatewayHandler{}).handleCCFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}, true)

	require.Equal(t, string(SSEPingFormatComment)+a109Chunk, recorder.Body.String())
}
