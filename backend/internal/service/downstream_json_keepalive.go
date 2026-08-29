package service

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var downstreamJSONKeepalivePayload = []byte(" \n")

// downstreamJSONKeepalive writes leading JSON whitespace while a non-streaming
// request is waiting. Leading whitespace keeps the eventual response a single
// valid JSON document, at the cost of committing HTTP 200 on the first beat.
type downstreamJSONKeepalive struct {
	mu      sync.Mutex
	writer  gin.ResponseWriter
	wrapper *downstreamJSONKeepaliveWriter
	started bool
	stopped bool
	bytes   int
	stop    chan struct{}
}

func startDownstreamJSONKeepalive(c *gin.Context, key string, interval time.Duration) func() {
	if c == nil || c.Writer == nil || interval <= 0 || key == "" {
		return func() {}
	}
	if existing := downstreamJSONKeepaliveForKey(c, key); existing != nil {
		return func() { stopDownstreamJSONKeepalive(c, key) }
	}

	originalWriter := c.Writer
	k := &downstreamJSONKeepalive{writer: originalWriter, stop: make(chan struct{})}
	wrappedWriter := &downstreamJSONKeepaliveWriter{ResponseWriter: originalWriter, k: k}
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

	return func() { stopDownstreamJSONKeepalive(c, key) }
}

func (k *downstreamJSONKeepalive) beat() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stopped || k.writer == nil {
		return false
	}
	if !k.started {
		header := k.writer.Header()
		header.Set("Content-Type", "application/json; charset=utf-8")
		header.Set("Cache-Control", "no-cache")
		header.Set("X-Accel-Buffering", "no")
		k.writer.WriteHeader(http.StatusOK)
		k.started = true
	}
	n, err := k.writer.Write(downstreamJSONKeepalivePayload)
	k.bytes += n
	if err != nil {
		k.markStoppedLocked()
		return false
	}
	k.writer.Flush()
	return true
}

func (k *downstreamJSONKeepalive) Stop() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.markStoppedLocked()
	k.mu.Unlock()
}

func (k *downstreamJSONKeepalive) markStoppedLocked() {
	if k.stopped {
		return
	}
	k.stopped = true
	close(k.stop)
}

func downstreamJSONKeepaliveForKey(c *gin.Context, key string) *downstreamJSONKeepalive {
	if c == nil {
		return nil
	}
	value, ok := c.Get(key)
	if !ok {
		return nil
	}
	k, _ := value.(*downstreamJSONKeepalive)
	return k
}

func stopDownstreamJSONKeepalive(c *gin.Context, key string) bool {
	k := downstreamJSONKeepaliveForKey(c, key)
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

func downstreamJSONKeepalivePresent(c *gin.Context, key string) bool {
	return downstreamJSONKeepaliveForKey(c, key) != nil
}

func downstreamJSONKeepaliveCommitted(c *gin.Context, key string) bool {
	k := downstreamJSONKeepaliveForKey(c, key)
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.started
}

func downstreamJSONKeepaliveBytes(c *gin.Context, key string) int {
	k := downstreamJSONKeepaliveForKey(c, key)
	if k == nil {
		return 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.bytes
}

func downstreamJSONKeepaliveAdjustedWrittenSize(c *gin.Context, key string) int {
	if c == nil || c.Writer == nil {
		return -1
	}
	k := downstreamJSONKeepaliveForKey(c, key)
	if k == nil {
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

type downstreamJSONKeepaliveWriter struct {
	gin.ResponseWriter
	k *downstreamJSONKeepalive
}

func (w *downstreamJSONKeepaliveWriter) suspend() {
	if w.k != nil {
		w.k.Stop()
	}
}

func (w *downstreamJSONKeepaliveWriter) Header() http.Header {
	w.suspend()
	if w.ResponseWriter == nil {
		return http.Header{}
	}
	return w.ResponseWriter.Header()
}

func (w *downstreamJSONKeepaliveWriter) Write(data []byte) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *downstreamJSONKeepaliveWriter) WriteString(s string) (int, error) {
	w.suspend()
	if w.ResponseWriter == nil {
		return 0, nil
	}
	return w.ResponseWriter.WriteString(s)
}

func (w *downstreamJSONKeepaliveWriter) WriteHeader(code int) {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *downstreamJSONKeepaliveWriter) WriteHeaderNow() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.WriteHeaderNow()
	}
}

func (w *downstreamJSONKeepaliveWriter) Flush() {
	w.suspend()
	if w.ResponseWriter != nil {
		w.ResponseWriter.Flush()
	}
}

func (w *downstreamJSONKeepaliveWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.ResponseWriter == nil {
		return nil, nil, errors.New("response writer released")
	}
	return w.ResponseWriter.Hijack()
}

func (w *downstreamJSONKeepaliveWriter) CloseNotify() <-chan bool {
	if w.ResponseWriter == nil {
		ch := make(chan bool)
		close(ch)
		return ch
	}
	return w.ResponseWriter.CloseNotify()
}

func (w *downstreamJSONKeepaliveWriter) Pusher() http.Pusher {
	if w.ResponseWriter == nil {
		return nil
	}
	return w.ResponseWriter.Pusher()
}

func (w *downstreamJSONKeepaliveWriter) Status() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Status()
}

func (w *downstreamJSONKeepaliveWriter) Size() int {
	if w.k == nil || w.ResponseWriter == nil {
		return 0
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Size()
}

func (w *downstreamJSONKeepaliveWriter) Written() bool {
	if w.k == nil || w.ResponseWriter == nil {
		return false
	}
	w.k.mu.Lock()
	defer w.k.mu.Unlock()
	return w.ResponseWriter.Written()
}
