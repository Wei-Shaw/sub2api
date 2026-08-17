package handler

import (
	"net/http"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"go.uber.org/zap"
)

const requestBodyBytesReadKindRawTransport = "raw_transport"

type requestBodyReadLogContext struct {
	contentLength    int64
	contentEncoding  string
	transferEncoding string
}

func newRequestBodyReadLogContext(req *http.Request) requestBodyReadLogContext {
	if req == nil {
		return requestBodyReadLogContext{
			contentLength:    -1,
			contentEncoding:  "identity",
			transferEncoding: "unknown",
		}
	}
	return requestBodyReadLogContext{
		contentLength:    req.ContentLength,
		contentEncoding:  normalizeRequestBodyContentEncoding(req.Header.Get("Content-Encoding")),
		transferEncoding: normalizeRequestTransferEncoding(req.TransferEncoding, req.ContentLength),
	}
}

func normalizeRequestBodyContentEncoding(value string) string {
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

func normalizeRequestTransferEncoding(values []string, contentLength int64) string {
	if len(values) > 0 {
		if len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), "chunked") {
			return "chunked"
		}
		return "other"
	}
	if contentLength >= 0 {
		return "content_length"
	}
	return "unknown"
}

// logRequestBodyReadFailure records only bounded classifications and request
// framing metadata. It deliberately omits body bytes, raw header values, and
// underlying error text; requestLogger already carries the existing request
// identity context.
func logRequestBodyReadFailure(reqLog *zap.Logger, metadata requestBodyReadLogContext, err error, startedAt time.Time) {
	if reqLog == nil {
		return
	}
	diagnostics := pkghttputil.RequestBodyErrorDiagnosticsFromError(err)
	elapsedMs := time.Since(startedAt).Milliseconds()
	if elapsedMs < 0 {
		elapsedMs = 0
	}

	reqLog.Warn("request body read failed",
		zap.String("error_stage", diagnostics.Stage),
		zap.String("error_class", diagnostics.Class),
		zap.Int64("bytes_read", diagnostics.BytesRead),
		zap.String("bytes_read_kind", requestBodyBytesReadKindRawTransport),
		zap.Int64("content_length", metadata.contentLength),
		zap.String("content_encoding", metadata.contentEncoding),
		zap.String("transfer_encoding", metadata.transferEncoding),
		zap.Int64("elapsed_ms", elapsedMs),
	)
}
