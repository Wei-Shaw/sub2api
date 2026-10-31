package httputil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Stable, payload-free labels for request body read failures.
const (
	RequestBodyReadErrorMaxBytes                   = "max_bytes"
	RequestBodyReadErrorTimeout                    = "read_timeout"
	RequestBodyReadErrorClientDisconnect           = "client_disconnect"
	RequestBodyReadErrorTruncated                  = "truncated_body"
	RequestBodyReadErrorDecodeContentEncoding      = "decode_content_encoding"
	RequestBodyReadErrorUnsupportedContentEncoding = "unsupported_content_encoding"
	RequestBodyReadErrorTransport                  = "transport"
	RequestBodyReadErrorIO                         = "io_read"
)

// StatusClientClosedRequest is the non-standard status recorded when the client
// goes away before the server responds.
const StatusClientClosedRequest = 499

// RequestBodyReadError annotates a failed request body read with progress
// counters. It never carries request content.
type RequestBodyReadError struct {
	Err       error
	BytesRead int64
	Elapsed   time.Duration
}

func (e *RequestBodyReadError) Error() string { return e.Err.Error() }

func (e *RequestBodyReadError) Unwrap() error { return e.Err }

// RequestBodyReadFailure is the response and the payload-free diagnostics for a
// failed request body read.
type RequestBodyReadFailure struct {
	Kind    string
	Status  int
	Message string
	// MaxBytesLimit is set when Kind is RequestBodyReadErrorMaxBytes.
	MaxBytesLimit int64

	contentLength   int64
	contentEncoding string
	protocol        string
	chunked         bool
	bytesRead       int64
	elapsed         time.Duration
}

// DescribeRequestBodyReadFailure classifies a request body read error. Timeouts
// and client disconnects get their own status codes so they are not reported
// as malformed requests; other failures keep the generic 400 response.
func DescribeRequestBodyReadFailure(req *http.Request, err error) RequestBodyReadFailure {
	failure := RequestBodyReadFailure{
		Kind:            classifyRequestBodyReadError(req, err),
		contentLength:   -1,
		contentEncoding: "identity",
		bytesRead:       -1,
	}
	if req != nil {
		failure.contentLength = req.ContentLength
		failure.contentEncoding = RequestContentEncodingCategory(req.Header.Get("Content-Encoding"))
		failure.protocol = req.Proto
		failure.chunked = len(req.TransferEncoding) > 0
	}
	var readErr *RequestBodyReadError
	if errors.As(err, &readErr) {
		failure.bytesRead = readErr.BytesRead
		failure.elapsed = readErr.Elapsed
	}

	switch failure.Kind {
	case RequestBodyReadErrorMaxBytes:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			failure.MaxBytesLimit = maxErr.Limit
		}
		failure.Status = http.StatusRequestEntityTooLarge
		failure.Message = "Request body is too large"
	case RequestBodyReadErrorTimeout:
		failure.Status = http.StatusRequestTimeout
		failure.Message = "Timed out reading request body"
	case RequestBodyReadErrorClientDisconnect:
		failure.Status = StatusClientClosedRequest
		failure.Message = "Client closed the connection before the request body was received"
	default:
		failure.Status = http.StatusBadRequest
		failure.Message = "Failed to read request body"
	}
	return failure
}

// LogFields returns diagnostics that never include request content or error
// text, which may embed peer addresses or decoder input.
func (f RequestBodyReadFailure) LogFields() []zap.Field {
	fields := []zap.Field{
		zap.String("error_kind", f.Kind),
		zap.Int("status", f.Status),
		zap.String("protocol", f.protocol),
		zap.String("content_encoding", f.contentEncoding),
		zap.Int64("content_length", f.contentLength),
		zap.Bool("chunked", f.chunked),
	}
	if f.bytesRead >= 0 {
		fields = append(fields,
			zap.Int64("bytes_read", f.bytesRead),
			zap.Int64("read_ms", f.elapsed.Milliseconds()),
		)
	}
	return fields
}

func classifyRequestBodyReadError(req *http.Request, err error) string {
	if err == nil {
		return RequestBodyReadErrorIO
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return RequestBodyReadErrorMaxBytes
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "decode content-encoding") {
		if strings.Contains(lower, "unsupported content-encoding") {
			return RequestBodyReadErrorUnsupportedContentEncoding
		}
		return RequestBodyReadErrorDecodeContentEncoding
	}
	// Check timeouts first: on HTTP/1 a read deadline also cancels the request
	// context, which would otherwise look like a client disconnect.
	var netErr net.Error
	if errors.Is(err, os.ErrDeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return RequestBodyReadErrorTimeout
	}
	if requestBodyClientGone(req, err, lower) {
		return RequestBodyReadErrorClientDisconnect
	}
	// HTTP/2 reports an END_STREAM that arrives before Content-Length bytes
	// with "request declared a Content-Length of N but only wrote M bytes".
	if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(lower, "request declared a content-length") {
		return RequestBodyReadErrorTruncated
	}
	if errors.As(err, &netErr) {
		return RequestBodyReadErrorTransport
	}
	return RequestBodyReadErrorIO
}

func requestBodyClientGone(req *http.Request, err error, lowerErr string) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	// HTTP/2 reports a closed client connection with this message.
	if strings.Contains(lowerErr, "client disconnected") {
		return true
	}
	// net/http cancels the request context when the connection fails, so an
	// unexpected EOF with a canceled context is a disconnect, not a short body.
	return req != nil && errors.Is(req.Context().Err(), context.Canceled)
}

// RequestContentEncodingCategory maps Content-Encoding to a bounded label.
func RequestContentEncodingCategory(value string) string {
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
