package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
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
	// zstdRequestMaxExpansion bounds the buffer sized from an untrusted zstd
	// frame header to this many times the compressed body.
	zstdRequestMaxExpansion = 32
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

	raw, err := readRequestBodyChunks(req.Body, capHint, req.ContentLength)
	if err != nil {
		return nil, err
	}

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, nil
	}

	decoded, err := decompressRequestBody(enc, raw)
	if err != nil {
		return nil, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	return decoded, nil
}

// Read bounded chunks as bytes arrive, then assemble the exact-size result.
// This avoids doubling large buffers or eagerly allocating an untrusted
// Content-Length before the corresponding bytes have arrived.
func readRequestBodyChunks(reader io.Reader, initialCapacity int, contentLength int64) ([]byte, error) {
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
			return nil, err
		}
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			total += n
		}
		if err != nil {
			if len(chunks) == 0 {
				return chunk[:0], nil
			}
			if len(chunks) == 1 {
				return chunks[0], nil
			}
			body := make([]byte, total)
			offset := 0
			for _, part := range chunks {
				offset += copy(body[offset:], part)
			}
			return body, nil
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
		if decoded, ok := decodeZstdRequestBodyOfDeclaredSize(raw); ok {
			return decoded, nil
		}
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
		return nil, errors.New("unsupported Content-Encoding")
	}
}

// decodeZstdRequestBodyOfDeclaredSize decodes a body that is a single zstd
// frame declaring its content size into a buffer of exactly that size, instead
// of growing the result through io.ReadAll behind a concurrent streaming
// decoder. The decoder is synchronous and belongs to this call only: a decoder
// kept across requests holds on to the last body it decoded. ok is false when
// the body is not such a frame or decoding fails; the caller then decodes with
// the streaming reader, which keeps the existing limit and error behavior.
func decodeZstdRequestBodyOfDeclaredSize(raw []byte) ([]byte, bool) {
	size, ok := zstdSingleFrameContentSize(raw)
	if !ok {
		return nil, false
	}
	dec, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(maxDecompressedBodySize),
		zstd.WithDecodeAllCapLimit(true))
	if err != nil {
		return nil, false
	}
	defer dec.Close()
	decoded, err := dec.DecodeAll(raw, make([]byte, 0, size))
	if err != nil {
		return nil, false
	}
	return decoded, true
}

// zstdSingleFrameContentSize returns the content size declared by raw when raw
// is exactly one complete zstd frame whose size is non-zero, within the limit
// and at most zstdRequestMaxExpansion times the compressed body. Anything after
// the frame, a skippable frame included, disqualifies the body: when DecodeAll
// moves on to another frame it hands the previous frame's Huffman state, which
// still points into raw, to a package-level pool of the zstd library.
func zstdSingleFrameContentSize(raw []byte) (int, bool) {
	var header zstd.Header
	if err := header.Decode(raw); err != nil || !header.HasFCS || header.FrameContentSize == 0 ||
		header.FrameContentSize > maxDecompressedBodySize ||
		header.FrameContentSize > uint64(len(raw))*zstdRequestMaxExpansion {
		return 0, false
	}
	// Walk the block headers (RFC 8878 section 3.1.1.2) to find where the
	// frame ends.
	i := header.HeaderSize
	for {
		if i+3 > len(raw) {
			return 0, false
		}
		blockHeader := uint32(raw[i]) | uint32(raw[i+1])<<8 | uint32(raw[i+2])<<16
		i += 3
		switch (blockHeader >> 1) & 3 {
		case 0, 2: // raw and compressed blocks carry Block_Size bytes
			i += int(blockHeader >> 3)
		case 1: // an RLE block carries the single byte to repeat
			i++
		default:
			return 0, false
		}
		if i > len(raw) {
			return 0, false
		}
		if blockHeader&1 != 0 {
			break
		}
	}
	if header.HasCheckSum {
		i += 4
	}
	if i != len(raw) {
		return 0, false
	}
	return int(header.FrameContentSize), true
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
