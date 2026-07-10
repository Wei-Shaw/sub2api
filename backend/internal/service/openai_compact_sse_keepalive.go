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
	stop    chan struct{REDACTED
REDACTED

// StartOpenAICompactSSEKeepalive 为已标记 body-signal 客户端流式的 compact
// 请求启动下游心跳，返回幂等的停止函数。interval<=0 或请求未标记时为 no-op。
func StartOpenAICompactSSEKeepalive(c *gin.Context, interval time.Duration) func() {
	if c == nil || c.Writer == nil || interval <= 0 || !openAICompactClientWantsStream(c) {
		return func() {REDACTED
REDACTED
	k := &openAICompactSSEKeepalive{
		writer: c.Writer,
		stop:   make(chan struct{REDACTED),
REDACTED
	c.Set(openAICompactSSEKeepaliveKey, k)

	var reqDone <-chan struct{REDACTED
	if c.Request != nil {
		reqDone = c.Request.Context().Done()
REDACTED
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
		REDACTED
			if !k.beat() {
				return
		REDACTED
			timer.Reset(interval)
	REDACTED
REDACTED()
	return k.Stop
REDACTED

// beat 在锁内提交（首次）响应头并写出一条 SSE 注释行；返回 false 表示心跳已
// 停止或下游写入失败，goroutine 应退出。
func (k *openAICompactSSEKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped {
		return false
REDACTED
	if !k.started {
		header := k.writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		k.writer.WriteHeader(http.StatusOK)
		k.started = true
REDACTED
	if _, err := k.writer.Write([]byte(": keepalive\n\n")); err != nil {
		k.stopped = true
		return false
REDACTED
	k.writer.Flush()
	return true
REDACTED

// Stop 停止心跳；幂等，可与写回路径并发调用。
func (k *openAICompactSSEKeepalive) Stop() {
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
REDACTED

func (k *openAICompactSSEKeepalive) markStoppedLocked() {
	if k.stopped {
		return
REDACTED
	k.stopped = true
	close(k.stop)
REDACTED

// StopOpenAICompactSSEKeepaliveCommitted 停止当前请求的 compact 心跳（若有）
// 并报告响应头是否已被心跳提交为 200。写回方以此决定继续走原 JSON/状态码
// 链路，还是降级为流内终止事件。调用后不会再有心跳字节写出，且经由互斥锁
// 与心跳 goroutine 建立 happens-before，调用方可安全接管 ResponseWriter。
func StopOpenAICompactSSEKeepaliveCommitted(c *gin.Context) bool {
	if c == nil {
		return false
REDACTED
	value, ok := c.Get(openAICompactSSEKeepaliveKey)
	if !ok {
		return false
REDACTED
	k, ok := value.(*openAICompactSSEKeepalive)
	if !ok || k == nil {
		return false
REDACTED
	k.mu.Lock()
	k.markStoppedLocked()
	committed := k.started
	k.mu.Unlock()
	return committed
REDACTED
