package middleware

import (
	"bufio"
	"net"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/traffic"
	"github.com/gin-gonic/gin"
)

// TrafficCapture must precede ResponseCompress: its writer observes encoded
// entity bytes, and its defer runs after the compressor has emitted its trailer.
// No body contents are retained. Hijacked/WebSocket messages are out of scope.
func TrafficCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}
		collector := traffic.NewCollector()
		c.Request = c.Request.WithContext(traffic.WithCollector(c.Request.Context(), collector))
		compressed := traffic.IsCompressed(traffic.ContentEncoding(c.Request.Header))
		request := collector.Begin(traffic.DownstreamRequest, compressed)
		if c.Request.Body == nil || c.Request.Body == http.NoBody {
			request.CompleteActual()
			if !compressed {
				request.CompleteOriginal()
			}
		} else {
			expected := c.Request.ContentLength
			if expected == 0 {
				expected = -1
			} // non-nil Body with unknown length
			c.Request.Body = traffic.ObserveBody(c.Request.Body, request, !compressed, expected)
		}
		response := collector.Begin(traffic.DownstreamResponse, false)
		writer := &trafficResponseWriter{ResponseWriter: c.Writer, counter: response, collector: collector, head: c.Request.Method == http.MethodHead}
		c.Writer = writer
		returned := false
		defer func() {
			c.Writer = writer.ResponseWriter
			if !returned {
				// Outer Recovery can still write after this defer. Do not freeze a
				// misleading partial/zero response as the complete error body.
				response.FailActual()
				response.FailOriginal()
			}
			response.CompleteActual()
			if writer.noBody() || !traffic.IsCompressed(traffic.ContentEncoding(writer.Header())) {
				response.CompleteOriginal()
			} else {
				response.SetCompressed()
			}
			if writer.Status() == http.StatusSwitchingProtocols {
				collector.Exclude()
			}
			collector.Finalize()
		}()
		c.Next()
		returned = true
	}
}

type trafficResponseWriter struct {
	gin.ResponseWriter
	counter   *traffic.Counter
	collector *traffic.Collector
	head      bool
}

func (w *trafficResponseWriter) noBody() bool {
	status := w.Status()
	return w.head || status == http.StatusNoContent || status == http.StatusNotModified || (status >= 100 && status < 200)
}

func (w *trafficResponseWriter) record(n int, err error) {
	if w.noBody() {
		return
	}
	w.counter.AddActual(n)
	compressed := traffic.IsCompressed(traffic.ContentEncoding(w.Header()))
	if compressed {
		w.counter.SetCompressed()
	} else {
		w.counter.AddOriginal(n)
	}
	if err != nil {
		w.counter.FailActual()
		if !compressed {
			w.counter.FailOriginal()
		}
	}
}

func (w *trafficResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.record(n, err)
	return n, err
}
func (w *trafficResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.record(n, err)
	return n, err
}
func (w *trafficResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.Hijack()
	if err == nil {
		w.collector.Exclude()
	}
	return conn, rw, err
}
