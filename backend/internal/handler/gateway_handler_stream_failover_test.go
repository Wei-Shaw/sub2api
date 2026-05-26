package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// partialMessageStartSSE 模拟 handleStreamingResponse 已写入的首批 SSE 事件。
const partialMessageStartSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_01\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-5\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n" +
	"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"

// TestStreamWrittenGuard_MessagesPath_AbortFailoverOnSSEContentWritten 验证：
// 当 Forward 在返回 UpstreamFailoverError 前已向客户端写入 SSE 内容时，
// 故障转移保护逻辑必须终止循环并发送 SSE 错误事件，而不是进行下一次 Forward。
// 具体验证：
//  1. c.Writer.Size() 检测条件正确触发（字节数已增加）
//  2. handleFailoverExhausted 以 streamStarted=true 调用后，响应体以 SSE 错误事件结尾
//  3. 响应体中只出现一个 message_start，不存在第二个（防止流拼接腐化）
func TestStreamWrittenGuard_MessagesPath_AbortFailoverOnSSEContentWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 步骤 1：记录 Forward 前的 writer size（模拟 writerSizeBeforeForward := c.Writer.Size()）
	sizeBeforeForward := c.Writer.Size()
	require.Equal(t, -1, sizeBeforeForward, "gin writer 初始 Size 应为 -1（未写入任何字节）")

	// 步骤 2：模拟 Forward 已向客户端写入部分 SSE 内容（message_start + content_block_start）
	_, err := c.Writer.Write([]byte(partialMessageStartSSE))
	require.NoError(t, err)

	// 步骤 3：验证守卫条件成立（c.Writer.Size() != sizeBeforeForward）
	require.NotEqual(t, sizeBeforeForward, c.Writer.Size(),
		"写入 SSE 内容后 writer size 必须增加，守卫条件应为 true")

	// 步骤 4：模拟 UpstreamFailoverError（上游在流中途返回 403）
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:   http.StatusForbidden,
		ResponseBody: []byte(`{"error":{"type":"permission_error","message":"forbidden"}}`),
	}

	// 步骤 5：守卫触发 → 调用 handleFailoverExhausted，streamStarted=true
	h := &GatewayHandler{}
	h.handleFailoverExhausted(c, failoverErr, service.PlatformAnthropic, true)

	body := w.Body.String()

	// 断言 A：响应体中包含最初写入的 message_start SSE 事件行
	require.Contains(t, body, "event: message_start", "响应体应包含已写入的 message_start SSE 事件")

	// 断言 B：响应体以 SSE 错误事件结尾（data: {"type":"error",...}\n\n）
	require.True(t, strings.HasSuffix(strings.TrimRight(body, "\n"), "}"),
		"响应体应以 JSON 对象结尾（SSE error event 的 data 字段）")
	require.Contains(t, body, `"type":"error"`, "响应体末尾必须包含 SSE 错误事件")

	// 断言 C：SSE event 行 "event: message_start" 只出现一次（防止双 message_start 拼接腐化）
	firstIdx := strings.Index(body, "event: message_start")
	lastIdx := strings.LastIndex(body, "event: message_start")
	assert.Equal(t, firstIdx, lastIdx,
		"响应体中 'event: message_start' 必须只出现一次，不得因 failover 拼接导致两次")
}

// TestStreamWrittenGuard_GeminiPath_AbortFailoverOnSSEContentWritten 与上述测试相同，
// 验证 Gemini 路径使用 service.PlatformGemini（而非 account.Platform）时行为一致。
func TestStreamWrittenGuard_GeminiPath_AbortFailoverOnSSEContentWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:streamGenerateContent", nil)

	sizeBeforeForward := c.Writer.Size()

	_, err := c.Writer.Write([]byte(partialMessageStartSSE))
	require.NoError(t, err)

	require.NotEqual(t, sizeBeforeForward, c.Writer.Size())

	failoverErr := &service.UpstreamFailoverError{
		StatusCode: http.StatusForbidden,
	}

	h := &GatewayHandler{}
	h.handleFailoverExhausted(c, failoverErr, service.PlatformGemini, true)

	body := w.Body.String()

	require.Contains(t, body, "event: message_start")
	require.Contains(t, body, `"type":"error"`)

	firstIdx := strings.Index(body, "event: message_start")
	lastIdx := strings.LastIndex(body, "event: message_start")
	assert.Equal(t, firstIdx, lastIdx, "Gemini 路径不得出现双 message_start")
}

// TestStreamWrittenGuard_NoByteWritten_GuardNotTriggered 验证反向场景：
// 当 Forward 返回 UpstreamFailoverError 时若未向客户端写入任何 SSE 内容，
// 守卫条件（c.Writer.Size() != sizeBeforeForward）为 false，不应中止 failover。
func TestStreamWrittenGuard_NoByteWritten_GuardNotTriggered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 模拟 writerSizeBeforeForward：初始为 -1
	sizeBeforeForward := c.Writer.Size()

	// Forward 未写入任何字节直接返回错误（例如 401 发生在连接建立前）
	// c.Writer.Size() 仍为 -1

	// 守卫条件：sizeBeforeForward == c.Writer.Size() → 不触发
	guardTriggered := c.Writer.Size() != sizeBeforeForward
	require.False(t, guardTriggered,
		"未写入任何字节时，守卫条件必须为 false，应允许正常 failover 继续")
}
