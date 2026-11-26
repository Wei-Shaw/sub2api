package service

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const keepaliveTestInterval = 10 * time.Millisecond

// waitForKeepaliveBeats 等待至少一次心跳写出。读取 recorder 前必须先经
// StopOpenAICompactSSEKeepaliveCommitted 停拍建立 happens-before。
func waitForKeepaliveBeats() {
	time.Sleep(20 * keepaliveTestInterval)
}

// stripKeepaliveComments 去掉 SSE 注释块，返回真实事件文本。
func stripKeepaliveComments(body string) string {
	var blocks []string
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(block), ":") {
			continue
		}
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n\n")
}

func TestStartOpenAICompactSSEKeepalive_NoopWhenUnmarkedOrDisabled(t *testing.T) {
	// 未标记 client stream：不启动。
	c, rec := newCompactBridgeTestContext(t, false)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len())
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))

	// interval=0（配置禁用）：不启动。
	c, rec = newCompactBridgeTestContext(t, true)
	stop = StartOpenAICompactSSEKeepalive(c, 0)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len())
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
}

func TestOpenAICompactSSEKeepalive_CommitsHeadersAndComments(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
}

func TestOpenAICompactSSEKeepalive_StopBeforeFirstBeatKeepsWriterUntouched(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	stop()
	waitForKeepaliveBeats()
	require.Zero(t, rec.Body.Len())
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
}

func TestOpenAIAdjustedWrittenSizeExcludesResponsesStreamKeepalive(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, false)
	n, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	recordOpenAIStreamKeepaliveBytes(c, n)

	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))

	_, err = c.Writer.Write([]byte("data: semantic\n\n"))
	require.NoError(t, err)
	require.Equal(t, len("data: semantic\n\n"), OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.Equal(t, ":\n\ndata: semantic\n\n", rec.Body.String())
}

// 心跳已提交后，2xx 桥接续写事件而不重复提交响应头。
func TestWriteOpenAICompactSSEBridge_AfterKeepaliveCommitAppendsEvents(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	finalResponse := []byte(`{"id":"resp_ka_1","output":[{"id":"cmp_ka","type":"compaction","encrypted_content":"x"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	require.True(t, writeOpenAICompactSSEBridge(c, http.StatusOK, finalResponse))

	require.Equal(t, http.StatusOK, rec.Code)
	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 2)
	require.Equal(t, "response.output_item.done", events[0][0])
	require.Equal(t, "compaction", gjson.Get(events[0][1], "item.type").String())
	require.Equal(t, "response.completed", events[1][0])
	require.Equal(t, "resp_ka_1", gjson.Get(events[1][1], "response.id").String())
}

// 心跳已提交后上游非 2xx：状态码无法回传，必须以 response.failed 终止事件
// 收尾（Codex 将其作为终止事件处理），并标记流内错误供 ops 采集。
func TestWriteOpenAICompactSSEBridge_AfterKeepaliveCommitFailureEmitsFailedEvent(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	require.True(t, writeOpenAICompactSSEBridge(c, http.StatusBadGateway, []byte(`{"error":{"message":"upstream exploded"}}`)))

	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 1)
	require.Equal(t, "response.failed", events[0][0])
	require.Equal(t, "failed", gjson.Get(events[0][1], "response.status").String())
	require.Contains(t, gjson.Get(events[0][1], "response.error.message").String(), "upstream exploded")
	require.NotEmpty(t, gjson.Get(events[0][1], "response.id").String())

	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, streamErr.IntendedStatus)
}

// 心跳未提交时非 2xx 行为不变：返回 false，调用方按原 JSON+状态码写回。
func TestWriteOpenAICompactSSEBridge_BeforeKeepaliveCommitFailureKeepsJSONPath(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	stop()

	require.False(t, writeOpenAICompactSSEBridge(c, http.StatusBadGateway, []byte(`{"error":{"message":"fast fail"}}`)))
	require.Zero(t, rec.Body.Len())
}

// 未被显式拦截的写回路径（直接操作 c.Writer）也必须与心跳互斥：包装器在
// 请求侧任何响应构造时停拍。-race 下验证无数据竞争，且停拍后不再有心跳
// 字节写出。
func TestOpenAICompactKeepaliveWriter_RequestSideWriteSuspendsBeats(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	// 模拟未拦截路径的直接写回（如 Forward 内部本地拒绝的 c.JSON）。
	_, err := c.Writer.Write([]byte(`{"error":"local reject"}`))
	require.NoError(t, err)

	lenAfterWrite := rec.Body.Len()
	waitForKeepaliveBeats()
	require.Equal(t, lenAfterWrite, rec.Body.Len(), "请求侧写回后心跳必须停止")
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
	require.Contains(t, rec.Body.String(), `{"error":"local reject"}`)
}

func TestOpenAICompactKeepaliveWriter_NilInnerWriter_NoPanic(t *testing.T) {
	w := &openAICompactKeepaliveWriter{
		k: &openAICompactSSEKeepalive{stop: make(chan struct{})},
	}
	w.ResponseWriter = nil

	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Status())
	})
	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Size())
	})
	assert.NotPanics(t, func() {
		assert.False(t, w.Written())
	})
	assert.NotPanics(t, func() {
		assert.NotNil(t, w.Header())
	})
	assert.NotPanics(t, func() {
		n, err := w.Write([]byte("test"))
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
	})
	assert.NotPanics(t, func() {
		n, err := w.WriteString("test")
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
	})
	assert.NotPanics(t, func() {
		w.WriteHeader(http.StatusOK)
	})
	assert.NotPanics(t, func() {
		w.WriteHeaderNow()
	})
	assert.NotPanics(t, func() {
		w.Flush()
	})
	assert.NotPanics(t, func() {
		conn, rw, err := w.Hijack()
		assert.Nil(t, conn)
		assert.Nil(t, rw)
		assert.Error(t, err)
	})
	assert.NotPanics(t, func() {
		ch := w.CloseNotify()
		assert.NotNil(t, ch)
	})
	assert.NotPanics(t, func() {
		assert.Nil(t, w.Pusher())
	})
}

func TestOpenAICompactKeepaliveWriter_NilKeepalive_NoPanic(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	w := &openAICompactKeepaliveWriter{ResponseWriter: c.Writer}

	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Status())
	})
	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Size())
	})
	assert.NotPanics(t, func() {
		assert.False(t, w.Written())
	})
	assert.NotPanics(t, func() {
		w.Header().Set("X-Test", "ok")
	})
	assert.NotPanics(t, func() {
		w.WriteHeader(http.StatusAccepted)
	})
	assert.NotPanics(t, func() {
		n, err := w.WriteString("ok")
		assert.Equal(t, 2, n)
		assert.NoError(t, err)
	})
	assert.NotPanics(t, func() {
		w.Flush()
	})
	require.Equal(t, "ok", rec.Header().Get("X-Test"))
	require.Equal(t, "ok", rec.Body.String())
}

func TestOpenAICompactKeepaliveWriter_DelegatesWhenReady(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	defer stop()

	w, ok := c.Writer.(*openAICompactKeepaliveWriter)
	require.True(t, ok)

	w.Header().Set("X-Test", "ok")
	w.WriteHeader(http.StatusAccepted)
	n, err := w.WriteString("ready")
	require.NoError(t, err)
	require.Equal(t, len("ready"), n)

	require.Equal(t, http.StatusAccepted, w.Status())
	require.Equal(t, len("ready"), w.Size())
	require.True(t, w.Written())
	require.Equal(t, "ok", rec.Header().Get("X-Test"))
	require.Equal(t, "ready", rec.Body.String())
}

// fast policy block 在心跳提交后必须降级为 response.failed 终止事件。
func TestWriteOpenAIFastPolicyBlockedResponse_AfterKeepaliveCommit(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	writeOpenAIFastPolicyBlockedResponse(c, &OpenAIFastBlockedError{Message: "tier blocked"})

	require.Equal(t, http.StatusOK, rec.Code)
	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 1)
	require.Equal(t, "response.failed", events[0][0])
	require.Equal(t, "permission_error", gjson.Get(events[0][1], "response.error.code").String())
	require.Contains(t, gjson.Get(events[0][1], "response.error.message").String(), "tier blocked")
}

// failover"是否已写响应"判定的口径：心跳字节必须被排除，否则 compact 在
// 上游等待期间发过心跳后，可换号的 failover 会被误判放弃；真实响应字节
// 写出后口径必须变化。
func TestOpenAICompactKeepaliveAdjustedWrittenSize_ExcludesHeartbeatBytes(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	// 无心跳的请求：等价于 c.Writer.Size()。
	require.Equal(t, c.Writer.Size(), OpenAICompactKeepaliveAdjustedWrittenSize(c))

	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	before := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	waitForKeepaliveBeats()
	require.Equal(t, before, OpenAICompactKeepaliveAdjustedWrittenSize(c), "仅心跳字节不得改变判定口径")

	// 真实响应字节写出（经包装器，先停拍再写）后口径必须变化。
	_, err := c.Writer.Write([]byte("real-bytes"))
	require.NoError(t, err)
	require.Equal(t, len("real-bytes"), OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
}

func TestOpenAIStreamClientOutputStarted_IgnoresCompactKeepaliveBytes(t *testing.T) {
	c, _ := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	require.True(t, c.Writer.Written())
	require.False(t, openAIStreamClientOutputStarted(c, false), "keepalive comments are not semantic output")

	_, err := c.Writer.Write([]byte("real-output"))
	require.NoError(t, err)
	require.True(t, openAIStreamClientOutputStarted(c, false))
}

// fast policy block 在心跳未提交时保持 403 JSON 原语义。
func TestWriteOpenAIFastPolicyBlockedResponse_BeforeKeepaliveCommit(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	defer stop()

	writeOpenAIFastPolicyBlockedResponse(c, &OpenAIFastBlockedError{Message: "tier blocked"})

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "permission_error", gjson.Get(rec.Body.String(), "error.type").String())
}

// keepaliveInstance 取出当前请求上的心跳器；测试用 time.Hour 间隔启动并手动
// beat，不依赖 goroutine 调度时序。
func keepaliveInstance(t *testing.T, c *gin.Context) *openAICompactSSEKeepalive {
	t.Helper()
	value, ok := c.Get(openAICompactSSEKeepaliveKey)
	require.True(t, ok)
	k, ok := value.(*openAICompactSSEKeepalive)
	require.True(t, ok)
	require.NotNil(t, k)
	return k
}

// 透传路径每次尝试都会新建心跳器：上一次尝试的注释字节仍在底层 writer 的
// Size 里，必须跨实例累计扣除，否则下一次尝试会把旧心跳误判为语义输出。
func TestOpenAICompactKeepaliveAdjustedWrittenSize_CumulativeAcrossRestarts(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)

	stop := startOpenAISSEKeepalive(c, time.Hour)
	require.True(t, keepaliveInstance(t, c).beat())
	stop()
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "仅心跳时必须归一化为未写出")
	require.Contains(t, rec.Body.String(), ": keepalive")

	stop = startOpenAISSEKeepalive(c, time.Hour)
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "新实例尚未心跳时，旧实例的心跳字节也必须扣除")
	require.False(t, openAIStreamClientOutputStarted(c, false))
	require.True(t, keepaliveInstance(t, c).beat())
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "两次尝试的心跳字节必须全部扣除")

	_, err := c.Writer.Write([]byte("real-output"))
	require.NoError(t, err)
	stop()
	require.Equal(t, len("real-output"), OpenAICompactKeepaliveAdjustedWrittenSize(c), "语义输出仍须被识别")
	require.True(t, openAIStreamClientOutputStarted(c, false))
}

// 上一次尝试的心跳已把响应头提交为 200 时，即使当前实例从未心跳，
// committed 判定也必须为真，写回方才会降级为流内终止事件。
func TestStopOpenAICompactSSEKeepaliveCommitted_PreviousAttemptCommitted(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)

	stop := startOpenAISSEKeepalive(c, time.Hour)
	require.True(t, keepaliveInstance(t, c).beat())
	stop()

	stop = startOpenAISSEKeepalive(c, time.Hour)
	defer stop()
	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.Equal(t, http.StatusOK, rec.Code)
}

// 单次心跳器从未心跳时仍报告未提交（既有行为不变）。
func TestStopOpenAICompactSSEKeepaliveCommitted_RestartWithoutBeats(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)

	stop := startOpenAISSEKeepalive(c, time.Hour)
	stop()
	stop = startOpenAISSEKeepalive(c, time.Hour)
	defer stop()
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
	require.Zero(t, rec.Body.Len())
}

// 外层心跳活跃时再启动内层心跳：两者共享同一累计计数，内层停止后外层已写的
// 注释不能被重新算成语义输出。
func TestOpenAICompactKeepaliveAdjustedWrittenSize_NestedKeepalives(t *testing.T) {
	c, rec := newCompactBridgeTestContext(t, true)

	stopOuter := StartOpenAICompactSSEKeepalive(c, time.Hour)
	outer := keepaliveInstance(t, c)
	require.True(t, outer.beat())
	stopInner := startOpenAISSEKeepalive(c, time.Hour)
	require.True(t, keepaliveInstance(t, c).beat())
	stopInner()
	stopOuter()

	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "嵌套心跳的注释字节必须全部扣除")
	require.Equal(t, 2, strings.Count(rec.Body.String(), ": keepalive\n\n"))

	_, err := c.Writer.Write([]byte("real-output"))
	require.NoError(t, err)
	require.Equal(t, len("real-output"), OpenAICompactKeepaliveAdjustedWrittenSize(c))
}
