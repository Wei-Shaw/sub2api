package repository

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gatewaydebug"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamDebugTrace struct {
	logger        *gatewaydebug.Logger
	id            uint64
	requestBytes  atomic.Int64
	responseBytes atomic.Int64
}

type upstreamDebugBody struct {
	io.ReadCloser
	onChunk func([]byte)
	onDone  func(string, error)
	once    sync.Once
}

func (s *httpUpstreamService) beginUpstreamDebugTrace(
	req *http.Request,
	proxyURL string,
	accountID int64,
	profile service.HTTPUpstreamProfile,
) *upstreamDebugTrace {
	if s == nil || s.debugLogger == nil || req == nil {
		return nil
	}
	trace := &upstreamDebugTrace{
		logger: s.debugLogger,
		id:     s.debugLogger.NextID(),
	}

	var bodyCopy io.ReadCloser
	if req.Body != nil && req.GetBody != nil {
		bodyCopy, _ = req.GetBody()
	}
	trace.logger.Write(func(writer io.Writer) {
		fmt.Fprintf(writer, "\n========== [%s] UPSTREAM_REQUEST id=%d ==========\n", debugTimestamp(), trace.id)
		fmt.Fprintln(writer, "--- context ---")
		fmt.Fprintf(writer, "  account_id: %d\n", accountID)
		fmt.Fprintf(writer, "  profile: %s\n", profile)
		fmt.Fprintf(writer, "  proxy: %s\n", sanitizeDebugURL(proxyURL))
		fmt.Fprintln(writer, "--- request ---")
		fmt.Fprintf(writer, "  method: %s\n", req.Method)
		fmt.Fprintf(writer, "  url: %s\n", sanitizeDebugURL(req.URL.String()))
		if req.Host != "" {
			fmt.Fprintf(writer, "  host: %s\n", req.Host)
		}
		fmt.Fprintf(writer, "  content_length: %d\n", req.ContentLength)
		writeDebugHeaders(writer, req.Header)
		fmt.Fprintln(writer, "--- body ---")
		switch {
		case req.Body == nil:
			fmt.Fprintln(writer, "  (empty)")
			fmt.Fprintf(writer, "========== UPSTREAM_REQUEST_END id=%d bytes=0 ==========\n", trace.id)
		case bodyCopy != nil:
			written, copyErr := io.Copy(writer, bodyCopy)
			trace.requestBytes.Store(written)
			_ = bodyCopy.Close()
			fmt.Fprintln(writer)
			if copyErr != nil {
				fmt.Fprintf(writer, "========== UPSTREAM_REQUEST_END id=%d bytes=%d error=%q ==========\n", trace.id, written, copyErr.Error())
			} else {
				fmt.Fprintf(writer, "========== UPSTREAM_REQUEST_END id=%d bytes=%d ==========\n", trace.id, written)
			}
		default:
			fmt.Fprintf(writer, "  (streamed below with id=%d)\n", trace.id)
		}
	})

	if req.Body != nil && bodyCopy == nil {
		req.Body = &upstreamDebugBody{
			ReadCloser: req.Body,
			onChunk: func(chunk []byte) {
				trace.logBodyChunk("UPSTREAM_REQUEST_BODY_CHUNK", &trace.requestBytes, chunk)
			},
			onDone: func(reason string, err error) {
				trace.logBodyEnd("UPSTREAM_REQUEST_END", trace.requestBytes.Load(), reason, err)
			},
		}
	}
	return trace
}

func (t *upstreamDebugTrace) logTransportError(err error) {
	if t == nil || t.logger == nil || err == nil {
		return
	}
	t.logger.Write(func(writer io.Writer) {
		fmt.Fprintf(writer, "\n========== [%s] UPSTREAM_TRANSPORT_ERROR id=%d ==========\n", debugTimestamp(), t.id)
		fmt.Fprintf(writer, "  error: %s\n", err.Error())
	})
}

func (t *upstreamDebugTrace) logResponse(resp *http.Response) {
	if t == nil || t.logger == nil || resp == nil {
		return
	}
	t.logger.Write(func(writer io.Writer) {
		fmt.Fprintf(writer, "\n========== [%s] UPSTREAM_RESPONSE id=%d ==========\n", debugTimestamp(), t.id)
		fmt.Fprintln(writer, "--- response ---")
		fmt.Fprintf(writer, "  status: %s\n", resp.Status)
		fmt.Fprintf(writer, "  status_code: %d\n", resp.StatusCode)
		fmt.Fprintf(writer, "  content_length: %d\n", resp.ContentLength)
		writeDebugHeaders(writer, resp.Header)
		fmt.Fprintln(writer, "--- body ---")
		if resp.Body == nil {
			fmt.Fprintln(writer, "  (empty)")
			fmt.Fprintf(writer, "========== UPSTREAM_RESPONSE_END id=%d bytes=0 ==========\n", t.id)
		} else {
			fmt.Fprintf(writer, "  (streamed below with id=%d)\n", t.id)
		}
	})
}

func (t *upstreamDebugTrace) wrapResponseBody(body io.ReadCloser) io.ReadCloser {
	if t == nil || t.logger == nil || body == nil {
		return body
	}
	return &upstreamDebugBody{
		ReadCloser: body,
		onChunk: func(chunk []byte) {
			t.logBodyChunk("UPSTREAM_RESPONSE_BODY_CHUNK", &t.responseBytes, chunk)
		},
		onDone: func(reason string, err error) {
			t.logBodyEnd("UPSTREAM_RESPONSE_END", t.responseBytes.Load(), reason, err)
		},
	}
}

func (t *upstreamDebugTrace) logBodyChunk(tag string, total *atomic.Int64, chunk []byte) {
	if t == nil || t.logger == nil || len(chunk) == 0 {
		return
	}
	total.Add(int64(len(chunk)))
	t.logger.Write(func(writer io.Writer) {
		fmt.Fprintf(writer, "\n---------- [%s] %s id=%d bytes=%d ----------\n", debugTimestamp(), tag, t.id, len(chunk))
		_, _ = writer.Write(chunk)
		fmt.Fprintln(writer)
	})
}

func (t *upstreamDebugTrace) logBodyEnd(tag string, total int64, reason string, err error) {
	if t == nil || t.logger == nil {
		return
	}
	t.logger.Write(func(writer io.Writer) {
		fmt.Fprintf(writer, "========== [%s] %s id=%d bytes=%d reason=%s", debugTimestamp(), tag, t.id, total, reason)
		if err != nil {
			fmt.Fprintf(writer, " error=%q", err.Error())
		}
		fmt.Fprintln(writer, " ==========")
	})
}

func (b *upstreamDebugBody) Read(buffer []byte) (int, error) {
	read, err := b.ReadCloser.Read(buffer)
	if read > 0 && b.onChunk != nil {
		b.onChunk(buffer[:read])
	}
	if err != nil {
		reason := "read_error"
		var doneErr error
		if err == io.EOF {
			reason = "eof"
		} else {
			doneErr = err
		}
		b.finish(reason, doneErr)
	}
	return read, err
}

func (b *upstreamDebugBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish("closed", err)
	return err
}

func (b *upstreamDebugBody) finish(reason string, err error) {
	if b == nil || b.onDone == nil {
		return
	}
	b.once.Do(func() {
		b.onDone(reason, err)
	})
}

func writeDebugHeaders(writer io.Writer, headers http.Header) {
	fmt.Fprintln(writer, "--- headers ---")
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range headers[key] {
			fmt.Fprintf(writer, "  %s: %s\n", key, safeDebugHeaderValue(key, value))
		}
	}
}

func safeDebugHeaderValue(key string, value string) string {
	if !isSensitiveDebugName(key) {
		return strings.TrimSpace(value)
	}
	if strings.EqualFold(strings.TrimSpace(key), "authorization") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "bearer ") {
		return "Bearer [redacted]"
	}
	return "[redacted]"
}

func sanitizeDebugURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "direct"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[invalid-url]"
	}
	if parsed.User != nil {
		parsed.User = url.User("[redacted]")
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveDebugName(key) {
			query.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isSensitiveDebugName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "api-key", "apikey", "key", "x-api-key", "x-goog-api-key", "password", "passwd":
		return true
	}
	return strings.Contains(normalized, "access_token") ||
		strings.Contains(normalized, "access-token") ||
		strings.Contains(normalized, "client_secret") ||
		strings.Contains(normalized, "client-secret") ||
		strings.Contains(normalized, "signature") ||
		strings.HasSuffix(normalized, "_token") ||
		strings.HasSuffix(normalized, "-token") ||
		strings.HasSuffix(normalized, "_key") ||
		strings.HasSuffix(normalized, "-key")
}

func debugTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}
