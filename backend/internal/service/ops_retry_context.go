package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// limitedResponseWriter captures up to `limit` bytes of the HTTP response
// body for ops retry preview, while pretending all writes succeed so
// upstream/client code never sees a write error.
type limitedResponseWriter struct {
	header      http.Header
	wroteHeader bool

	limit        int
	totalWritten int64
	buf          bytes.Buffer
}

func newLimitedResponseWriter(limit int) *limitedResponseWriter {
	if limit <= 0 {
		limit = 1
	}
	return &limitedResponseWriter{
		header: make(http.Header),
		limit:  limit,
	}
}

func (w *limitedResponseWriter) Header() http.Header {
	return w.header
}

func (w *limitedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
}

func (w *limitedResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.totalWritten += int64(len(p))

	if w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(p) > remaining {
			_, _ = w.buf.Write(p[:remaining])
		} else {
			_, _ = w.buf.Write(p)
		}
	}

	// Pretend we wrote everything to avoid upstream/client code treating it as an error.
	return len(p), nil
}

func (w *limitedResponseWriter) Flush() {}

func (w *limitedResponseWriter) bodyBytes() []byte {
	return w.buf.Bytes()
}

func (w *limitedResponseWriter) truncated() bool {
	return w.totalWritten > int64(w.limit)
}

// newOpsRetryContext builds a fake gin.Context backed by a limitedResponseWriter
// for capturing the retry response. The context includes whitelisted headers
// from the original error log.
func newOpsRetryContext(ctx context.Context, errorLog *OpsErrorLogDetail) (*gin.Context, *limitedResponseWriter) {
	w := newLimitedResponseWriter(opsRetryCaptureBytesLimit)
	c, _ := gin.CreateTestContext(w)

	path := "/"
	if errorLog != nil && strings.TrimSpace(errorLog.RequestPath) != "" {
		path = errorLog.RequestPath
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+path, bytes.NewReader(nil))
	req.Header.Set("content-type", "application/json")
	if errorLog != nil && strings.TrimSpace(errorLog.UserAgent) != "" {
		req.Header.Set("user-agent", errorLog.UserAgent)
	}
	restoreAllowlistedHeaders(req, errorLog)

	c.Request = req
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return c, w
}

// restoreAllowlistedHeaders replays a minimal, whitelisted subset of request
// headers from the original error log (e.g. anthropic-beta / anthropic-version).
// Auth credentials are never replayed.
func restoreAllowlistedHeaders(req *http.Request, errorLog *OpsErrorLogDetail) {
	if errorLog == nil || strings.TrimSpace(errorLog.RequestHeaders) == "" {
		return
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(errorLog.RequestHeaders), &stored); err != nil {
		return
	}
	for k, v := range stored {
		key := strings.TrimSpace(k)
		if key == "" || !opsRetryRequestHeaderAllowlist[strings.ToLower(key)] {
			continue
		}
		if val := strings.TrimSpace(v); val != "" {
			req.Header.Set(key, val)
		}
	}
}

func extractUpstreamRequestID(c *gin.Context) string {
	if c == nil || c.Writer == nil {
		return ""
	}
	h := c.Writer.Header()
	if h == nil {
		return ""
	}
	for _, key := range []string{"x-request-id", "X-Request-Id", "X-Request-ID"} {
		if v := strings.TrimSpace(h.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func extractResponsePreview(w *limitedResponseWriter) (preview string, truncated bool) {
	if w == nil {
		return "", false
	}
	b := bytes.TrimSpace(w.bodyBytes())
	if len(b) == 0 {
		return "", w.truncated()
	}
	if len(b) > opsRetryResponsePreviewMax {
		return string(b[:opsRetryResponsePreviewMax]), true
	}
	return string(b), w.truncated()
}
