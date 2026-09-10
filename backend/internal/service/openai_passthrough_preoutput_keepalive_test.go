package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 这一组守住的是「首个可见输出之前下游不再静默」这条性质。
//
// 透传路径的 pendingLines 会把 response.created / response.in_progress 全部扣住，
// 于是首个可见输出之前下游一个字节都收不到，连 HTTP 响应头都不会提交。推理模型
// 思考数百秒时，中间层代理会按空闲超时把连接判死。Forward 路径早就用心跳解决了
// 这个问题（见 openai_gateway_response_handling.go 中 lastDownstreamWriteAt 的注释），
// 透传路径漏了。

func newPassthroughKeepaliveTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	// 刻意【不】调用 MarkOpenAICompactClientStream：普通 /v1/responses 透传不带
	// compact 标记，这正是它此前拿不到心跳的原因。
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

// startOpenAISSEKeepalive 必须在【没有】compact 标记时也能启动 ——
// 否则普通透传请求依旧静默。
func TestStartOpenAISSEKeepalive_WorksWithoutCompactMarker(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)

	// 对照:带 compact 标记检查的入口在这里应当直接 no-op。
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len(), "无 compact 标记时 StartOpenAICompactSSEKeepalive 应当 no-op")

	// 内部入口不检查标记,应当真的开始打拍。
	c, rec = newPassthroughKeepaliveTestContext(t)
	stop = startOpenAISSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c), "心跳应当提交响应头")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
}

// 🔴 最要紧的一条:心跳字节【不得】把请求判成「已向客户端写出语义响应」，
// 否则上游 429/5xx 时不再换号 —— 这正是 #3887 加固的那条不变量，
// 透传路径的 pre-output failover 完全依赖它。
func TestPassthroughKeepaliveDoesNotBlockPreOutputFailover(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()
	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.NotZero(t, rec.Body.Len(), "前提:心跳确实写出了字节")

	// 只有心跳字节时,仍应判定为「尚未向客户端输出」。
	require.False(t, openAIStreamClientOutputStarted(c, false),
		"心跳字节不构成语义输出,pre-output failover 必须仍然可用")

	// 写出一条真实事件之后,判定才翻转。
	_, err := c.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"))
	require.NoError(t, err)
	require.True(t, openAIStreamClientOutputStarted(c, false),
		"真实语义输出之后应当判定为已输出")
}

// 停拍之后不得再有心跳字节写出 —— 主循环接管 ResponseWriter 的前提。
func TestPassthroughKeepaliveStopsBeforeHandingOverWriter(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, keepaliveTestInterval)
	waitForKeepaliveBeats()
	stop()

	before := rec.Body.String()
	waitForKeepaliveBeats()
	require.Equal(t, before, rec.Body.String(), "停拍后不应再有字节写出")

	// 停拍后主循环写出的内容不应被心跳穿插。
	_, err := c.Writer.Write([]byte("data: real\n\n"))
	require.NoError(t, err)
	waitForKeepaliveBeats()
	require.True(t, strings.HasSuffix(rec.Body.String(), "data: real\n\n"),
		"停拍后写入应当是响应体的最后一段")
}

// interval<=0(配置禁用)时行为与改动前完全一致:一个字节都不写。
func TestPassthroughKeepaliveDisabledKeepsWriterUntouched(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, 0)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len())
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	_ = time.Now
}
