package service

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// openAICompactSSEKeepaliveKey 存放 body-signal compact 请求的下游 SSE 心跳器。
const openAICompactSSEKeepaliveKey = "openai_compact_sse_keepalive"

// openAICompactSSEKeepalive 在 compact 上游 unary 等待期间向下游写 SSE 注释行
// 心跳。上游 /responses/compact 在模型处理期间不发送任何字节（大上下文可长达
// 数分钟），下游若经过反向代理（Nginx/Cloudflare Tunnel 等），零字节静默会触发
// 代理的空闲/读超时并掐断连接，Codex 只会盲目重连并重复消耗上游 compact
// 配额（#3887）。SSE 注释行在 eventsource 解析层被直接忽略，不会进入客户端
// 事件流。
//
// 首拍延迟一个 interval：绝大多数硬错误（鉴权/参数/限流）在此之前返回，仍走
// 原 JSON+状态码链路（Codex 按 HTTP 状态码重试）；首拍之后状态码固化为 200，
// 后续错误由写回方降级为 response.failed 流内终止事件。
type openAICompactSSEKeepalive struct {
	mu      sync.Mutex
	writer  gin.ResponseWriter
	started bool
	stopped bool
	// bytes 是心跳已写出的注释字节数。心跳不构成语义响应，handler 的
	// "Forward 期间是否已写响应"判定（failover 放弃换号的依据）必须扣除
	// 这部分字节，见 OpenAICompactKeepaliveAdjustedWrittenSize。
	bytes int
	stop  chan struct{}
}

// StartOpenAICompactSSEKeepalive 为已标记 body-signal 客户端流式的 compact
// 请求启动下游心跳，返回幂等的停止函数。interval<=0 或请求未标记时为 no-op。
//
// 同时把 c.Writer 替换为 openAICompactKeepaliveWriter：请求 goroutine 的任何
// 响应构造都会先在心跳互斥锁下停拍，未被显式拦截的写回路径（如 Forward
// 内部的本地拒绝）也不会与心跳 goroutine 产生数据竞争或字节交错。
func StartOpenAICompactSSEKeepalive(c *gin.Context, interval time.Duration) func() {
	if c == nil || c.Writer == nil || interval <= 0 || !openAICompactClientWantsStream(c) {
		return func() {}
	}
	k := &openAICompactSSEKeepalive{
		writer: c.Writer,
		stop:   make(chan struct{}),
	}
	c.Set(openAICompactSSEKeepaliveKey, k)
	c.Writer = &openAICompactKeepaliveWriter{ResponseWriter: c.Writer, k: k}

	var reqDone <-chan struct{}
	if c.Request != nil {
		reqDone = c.Request.Context().Done()
	}
	go func() {
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-k.stop:
				return
			case <-reqDone:
				return
			case <-timer.C:
			}
			if !k.beat() {
				return
			}
			timer.Reset(interval)
		}
	}()
	return k.Stop
}

// beat 在锁内提交（首次）响应头并写出一条 SSE 注释行；返回 false 表示心跳已
// 停止或下游写入失败，goroutine 应退出。
func (k *openAICompactSSEKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped {
		return false
	}
	if !k.started {
		header := k.writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		k.writer.WriteHeader(http.StatusOK)
		k.started = true
	}
	n, err := k.writer.Write([]byte(": keepalive\n\n"))
	k.bytes += n
	if err != nil {
		k.stopped = true
		return false
	}
	k.writer.Flush()
	return true
}

// Stop 停止心跳；幂等，可与写回路径并发调用。
func (k *openAICompactSSEKeepalive) Stop() {
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
}

func (k *openAICompactSSEKeepalive) markStoppedLocked() {
	if k.stopped {
		return
	}
	k.stopped = true
	close(k.stop)
}

// StopOpenAICompactSSEKeepaliveCommitted 停止当前请求的 compact 心跳（若有）
// 并报告响应头是否已被心跳提交为 200。写回方以此决定继续走原 JSON/状态码
// 链路，还是降级为流内终止事件。调用后不会再有心跳字节写出，且经由互斥锁
// 与心跳 goroutine 建立 happens-before，调用方可安全接管 ResponseWriter。
func StopOpenAICompactSSEKeepaliveCommitted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(openAICompactSSEKeepaliveKey)
	if !ok {
		return false
	}
	k, ok := value.(*openAICompactSSEKeepalive)
	if !ok || k == nil {
		return false
	}
	k.mu.Lock()
	k.markStoppedLocked()
	committed := k.started
	k.mu.Unlock()
	return committed
}

// OpenAICompactKeepaliveAdjustedWrittenSize 返回排除 compact 心跳注释字节后
// 的响应已写字节数；无心跳的请求等价于 c.Writer.Size()。心跳字节不构成语义
// 响应——handler 以"Forward 前后 Size 是否变化"判定是否已向客户端写出响应
// （变化则放弃 failover 换号），该判定不得被心跳污染，否则 compact 请求
// 一旦在上游等待期间发过心跳，上游 429/5xx 就不再换号（#3887 加固审计）。
// 仅心跳字节时归一化为 -1（gin 的"未写出"哨兵值），与提交前的快照可比。
func OpenAICompactKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	if c == nil || c.Writer == nil {
		return -1
	}
	value, ok := c.Get(openAICompactSSEKeepaliveKey)
	if !ok {
		return c.Writer.Size()
	}
	k, ok := value.(*openAICompactSSEKeepalive)
	if !ok || k == nil {
		return c.Writer.Size()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	size := k.writer.Size()
	if size < 0 {
		return size
	}
	if real := size - k.bytes; real > 0 {
		return real
	}
	return -1
}

// openAICompactKeepaliveWriter 包装 gin.ResponseWriter：写侧方法先停拍心跳
// （互斥锁下建立 happens-before），读侧方法仅加锁不停拍——热路径的状态读取
// （如 Forward 前的 Size 快照）不能误杀心跳。心跳 goroutine 直接写内层
// writer（k.writer），不经过本包装器，不会递归。
type openAICompactKeepaliveWriter struct {
	gin.ResponseWriter
	k *openAICompactSSEKeepalive
}

// suspend 停拍心跳；幂等。任何响应构造（含 Header 访问——写响应必先操作
// 响应头）都视为请求侧接管 ResponseWriter。
func (w *openAICompactKeepaliveWriter) suspend() {
	w.k.Stop()
}

func (w *openAICompactKeepaliveWriter) Header() http.Header {
	w.suspend()
	return w.ResponseWriter.Header()
}

func (w *openAICompactKeepaliveWriter) Write(data []byte) (int, error) {
	w.suspend()
	return w.ResponseWriter.Write(data)
}

func (w *openAICompactKeepaliveWriter) WriteString(s string) (int, error) {
	w.suspend()
	return w.ResponseWriter.WriteString(s)
}

func (w *openAICompactKeepaliveWriter) WriteHeader(code int) {
	w.suspend()
	w.ResponseWriter.WriteHeader(code)
}

func (w *openAICompactKeepaliveWriter) WriteHeaderNow() {
	w.suspend()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *openAICompactKeepaliveWriter) Flush() {
	w.suspend()
	w.ResponseWriter.Flush()
}

func (w *openAICompactKeepaliveWriter) Status() int {
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Status()
}

func (w *openAICompactKeepaliveWriter) Size() int {
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Size()
}

func (w *openAICompactKeepaliveWriter) Written() bool {
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Written()
}
