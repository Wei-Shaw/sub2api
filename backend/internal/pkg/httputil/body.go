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
	"time"

	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize  = 64 << 20
	maxRequestZstdWindowSize = 8 << 20
)

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
	body, err := readRequestBodyWithBudget(req)
	return body.data, err
}

// requestBodyBuffer carries only allocations owned by the current helper.
// Borrowed input (notably PrereadBody) has no charge to release: its original
// owner may still retain it even if normalization creates a replacement.
type requestBodyBuffer struct {
	data    []byte
	charged int64
}

func (b *requestBodyBuffer) release(ctx context.Context) {
	charged := b.charged
	*b = requestBodyBuffer{}
	releaseRequestBodyMemory(ctx, charged)
}

func readRequestBodyWithBudget(req *http.Request) (requestBodyBuffer, error) {
	if req == nil || req.Body == nil {
		return requestBodyBuffer{}, nil
	}
	if err := req.Context().Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	if preread, ok := req.Body.(*PrereadBody); ok {
		return requestBodyBuffer{data: preread.Bytes()}, nil
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

	startedAt := time.Now()
	raw, err := readRequestBodyChunksWithBudget(req.Context(), req.Body, capHint, req.ContentLength)
	if err != nil {
		var readErr *RequestBodyReadError
		if errors.As(err, &readErr) {
			readErr.Elapsed = time.Since(startedAt)
		}
		return requestBodyBuffer{}, err
	}

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, nil
	}

	defer raw.release(req.Context())
	decoded, err := decompressRequestBodyWithBudget(req.Context(), enc, raw.data)
	if err != nil {
		return requestBodyBuffer{}, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded.data))

	return decoded, nil
}

// Read bounded chunks as bytes arrive, then assemble the exact-size result.
// This avoids doubling large buffers or eagerly allocating an untrusted
// Content-Length before the corresponding bytes have arrived.
func readRequestBodyChunksWithBudget(ctx context.Context, reader io.Reader, initialCapacity int, contentLength int64) (result requestBodyBuffer, err error) {
	var reserved int64
	defer func() {
		// Chunks and the EOF probe are temporary once the final buffer is
		// assembled. On failure there is no result, so refund all allocations.
		releaseRequestBodyMemory(ctx, reserved-result.charged)
	}()
	capacity := initialCapacity
	var chunks [][]byte
	total := 0
	for {
		chunkCapacity := capacity
		if remaining := contentLength - int64(total); remaining >= 0 && remaining < int64(chunkCapacity) {
			chunkCapacity = int(remaining) + 1
		}
		if err := reserveRequestBodyMemory(ctx, int64(chunkCapacity)); err != nil {
			return requestBodyBuffer{}, err
		}
		reserved += int64(chunkCapacity)
		chunk := make([]byte, chunkCapacity)
		n := 0
		var err error
		for n < len(chunk) && err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return requestBodyBuffer{}, &RequestBodyReadError{Err: ctxErr, BytesRead: int64(total + n)}
			}
			var read int
			read, err = reader.Read(chunk[n:])
			n += read
		}
		// Report the read error before the context: on HTTP/1 a read deadline
		// also cancels the request context, which would hide the timeout.
		if err != nil && err != io.EOF {
			return requestBodyBuffer{}, &RequestBodyReadError{Err: err, BytesRead: int64(total + n)}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return requestBodyBuffer{}, &RequestBodyReadError{Err: ctxErr, BytesRead: int64(total + n)}
		}
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			total += n
		}
		if err != nil {
			if len(chunks) == 0 {
				return requestBodyBuffer{}, nil
			}
			if len(chunks) == 1 {
				return requestBodyBuffer{data: chunks[0], charged: int64(cap(chunks[0]))}, nil
			}
			if err := reserveRequestBodyMemory(ctx, int64(total)); err != nil {
				return requestBodyBuffer{}, err
			}
			reserved += int64(total)
			body := make([]byte, total)
			offset := 0
			for _, part := range chunks {
				offset += copy(body[offset:], part)
			}
			return requestBodyBuffer{data: body, charged: int64(total)}, nil
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
	if req == nil {
		return nil, nil
	}
	body, err := readRequestBodyWithBudget(req)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeLenientJSONRequestBody(req.Context(), body.data, maxNormalizedBytes)
	if err != nil || normalized.charged > 0 {
		body.release(req.Context())
	}
	return normalized.data, err
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	body, err := decompressRequestBodyWithBudget(context.Background(), encoding, raw)
	return body.data, err
}

func decompressRequestBodyWithBudget(ctx context.Context, encoding string, raw []byte) (requestBodyBuffer, error) {
	var reader io.Reader
	switch encoding {
	case "zstd":
		if err := reserveRequestBodyMemory(ctx, maxRequestZstdWindowSize); err != nil {
			return requestBodyBuffer{}, err
		}
		defer releaseRequestBodyMemory(ctx, maxRequestZstdWindowSize)
		dec, err := zstd.NewReader(bytes.NewReader(raw),
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxWindow(maxRequestZstdWindowSize),
			zstd.WithDecoderMaxMemory(maxDecompressedBodySize))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer dec.Close()
		reader = dec
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer func() { _ = gr.Close() }()
		reader = gr
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return requestBodyBuffer{}, err
		}
		defer func() { _ = zr.Close() }()
		reader = zr
	default:
		return requestBodyBuffer{}, errors.New("unsupported Content-Encoding")
	}
	decoded, err := readRequestBodyChunksWithBudget(ctx, io.LimitReader(reader, maxDecompressedBodySize+1), requestBodyReadInitCap, -1)
	if err != nil {
		return requestBodyBuffer{}, err
	}
	if len(decoded.data) > maxDecompressedBodySize {
		decoded.release(ctx)
		return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxDecompressedBodySize}
	}
	return decoded, nil
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	normalized, err := normalizeLenientJSONRequestBody(context.Background(), body, maxNormalizedBytes)
	return normalized.data, err
}

func normalizeLenientJSONRequestBody(ctx context.Context, body []byte, maxNormalizedBytes int64) (requestBodyBuffer, error) {
	if err := ctx.Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return requestBodyBuffer{data: body}, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	normalizedSize := int64(len(body))
	inString := false
	escaped := false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			if maxNormalizedBytes-normalizedSize < 5 {
				return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			normalizedSize += 5
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
	}
	if err := ctx.Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	if normalizedSize == int64(len(body)) {
		return requestBodyBuffer{data: body}, nil
	}
	if err := reserveRequestBodyMemory(ctx, normalizedSize); err != nil {
		return requestBodyBuffer{}, err
	}
	// Allocate once after counting expansion, so escaped control bytes cannot
	// bypass the ingress budget through append's geometric capacity growth.
	out := make([]byte, 0, int(normalizedSize))
	inString, escaped = false, false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
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
		out = append(out, b)
	}
	if err := ctx.Err(); err != nil {
		releaseRequestBodyMemory(ctx, normalizedSize)
		return requestBodyBuffer{}, err
	}
	return requestBodyBuffer{data: out, charged: normalizedSize}, nil
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
