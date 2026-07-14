package middleware

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Align with deploy/Caddyfile encode { gzip 6; minimum_length 256 }.
const (
	gzipCompressionLevel = 6
	gzipMinLength        = 256
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzipCompressionLevel)
		return w
	},
}

// Gzip compresses compressible HTTP responses for direct Go/Docker deployments.
// Gateway/streaming paths are skipped so SSE and websocket upgrades are not buffered.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldOfferGzip(c.Request) {
			c.Next()
			return
		}

		w := &lazyGzipResponseWriter{
			ResponseWriter: c.Writer,
			minLength:      gzipMinLength,
		}
		c.Writer = w
		defer w.finish()

		c.Next()
	}
}

func shouldOfferGzip(req *http.Request) bool {
	if req == nil {
		return false
	}
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}
	if strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") {
		return false
	}
	return !isGzipExcludedPath(req.URL.Path)
}

// isGzipExcludedPath skips API gateway / streaming surfaces.
// Kept in sync with embedded-frontend bypass prefixes that serve live upstream traffic.
func isGzipExcludedPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	switch {
	case trimmed == "/v1", strings.HasPrefix(trimmed, "/v1/"),
		trimmed == "/v1beta", strings.HasPrefix(trimmed, "/v1beta/"),
		strings.HasPrefix(trimmed, "/backend-api/"),
		strings.HasPrefix(trimmed, "/antigravity/"),
		trimmed == "/responses", strings.HasPrefix(trimmed, "/responses/"),
		trimmed == "/alpha/search", strings.HasPrefix(trimmed, "/alpha/search/"),
		strings.HasPrefix(trimmed, "/images/"),
		strings.HasPrefix(trimmed, "/videos/"):
		return true
	default:
		return false
	}
}

func isCompressibleContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return false
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "text/event-stream" {
		return false
	}
	switch {
	case strings.HasPrefix(ct, "text/"),
		strings.HasPrefix(ct, "application/json"),
		strings.HasPrefix(ct, "application/javascript"),
		strings.HasPrefix(ct, "application/xml"),
		strings.HasPrefix(ct, "application/rss+xml"),
		ct == "image/svg+xml",
		ct == "application/xhtml+xml",
		ct == "application/wasm":
		return true
	default:
		return false
	}
}

type lazyGzipResponseWriter struct {
	gin.ResponseWriter
	minLength int
	buf       bytes.Buffer
	gz        *gzip.Writer
	started   bool
	skipped   bool
	status    int
}

func (w *lazyGzipResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	// Delay header write until we know whether to compress.
	if w.started || w.skipped {
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *lazyGzipResponseWriter) Write(data []byte) (int, error) {
	if w.skipped {
		return w.ResponseWriter.Write(data)
	}
	if w.started {
		return w.gz.Write(data)
	}

	if w.status == 0 {
		w.status = http.StatusOK
	}

	// Never compress error bodies that handlers may stream partially.
	if w.status >= http.StatusBadRequest {
		return w.passthrough(data)
	}

	header := w.Header()
	if enc := header.Get("Content-Encoding"); enc != "" {
		return w.passthrough(data)
	}
	if !isCompressibleContentType(header.Get("Content-Type")) {
		return w.passthrough(data)
	}

	if _, err := w.buf.Write(data); err != nil {
		return 0, err
	}
	if w.buf.Len() < w.minLength {
		return len(data), nil
	}
	if err := w.startGzip(); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *lazyGzipResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *lazyGzipResponseWriter) Flush() {
	if w.skipped {
		w.ResponseWriter.Flush()
		return
	}
	if !w.started {
		// Streaming flush before we reached min length — avoid buffering forever.
		_, _ = w.passthrough(nil)
		w.ResponseWriter.Flush()
		return
	}
	_ = w.gz.Flush()
	w.ResponseWriter.Flush()
}

func (w *lazyGzipResponseWriter) passthrough(data []byte) (int, error) {
	w.skipped = true
	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
	n := 0
	if w.buf.Len() > 0 {
		written, err := w.ResponseWriter.Write(w.buf.Bytes())
		n += written
		w.buf.Reset()
		if err != nil {
			return n, err
		}
	}
	if len(data) == 0 {
		return n, nil
	}
	written, err := w.ResponseWriter.Write(data)
	return n + written, err
}

func (w *lazyGzipResponseWriter) startGzip() error {
	w.started = true
	header := w.Header()
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	header.Add("Vary", "Accept-Encoding")

	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}

	gz, ok := gzipWriterPool.Get().(*gzip.Writer)
	if !ok || gz == nil {
		return errors.New("gzip writer pool returned unexpected value")
	}
	gz.Reset(w.ResponseWriter)
	w.gz = gz

	if w.buf.Len() > 0 {
		if _, err := w.gz.Write(w.buf.Bytes()); err != nil {
			return err
		}
		w.buf.Reset()
	}
	return nil
}

func (w *lazyGzipResponseWriter) finish() {
	if w.skipped {
		return
	}
	if !w.started {
		if w.buf.Len() == 0 {
			if w.status != 0 && !w.Written() {
				w.ResponseWriter.WriteHeader(w.status)
			}
			return
		}
		// Below minimum length: write uncompressed.
		_, _ = w.passthrough(nil)
		return
	}
	err := w.gz.Close()
	if err != nil {
		// Best-effort: response may already be partially written.
		_ = err
	}
	gzipWriterPool.Put(w.gz)
	w.gz = nil
}
