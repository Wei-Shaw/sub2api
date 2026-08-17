package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"syscall"

	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

const (
	requestBodyErrorStageTransportRead = "transport_read"
	requestBodyErrorStageDecompress    = "decompress"
	requestBodyErrorStageNormalize     = "normalize"

	requestBodyErrorClassBodyTooLarge     = "body_too_large"
	requestBodyErrorClassUnexpectedEOF    = "unexpected_eof"
	requestBodyErrorClassConnectionReset  = "connection_reset"
	requestBodyErrorClassContextCanceled  = "context_canceled"
	requestBodyErrorClassDeadlineExceeded = "deadline_exceeded"
	requestBodyErrorClassUnsupported      = "unsupported_encoding"
	requestBodyErrorClassDecompression    = "decompression_failed"
	requestBodyErrorClassNormalization    = "normalization_failed"
	requestBodyErrorClassReadFailed       = "read_failed"
)

var errUnsupportedContentEncoding = errors.New("unsupported content encoding")

// RequestBodyErrorDiagnostics is the bounded, non-sensitive classification of a
// request body read failure.
type RequestBodyErrorDiagnostics struct {
	Stage string
	Class string
	// BytesRead always counts raw transport bytes consumed before decompression
	// or normalization; it is never the decompressed or normalized length.
	BytesRead int64
}

// RequestBodyErrorDiagnosticsFromError extracts safe diagnostics without
// exposing the wrapped error text. Unwrapped errors use conservative defaults.
func RequestBodyErrorDiagnosticsFromError(err error) RequestBodyErrorDiagnostics {
	defaultDiagnostics := RequestBodyErrorDiagnostics{
		Stage:     requestBodyErrorStageTransportRead,
		Class:     requestBodyErrorClassReadFailed,
		BytesRead: 0,
	}

	var diagnosticErr *requestBodyDiagnosticError
	if errors.As(err, &diagnosticErr) && diagnosticErr != nil {
		return diagnosticErr.diagnostics
	}
	return defaultDiagnostics
}

type requestBodyDiagnosticError struct {
	diagnostics RequestBodyErrorDiagnostics
	err         error
}

func (e *requestBodyDiagnosticError) Error() string {
	if e == nil {
		return "request body operation failed"
	}
	switch e.diagnostics.Stage {
	case requestBodyErrorStageDecompress:
		return "request body decompression failed"
	case requestBodyErrorStageNormalize:
		return "request body normalization failed"
	default:
		return "request body read failed"
	}
}

func (e *requestBodyDiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func newRequestBodyDiagnosticError(stage string, bytesRead int64, err error) error {
	if err == nil {
		return nil
	}
	if bytesRead < 0 {
		bytesRead = 0
	}
	return &requestBodyDiagnosticError{
		diagnostics: RequestBodyErrorDiagnostics{
			Stage:     stage,
			Class:     classifyRequestBodyError(stage, err),
			BytesRead: bytesRead,
		},
		err: err,
	}
}

func classifyRequestBodyError(stage string, err error) string {
	switch stage {
	case requestBodyErrorStageTransportRead:
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return requestBodyErrorClassBodyTooLarge
		case errors.Is(err, context.Canceled):
			return requestBodyErrorClassContextCanceled
		case errors.Is(err, context.DeadlineExceeded):
			return requestBodyErrorClassDeadlineExceeded
		case errors.Is(err, io.ErrUnexpectedEOF):
			return requestBodyErrorClassUnexpectedEOF
		case errors.Is(err, syscall.ECONNRESET):
			return requestBodyErrorClassConnectionReset
		default:
			return requestBodyErrorClassReadFailed
		}
	case requestBodyErrorStageDecompress:
		if errors.Is(err, errUnsupportedContentEncoding) {
			return requestBodyErrorClassUnsupported
		}
		return requestBodyErrorClassDecompression
	case requestBodyErrorStageNormalize:
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return requestBodyErrorClassBodyTooLarge
		}
		return requestBodyErrorClassNormalization
	default:
		return requestBodyErrorClassReadFailed
	}
}

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	body, _, err := readRequestBodyWithPrealloc(req)
	return body, err
}

func readRequestBodyWithPrealloc(req *http.Request) ([]byte, int64, error) {
	if req == nil || req.Body == nil {
		return nil, 0, nil
	}

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	n, err := io.Copy(buf, req.Body)
	if err != nil {
		return nil, n, newRequestBodyDiagnosticError(requestBodyErrorStageTransportRead, n, err)
	}
	raw := buf.Bytes()
	rawBytesRead := int64(len(raw))

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, rawBytesRead, nil
	}

	decoded, err := decompressRequestBody(enc, raw)
	if err != nil {
		return nil, rawBytesRead, newRequestBodyDiagnosticError(requestBodyErrorStageDecompress, rawBytesRead, err)
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	return decoded, rawBytesRead, nil
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	body, rawBytesRead, err := readRequestBodyWithPrealloc(req)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
	if err != nil {
		return nil, newRequestBodyDiagnosticError(requestBodyErrorStageNormalize, rawBytesRead, err)
	}
	return normalized, nil
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return io.ReadAll(io.LimitReader(dec, maxDecompressedBodySize))
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return io.ReadAll(io.LimitReader(gr, maxDecompressedBodySize))
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return io.ReadAll(io.LimitReader(zr, maxDecompressedBodySize))
	default:
		return nil, errUnsupportedContentEncoding
	}
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return body, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	var out []byte
	inString := false
	escaped := false
	for i, b := range body {
		if inString && isJSONControlByte(b) {
			if out == nil {
				capHint := len(body) + 6
				if int64(capHint) > maxNormalizedBytes {
					capHint = int(maxNormalizedBytes)
				}
				out = make([]byte, 0, capHint)
				out = append(out, body[:i]...)
			}
			if int64(len(out)+6) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}

		if out != nil {
			if int64(len(out)+1) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = append(out, b)
		}
	}
	if out != nil {
		return out, nil
	}
	return body, nil
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= jsonUTF8BOMLen && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[jsonUTF8BOMLen:]
	}
	return body
}

func isJSONControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func appendJSONUnicodeEscape(dst []byte, b byte) []byte {
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0x0f])
}
