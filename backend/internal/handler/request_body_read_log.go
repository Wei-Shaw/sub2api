package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	requestBodyDiagnosticContextKey = "ops_request_body_diagnostic"
	requestBodyFailureWindow        = 5 * time.Minute
	requestBodyFailureMaxSubjects   = 4096
)

type requestBodyDiagnostic struct {
	Kind             pkghttputil.RequestBodyErrorKind
	Encoding         string
	BytesRead        int64
	ContentLength    int64
	TransferEncoding string
	WindowCount      int
	WindowScore      int
}

type requestBodyFailureWindowEntry struct {
	startedAt time.Time
	lastSeen  time.Time
	count     int
	score     int
	level     int
}

type requestBodyFailureCounter struct {
	mu      sync.Mutex
	entries map[string]requestBodyFailureWindowEntry
}

var bodyFailureCounter = requestBodyFailureCounter{
	entries: make(map[string]requestBodyFailureWindowEntry),
}

func recordRequestBodyReadFailure(c *gin.Context, reqLog *zap.Logger, err error) {
	diagnostic := pkghttputil.RequestBodyDiagnostics(err)
	entry := requestBodyDiagnostic{
		Kind:          diagnostic.Kind,
		Encoding:      diagnostic.Encoding,
		BytesRead:     diagnostic.BytesRead,
		ContentLength: diagnostic.ContentLength,
	}
	if c != nil && c.Request != nil {
		entry.TransferEncoding = strings.Join(c.Request.TransferEncoding, ",")
	}

	subjectKey := requestBodyFailureSubjectKey(c)
	var level int
	entry.WindowCount, entry.WindowScore, level = bodyFailureCounter.record(
		subjectKey,
		requestBodyFailureWeight(entry.Kind),
		time.Now(),
	)
	if c != nil {
		c.Set(requestBodyDiagnosticContextKey, entry)
	}

	if reqLog == nil {
		return
	}
	fields := requestBodyDiagnosticFields(entry)
	fields = append(fields, zap.Error(err))
	reqLog.Warn("read request body failed", fields...)
	if level > 0 {
		reqLog.Warn("repeated request body failures",
			zap.String("severity", "P3"),
			zap.Int("window_minutes", int(requestBodyFailureWindow/time.Minute)),
			zap.Int("window_count", entry.WindowCount),
			zap.Int("window_score", entry.WindowScore),
			zap.String("subject", subjectKey),
		)
	}
}

func requestBodyDiagnosticFields(d requestBodyDiagnostic) []zap.Field {
	return []zap.Field{
		zap.String("body_error_kind", string(d.Kind)),
		zap.String("content_encoding", normalizedDiagnosticValue(d.Encoding, "identity")),
		zap.Int64("content_length", d.ContentLength),
		zap.Int64("bytes_read", d.BytesRead),
		zap.String("transfer_encoding", normalizedDiagnosticValue(d.TransferEncoding, "identity")),
		zap.Int("window_count", d.WindowCount),
		zap.Int("window_score", d.WindowScore),
	}
}

func requestBodyFailureSubjectKey(c *gin.Context) string {
	if c == nil {
		return "unknown"
	}
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		return fmt.Sprintf("user:%d", subject.UserID)
	}
	if apiKey, ok := middleware2.GetAPIKeyFromContext(c); ok && apiKey != nil && apiKey.ID > 0 {
		return fmt.Sprintf("api_key:%d", apiKey.ID)
	}
	return "unknown"
}

func requestBodyFailureWeight(kind pkghttputil.RequestBodyErrorKind) int {
	switch kind {
	case pkghttputil.RequestBodyErrorReadCanceled:
		return 0
	case pkghttputil.RequestBodyErrorUnsupportedEncoding,
		pkghttputil.RequestBodyErrorInvalidCompression:
		return 3
	case pkghttputil.RequestBodyErrorTooLarge,
		pkghttputil.RequestBodyErrorDecompressedTooLarge:
		return 5
	default:
		return 1
	}
}

func requestBodyFailureLevel(count, score int) int {
	switch {
	case count >= 60 || score >= 60:
		return 3
	case count >= 30 || score >= 30:
		return 2
	case count >= 10 || score >= 10:
		return 1
	default:
		return 0
	}
}

func (c *requestBodyFailureCounter) record(subject string, weight int, now time.Time) (count, score, alertLevel int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[subject]
	if !exists || now.Sub(entry.startedAt) >= requestBodyFailureWindow {
		entry = requestBodyFailureWindowEntry{startedAt: now}
	}
	entry.lastSeen = now
	entry.count++
	entry.score += weight

	level := requestBodyFailureLevel(entry.count, entry.score)
	if level > entry.level {
		alertLevel = level
		entry.level = level
	}
	c.entries[subject] = entry

	if len(c.entries) > requestBodyFailureMaxSubjects {
		c.removeExpired(now)
		if len(c.entries) > requestBodyFailureMaxSubjects {
			c.removeOldest()
		}
	}
	return entry.count, entry.score, alertLevel
}

func (c *requestBodyFailureCounter) removeExpired(now time.Time) {
	for subject, entry := range c.entries {
		if now.Sub(entry.lastSeen) >= requestBodyFailureWindow {
			delete(c.entries, subject)
		}
	}
}

func (c *requestBodyFailureCounter) removeOldest() {
	oldestSubject := ""
	var oldest time.Time
	for subject, entry := range c.entries {
		if oldestSubject == "" || entry.lastSeen.Before(oldest) {
			oldestSubject = subject
			oldest = entry.lastSeen
		}
	}
	if oldestSubject != "" {
		delete(c.entries, oldestSubject)
	}
}

func requestBodyDiagnosticFromContext(c *gin.Context) (requestBodyDiagnostic, bool) {
	if c == nil {
		return requestBodyDiagnostic{}, false
	}
	value, exists := c.Get(requestBodyDiagnosticContextKey)
	if !exists {
		return requestBodyDiagnostic{}, false
	}
	diagnostic, ok := value.(requestBodyDiagnostic)
	return diagnostic, ok
}

func formatRequestBodyDiagnostic(d requestBodyDiagnostic) string {
	return fmt.Sprintf(
		"request_body_error=%s, content_encoding=%s, content_length=%d, bytes_read=%d, transfer_encoding=%s, failures_5m=%d, risk_score_5m=%d",
		d.Kind,
		normalizedDiagnosticValue(d.Encoding, "identity"),
		d.ContentLength,
		d.BytesRead,
		normalizedDiagnosticValue(d.TransferEncoding, "identity"),
		d.WindowCount,
		d.WindowScore,
	)
}

func normalizedDiagnosticValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

// logRequestBodyReadFailure records a bounded, payload-free reason for a body
// read failure. Clients continue to receive the stable generic error message;
// operators get enough information to distinguish compression failures from a
// disconnected/truncated upload without logging request content.
func logRequestBodyReadFailure(reqLog *zap.Logger, req *http.Request, err error) {
	if reqLog == nil || err == nil {
		return
	}

	contentLength := int64(-1)
	contentEncoding := "identity"
	if req != nil {
		contentLength = req.ContentLength
		contentEncoding = requestContentEncodingCategory(req.Header.Get("Content-Encoding"))
	}

	reqLog.Warn("read request body failed",
		zap.String("error_kind", requestBodyReadErrorKind(err)),
		zap.String("content_encoding", contentEncoding),
		zap.Int64("content_length", contentLength),
	)
}

func requestContentEncodingCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "identity":
		return "identity"
	case "gzip", "x-gzip":
		return "gzip"
	case "zstd":
		return "zstd"
	case "deflate":
		return "deflate"
	default:
		return "other"
	}
}

func requestBodyReadErrorKind(err error) string {
	if err == nil {
		return "none"
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return "max_bytes"
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "decode content-encoding") {
		if strings.Contains(lower, "unsupported content-encoding") {
			return "unsupported_content_encoding"
		}
		return "decode_content_encoding"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return "client_disconnect"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "truncated_body"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "transport"
	}
	return "io_read"
}
