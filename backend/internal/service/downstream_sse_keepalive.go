package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	anthropicPreHeaderSSEKeepaliveKey = "anthropic_pre_header_sse_keepalive"
	openAIPreHeaderSSEKeepaliveKey    = "openai_pre_header_sse_keepalive"
)

var (
	anthropicSSEKeepalivePayload = []byte("event: ping\ndata: {\"type\": \"ping\"}\n\n")
	openAISSEKeepalivePayload    = []byte(":\n\n")
)

// downstreamSSEKeepalive serializes an independent heartbeat writer with the
// request goroutine's first real response write. Heartbeats only touch the
// downstream ResponseWriter; the upstream request/body is never involved.
type downstreamSSEKeepalive struct {
	mu              sync.Mutex
	writer          gin.ResponseWriter
	wrapper         *downstreamSSEKeepaliveWriter
	payload         []byte
	suspendOnHeader bool
	started         bool
	stopped         bool
	bytes           int
	stop            chan struct{}
}

func startDownstreamSSEKeepalive(
	c *gin.Context,
	key string,
	interval time.Duration,
	payload []byte,
	enabled bool,
	suspendOnHeader bool,
) func() {
	if c == nil || c.Writer == nil || interval <= 0 || !enabled || len(payload) == 0 {
		return func() {}
	}
	if existing := downstreamSSEKeepaliveForKey(c, key); existing != nil {
		return func() { stopDownstreamSSEKeepalive(c, key) }
	}

	originalWriter := c.Writer
	k := &downstreamSSEKeepalive{
		writer:          originalWriter,
		payload:         append([]byte(nil), payload...),
		suspendOnHeader: suspendOnHeader,
		stop:            make(chan struct{}),
	}
	wrappedWriter := &downstreamSSEKeepaliveWriter{ResponseWriter: originalWriter, k: k}
	k.wrapper = wrappedWriter
	c.Set(key, k)
	c.Writer = wrappedWriter

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

	return func() { stopDownstreamSSEKeepalive(c, key) }
}

func (k *downstreamSSEKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped || k.writer == nil {
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
	n, err := k.writer.Write(k.payload)
	k.bytes += n
	if err != nil {
		k.markStoppedLocked()
		return false
	}
	k.writer.Flush()
	return true
}

func (k *downstreamSSEKeepalive) Stop() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
}

func (k *downstreamSSEKeepalive) markStoppedLocked() {
	if k.stopped {
		return
	}
	k.stopped = true
	close(k.stop)
}

func downstreamSSEKeepaliveForKey(c *gin.Context, key string) *downstreamSSEKeepalive {
	if c == nil {
		return nil
	}
	value, ok := c.Get(key)
	if !ok {
		return nil
	}
	k, _ := value.(*downstreamSSEKeepalive)
	return k
}

func stopDownstreamSSEKeepalive(c *gin.Context, key string) bool {
	k := downstreamSSEKeepaliveForKey(c, key)
	if k == nil {
		return false
	}
	k.mu.Lock()
	k.markStoppedLocked()
	committed := k.started
	wrapper := k.wrapper
	originalWriter := k.writer
	k.mu.Unlock()
	if c != nil && c.Writer == wrapper {
		c.Writer = originalWriter
	}
	return committed
}

func downstreamSSEKeepaliveCommitted(c *gin.Context, key string) bool {
	k := downstreamSSEKeepaliveForKey(c, key)
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.started
}

func downstreamSSEKeepaliveAdjustedWrittenSize(c *gin.Context, keys ...string) int {
	if c == nil || c.Writer == nil {
		return -1
	}
	writer := c.Writer
	heartbeatBytes := 0
	seen := make(map[*downstreamSSEKeepalive]struct{}, len(keys))
	locked := make([]*downstreamSSEKeepalive, 0, len(keys))
	for _, key := range keys {
		k := downstreamSSEKeepaliveForKey(c, key)
		if k == nil {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		k.mu.Lock()
		locked = append(locked, k)
		writer = k.writer
		heartbeatBytes += k.bytes
	}
	// Size and heartbeat bytes must be observed under the same lock as beat's
	// downstream Write; otherwise gin.ResponseWriter.Size races with the timer.
	// Unwrap our own writers while their locks are already held so an accidental
	// helper overlap cannot call Size through a wrapper and lock itself again.
	for {
		wrapped, ok := writer.(*downstreamSSEKeepaliveWriter)
		if !ok || wrapped.ResponseWriter == nil {
			break
		}
		writer = wrapped.ResponseWriter
	}
	size := writer.Size()
	for i := len(locked) - 1; i >= 0; i-- {
		locked[i].mu.Unlock()
	}
	if size < 0 || heartbeatBytes <= 0 {
		return size
	}
	if real := size - heartbeatBytes; real > 0 {
		return real
	}
	return -1
}

// EnsureAnthropicPreHeaderSSEKeepalive starts a request-scoped pre-header
// heartbeat once. The handler owns final cleanup so it survives account
// retry/failover. The first beat is delayed by one complete interval.
func EnsureAnthropicPreHeaderSSEKeepalive(c *gin.Context, interval time.Duration) {
	_ = startDownstreamSSEKeepalive(c, anthropicPreHeaderSSEKeepaliveKey, interval, anthropicSSEKeepalivePayload, true, false)
}

func StopAnthropicPreHeaderSSEKeepaliveCommitted(c *gin.Context) bool {
	return stopDownstreamSSEKeepalive(c, anthropicPreHeaderSSEKeepaliveKey)
}

func AnthropicPreHeaderSSEKeepaliveCommitted(c *gin.Context) bool {
	return downstreamSSEKeepaliveCommitted(c, anthropicPreHeaderSSEKeepaliveKey)
}

func AnthropicPreHeaderKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	return downstreamSSEKeepaliveAdjustedWrittenSize(c, anthropicPreHeaderSSEKeepaliveKey)
}

func isAnthropicMessagesRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.URL != nil && c.Request.URL.Path == "/v1/messages"
}

func writeAnthropicGatewayErrorResponse(c *gin.Context, status int, errType, message string) {
	if AnthropicJSONKeepalivePresent(c) {
		if StopAnthropicJSONKeepaliveCommitted(c) {
			MarkOpsStreamError(c, errType, message, status)
		}
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": message,
			},
		})
		return
	}
	if !StopAnthropicPreHeaderSSEKeepaliveCommitted(c) {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": message,
			},
		})
		return
	}
	MarkResponseCommitted(c)
	MarkOpsStreamError(c, errType, message, status)
	payload, err := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	if err != nil {
		return
	}
	_, _ = c.Writer.Write([]byte("event: error\ndata: "))
	_, _ = c.Writer.Write(payload)
	_, _ = c.Writer.Write([]byte("\n\n"))
	c.Writer.Flush()
}

// EnsureOpenAIPreHeaderSSEKeepalive uses the Responses-compatible SSE comment.
// Standard Forward stops it when response headers arrive; passthrough keeps it
// until the first real downstream write so buffered reasoning cannot go silent.
func EnsureOpenAIPreHeaderSSEKeepalive(c *gin.Context, interval time.Duration) {
	// Body-signal compact requests start their own request-scoped heartbeat in
	// the handler before Forward. Do not stack a second writer/timer on it.
	if downstreamSSEKeepaliveForKey(c, openAICompactSSEKeepaliveKey) != nil {
		return
	}
	_ = startDownstreamSSEKeepalive(c, openAIPreHeaderSSEKeepaliveKey, interval, openAISSEKeepalivePayload, true, false)
}

func StopOpenAIPreHeaderSSEKeepaliveCommitted(c *gin.Context) bool {
	return stopDownstreamSSEKeepalive(c, openAIPreHeaderSSEKeepaliveKey)
}

func OpenAIPreHeaderSSEKeepaliveCommitted(c *gin.Context) bool {
	return downstreamSSEKeepaliveCommitted(c, openAIPreHeaderSSEKeepaliveKey)
}

// downstreamSSEKeepaliveWriter stops the independent writer before the first
// request-side response write. Header access may remain non-terminal for the
// pre-header helper so passthrough can install upstream headers while buffering
// non-visible preamble events.
type downstreamSSEKeepaliveWriter struct {
	gin.ResponseWriter
	k *downstreamSSEKeepalive
}

func (w *downstreamSSEKeepaliveWriter) suspend() {
	if w.k != nil {
		w.k.Stop()
	}
}

func (w *downstreamSSEKeepaliveWriter) Header() http.Header {
	if w.k != nil && w.k.suspendOnHeader {
		w.suspend()
	}
	if w.ResponseWriter == nil {
		return http.Header{}
	}
	return w.ResponseWriter.Header()
}

func (w *downstreamSSEKeepaliveWriter) Write(data []byte) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *downstreamSSEKeepaliveWriter) WriteString(s string) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.WriteString(s)
}

func (w *downstreamSSEKeepaliveWriter) WriteHeader(code int) {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *downstreamSSEKeepaliveWriter) WriteHeaderNow() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *downstreamSSEKeepaliveWriter) Flush() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.Flush()
	}
}

func (w *downstreamSSEKeepaliveWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.ResponseWriter == nil {
		return nil, nil, errors.New("response writer released")
	}
	return w.ResponseWriter.Hijack()
}

func (w *downstreamSSEKeepaliveWriter) CloseNotify() <-chan bool {
	if w.ResponseWriter == nil {
		ch := make(chan bool)
		close(ch)
		return ch
	}
	return w.ResponseWriter.CloseNotify()
}

func (w *downstreamSSEKeepaliveWriter) Pusher() http.Pusher {
	if w.ResponseWriter == nil {
		return nil
	}
	return w.ResponseWriter.Pusher()
}

func (w *downstreamSSEKeepaliveWriter) Status() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Status()
}

func (w *downstreamSSEKeepaliveWriter) Size() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Size()
}

func (w *downstreamSSEKeepaliveWriter) Written() bool {
	if w.k == nil || w.ResponseWriter == nil {
		return false
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Written()
}
