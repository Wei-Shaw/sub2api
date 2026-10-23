package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 这一组守住的是「心跳注释行不得插进一帧中间」。
//
// 下游其实有两条通往同一个连接的写入通道：帧走 firstOutputStage / bufferedWriter，
// 心跳在 stageFirstOutput 时直写 ResponseWriter。上游 PR #4275 加的 eventInProgress
// 守的是上游帧的逻辑边界，管不到下游缓冲里的物理残留；4 小时后合入的 PR #4307 引入
// 暂存区与直写旁路，这条不变量就被绕过去了。线上实测坏帧：
//
//	data: {"type":"response.function_call_arguments.delta","sequen:\n\nce_number":3287,...}
//
// 客户端解析器遇到帧内空行会提前提交事件，拿到半个 JSON 直接报 Extra data。

func TestOpenAIKeepaliveMayBypassDownstreamBuffer(t *testing.T) {
	// 首个可见输出之前：帧全在暂存区，w 上没有未提交字节，直写安全。
	require.True(t, openAIKeepaliveMayBypassDownstreamBuffer(true, 0))
	// 暂存区已提交、缓冲里还压着字节：直写会抢在它们前面劈开帧。
	require.False(t, openAIKeepaliveMayBypassDownstreamBuffer(true, 1))
	require.False(t, openAIKeepaliveMayBypassDownstreamBuffer(true, 4096))
	// 非 OpenAI 平台本就走缓冲路径，不存在旁路。
	require.False(t, openAIKeepaliveMayBypassDownstreamBuffer(false, 0))
	require.False(t, openAIKeepaliveMayBypassDownstreamBuffer(false, 1))
}

// 🔴 回归用例：缓冲里压着未提交的帧时，心跳必须排在它后面，不能把帧劈开。
//
// 构造出「缓冲非空 + eventInProgress=false + 循环空转」这个状态需要三步：
//  1. 先发一个可见输出事件，把 attempt-local 暂存区提交掉 —— 之后帧才改走 bufferedWriter；
//  2. 再发一个超过 4KB 的大帧，并让它的空行在【队列未排空】时到达（shouldFlush 为假），
//     帧尾就留在缓冲里；
//  3. 紧跟一批裸 error 事件，它们会被 suppressCurrentEvent 吃掉：不写字节、不刷缓冲，
//     却会把 eventInProgress 归零。队列排空后循环空转，心跳拍子到点。
func TestOpenAIResponseFlush_KeepaliveDoesNotJumpBufferedFrame(t *testing.T) {
	firstOutput := `data: {"type":"response.output_text.delta","delta":"a"}` + "\n\n"
	// 9000 字节远超 bufferedWriter 的 4KB：写入过程中 bufio 会自动 flush 整块，
	// 切点落在哪个字节与帧边界无关，剩余部分留在缓冲里。
	bigFrame := `data: {"type":"response.function_call_arguments.delta","sequence_number":3287,"delta":"` +
		strings.Repeat("x", 9000) + `"}` + "\n\n"
	// 17 个事件 = 34 行，足以灌满 16 格的扫描队列，保证大帧的空行到达时队列未排空。
	suppressed := strings.Repeat(`data: {"type":"error","error":{"message":"x"}}`+"\n\n", 17)
	terminal := "data: [DONE]\n\n"

	allowBurst := make(chan struct{})
	allowTerminal := make(chan struct{})
	burstWaiting := make(chan struct{})
	terminalWaiting := make(chan struct{})
	reader := &stagedOpenAISSEReadCloser{
		segments: [][]byte{[]byte(firstOutput), []byte(bigFrame + suppressed), []byte(terminal)},
		gates:    []<-chan struct{}{nil, allowBurst, allowTerminal},
		waiting:  []chan struct{}{nil, burstWaiting, terminalWaiting},
	}
	recorder := newOpenAIResponseFlushRecorder()
	resultCh, errCh := runOpenAIResponseFlushTestAsync(recorder, reader, config.GatewayConfig{StreamKeepaliveInterval: 1})

	// 等首个可见输出落地：这一刷把暂存区提交掉，后续帧才会进 bufferedWriter。
	waitOpenAIResponseFlushCount(t, recorder, 1)
	waitOpenAIResponseFlushSignal(t, burstWaiting)
	close(allowBurst)

	// 读侧停在终止帧前，主循环空转，心跳拍子（1s）必然到点。
	waitOpenAIResponseFlushSignal(t, terminalWaiting)
	time.Sleep(1500 * time.Millisecond)
	close(allowTerminal)

	<-errCh
	<-resultCh

	gotBody, _ := recorder.snapshot()
	require.Contains(t, gotBody, ":\n\n", "心跳应当真的打出来了，否则这条用例没有守住任何东西")
	require.Contains(t, gotBody, bigFrame, "心跳注释行插进了帧中间，大帧在下游不再连续")
	assertOpenAISSEDataLinesIntact(t, gotBody)
}

// assertOpenAISSEDataLinesIntact 逐行检查：注释行只能独立成行，不得出现在 data 行内部。
func assertOpenAISSEDataLinesIntact(t *testing.T, body string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		require.False(t, strings.HasSuffix(payload, ":"),
			"data 行以冒号结尾，说明心跳注释行被拼了进来: %q", line)
	}
}

// 心跳包装器的语义：帧写入必须经过 c.Writer（包装器）才能在锁内停拍。
// 启动心跳【之前】捕获的裸 writer 不是包装器，用它写帧会绕过 suspend()，与心跳
// goroutine 之间没有任何互斥 —— 这正是透传路径必须在 startOpenAISSEKeepalive
// 之后重新取一次 writer 的原因。
//
// 这里只做身份判定与「经包装器写入后心跳停拍」的验证，不去真的用裸 writer 并发写：
// 那是一个确定的数据竞争，-race 下必然报错，不该写进用例里。
func TestOpenAISSEKeepaliveSwapsWriterSoFrameWritesSuspendBeats(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	staleWriter := c.Writer

	stop := startOpenAISSEKeepalive(c, keepaliveTestInterval)
	defer stop()

	wrapped, isWrapper := c.Writer.(*openAICompactKeepaliveWriter)
	require.True(t, isWrapper, "心跳启动后 c.Writer 必须换成带互斥锁的包装器")
	require.NotEqual(t, staleWriter, gin.ResponseWriter(wrapped),
		"启动前捕获的 writer 不是包装器：继续用它写帧就绕过了 suspend()")

	waitForKeepaliveBeats()

	// 经包装器写帧：WriteString 先在锁内停拍，返回后心跳 goroutine 不会再写字节，
	// 与它之间已建立 happens-before，这之后读 recorder 是安全的。
	_, err := io.WriteString(c.Writer, "data: {\"type\":\"response.in_progress\"}\n\n")
	require.NoError(t, err)

	body := rec.Body.String()
	require.Contains(t, body, ": keepalive\n\n", "心跳应当已经打过拍")
	require.True(t, strings.HasSuffix(body, "data: {\"type\":\"response.in_progress\"}\n\n"),
		"帧必须是最后写出的字节，心跳不得追加在它后面")

	waitForKeepaliveBeats()
	require.Equal(t, body, rec.Body.String(), "经包装器写帧之后心跳必须已停拍")
}

var _ http.Flusher = (*openAICompactKeepaliveWriter)(nil)
