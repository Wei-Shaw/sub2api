package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// RequestBodyErrorKind is a stable, non-sensitive classification for failures
// that happen before a JSON request body can be parsed.
type RequestBodyErrorKind string

const (
	RequestBodyErrorReadCanceled         RequestBodyErrorKind = "read_canceled"
	RequestBodyErrorUnexpectedEOF        RequestBodyErrorKind = "unexpected_eof"
	RequestBodyErrorReadFailed           RequestBodyErrorKind = "read_failed"
	RequestBodyErrorTooLarge             RequestBodyErrorKind = "request_body_too_large"
	RequestBodyErrorUnsupportedEncoding  RequestBodyErrorKind = "unsupported_content_encoding"
	RequestBodyErrorInvalidCompression   RequestBodyErrorKind = "invalid_compressed_body"
	RequestBodyErrorDecompressedTooLarge RequestBodyErrorKind = "decompressed_body_too_large"
)

// RequestBodyError carries diagnostics that are safe to log and persist. It
// deliberately never contains request body bytes.
type RequestBodyError struct {
	Kind          RequestBodyErrorKind
	Encoding      string
	BytesRead     int64
	ContentLength int64
	Err           error
}

func (e *RequestBodyError) Error() string {
	if e == nil {
		return "request body read failed"
	}
	if e.Err == nil {
		return fmt.Sprintf("request body read failed: %s", e.Kind)
	}
	return fmt.Sprintf("request body read failed: %s: %v", e.Kind, e.Err)
}

func (e *RequestBodyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RequestBodyDiagnostics extracts the stable diagnostics from err. Unknown
// errors are classified conservatively as read_failed.
func RequestBodyDiagnostics(err error) RequestBodyError {
	var bodyErr *RequestBodyError
	if errors.As(err, &bodyErr) && bodyErr != nil {
		return *bodyErr
	}
	return RequestBodyError{Kind: classifyRequestBodyReadError(err), Err: err}
}

// PrereadBody 回填已读取完成的请求体：作为 io.ReadCloser 可被再次顺序消费
// （multipart 流式解析），同时暴露 Bytes() 让 ReadRequestBodyWithPrealloc
// 直接返回原始切片，避免二次分配与复制。
//
// 注意：ReadRequestBodyWithPrealloc 对 PrereadBody 的快速路径不检查内部
// reader 是否已被（部分）消费——包装的字节完整且不可变，即使 reader 已被
// 流式消费过，Bytes() 也始终返回完整请求体。
type PrereadBody struct {
	body   []byte
	reader *bytes.Reader
}

// NewPrereadBody 包装一段已读取的请求体。
func NewPrereadBody(body []byte) *PrereadBody {
	return &PrereadBody{body: body, reader: bytes.NewReader(body)}
}

// Read 实现 io.Reader（转发给内部 bytes.Reader）。
func (p *PrereadBody) Read(b []byte) (int, error) {
	if p == nil {
		return 0, io.EOF
	}
	return p.reader.Read(b)
}

// Close 实现 io.Closer；请求体已在内存中，无需释放资源。
func (p *PrereadBody) Close() error { return nil }

// Bytes 返回完整的原始请求体切片。
func (p *PrereadBody) Bytes() []byte {
	if p == nil {
		return nil
	}
	return p.body
}

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
// 已由 PrereadBody 回填的请求体直接返回其完整切片（零拷贝），不检查内部
// reader 是否已被消费——见 PrereadBody 的文档说明。
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	if preread, ok := req.Body.(*PrereadBody); ok {
		return preread.Bytes(), nil
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

	originalContentLength := req.ContentLength
	encoding := normalizeContentEncoding(req.Header.Get("Content-Encoding"))
	raw, bytesRead, err := readRequestBodyChunks(req.Body, capHint, originalContentLength)
	if err != nil {
		return nil, &RequestBodyError{Kind: classifyRequestBodyReadError(err), Encoding: encoding, BytesRead: bytesRead, ContentLength: originalContentLength, Err: err}
	}
	if bytesRead > 0 && originalContentLength > bytesRead && originalContentLength <= int64(requestBodyReadMaxInitCap) {
		return nil, &RequestBodyError{Kind: RequestBodyErrorUnexpectedEOF, Encoding: encoding, BytesRead: bytesRead, ContentLength: originalContentLength, Err: io.ErrUnexpectedEOF}
	}

	if encoding == "" || encoding == "identity" {
		return raw, nil
	}

	decoded, err := decompressRequestBody(encoding, raw)
	if err != nil {
		kind := RequestBodyErrorInvalidCompression
		var nestedBodyErr *RequestBodyError
		var maxErr *http.MaxBytesError
		if errors.As(err, &nestedBodyErr) && nestedBodyErr != nil {
			kind = nestedBodyErr.Kind
		} else if errors.As(err, &maxErr) {
			kind = RequestBodyErrorDecompressedTooLarge
		}
		return nil, &RequestBodyError{
			Kind:          kind,
			Encoding:      encoding,
			BytesRead:     bytesRead,
			ContentLength: originalContentLength,
			Err:           fmt.Errorf("decode Content-Encoding %q: %w", encoding, err),
		}
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	return decoded, nil
}

// Read bounded chunks as bytes arrive, then assemble the exact-size result.
// This avoids doubling large buffers or eagerly allocating an untrusted
// Content-Length before the corresponding bytes have arrived.
func readRequestBodyChunks(reader io.Reader, initialCapacity int, contentLength int64) ([]byte, int64, error) {
	capacity := initialCapacity
	var chunks [][]byte
	total := 0
	for {
		chunkCapacity := capacity
		if remaining := contentLength - int64(total); remaining >= 0 && remaining < int64(chunkCapacity) {
			chunkCapacity = int(remaining) + 1
		}
		chunk := make([]byte, chunkCapacity)
		n := 0
		var err error
		for n < len(chunk) && err == nil {
			var read int
			read, err = reader.Read(chunk[n:])
			n += read
		}
		if err != nil && err != io.EOF {
			return nil, int64(total + n), err
		}
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			total += n
		}
		if err != nil {
			if len(chunks) == 0 {
				return chunk[:0], 0, nil
			}
			if len(chunks) == 1 {
				return chunks[0], int64(total), nil
			}
			body := make([]byte, total)
			offset := 0
			for _, part := range chunks {
				offset += copy(body[offset:], part)
			}
			return body, int64(total), nil
		}
		if capacity < requestBodyReadMaxInitCap {
			capacity *= 2
			if capacity > requestBodyReadMaxInitCap {
				capacity = requestBodyReadMaxInitCap
			}
		}
	}
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	body, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		return nil, err
	}
	return NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return readDecompressedBody(dec, maxDecompressedBodySize)
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return readDecompressedBody(gr, maxDecompressedBodySize)
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return readDecompressedBody(zr, maxDecompressedBodySize)
	default:
		return nil, &RequestBodyError{
			Kind:     RequestBodyErrorUnsupportedEncoding,
			Encoding: encoding,
			Err:      errors.New("unsupported Content-Encoding"),
		}
	}
}

func readDecompressedBody(reader io.Reader, limit int64) ([]byte, error) {
	decoded, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(decoded)) > limit {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	return decoded, nil
}

func normalizeContentEncoding(encoding string) string {
	return strings.ToLower(strings.TrimSpace(encoding))
}

func classifyRequestBodyReadError(err error) RequestBodyErrorKind {
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, context.Canceled):
		return RequestBodyErrorReadCanceled
	case errors.Is(err, io.ErrUnexpectedEOF):
		return RequestBodyErrorUnexpectedEOF
	case errors.As(err, &maxErr):
		return RequestBodyErrorTooLarge
	default:
		return RequestBodyErrorReadFailed
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
