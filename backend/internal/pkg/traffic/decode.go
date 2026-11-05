package traffic

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// DecodeResponseBody replaces a gzip/br/deflate/zstd response body with its
// decoded representation and reports whether it did. Decoded responses drop
// Content-Encoding and Content-Length (the length no longer applies).
//
// The underlying I/O/codec error is preserved so lazy callers can distinguish
// cancellation and network failure from malformed encoding. A failed decode
// keeps every undecodable byte and the original headers: it must not silently
// turn a real body into an empty one.
func DecodeResponseBody(resp *http.Response) (bool, error) {
	if resp == nil || resp.Body == nil {
		return false, nil
	}
	ce := strings.ToLower(strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ",")))
	if ce == "" || ce == "identity" {
		return false, nil
	}

	originalBody := resp.Body
	bufferedBody := bufio.NewReader(originalBody)
	// Preserve undecodable bytes, including bytes consumed by decoder creation.
	// Failed decoding must not silently turn a real body into an empty one.
	resp.Body = &decompressedBody{reader: bufferedBody, closer: originalBody}
	initializer := &decoderInitReader{Reader: bufferedBody, captured: &bytes.Buffer{}}
	failed := func(err error) (bool, error) {
		resp.Body = &decompressedBody{reader: io.MultiReader(bytes.NewReader(initializer.captured.Bytes()), bufferedBody), closer: originalBody}
		return false, err
	}
	var reader io.Reader
	switch ce {
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(initializer)
		if err != nil {
			return failed(err)
		}
		reader = gr
	case "br":
		reader = brotli.NewReader(bufferedBody)
	case "deflate":
		// HTTP deflate is zlib-wrapped (RFC 1950). Retain compatibility with
		// legacy upstreams emitting a raw RFC 1951 stream.
		header, peekErr := bufferedBody.Peek(2)
		if peekErr != nil && len(header) < 2 {
			return failed(peekErr)
		}
		if len(header) == 2 && header[0]&15 == 8 && (int(header[0])*256+int(header[1]))%31 == 0 {
			zr, err := zlib.NewReader(initializer)
			if err != nil {
				return failed(err)
			}
			reader = zr
		} else {
			reader = flate.NewReader(bufferedBody)
		}
	case "zstd":
		headerBytes, peekErr := bufferedBody.Peek(zstd.HeaderMaxSize)
		var header zstd.Header
		if err := header.Decode(headerBytes); err != nil {
			if peekErr != nil && !errors.Is(peekErr, io.EOF) {
				err = peekErr
			}
			slog.Warn("zstd_decompress_failed", "error", err)
			return false, err
		}
		zr, err := zstd.NewReader(bufferedBody)
		if err != nil {
			slog.Warn("zstd_decompress_failed", "error", err)
			return false, err
		}
		reader = &zstdResponseReader{ReadCloser: zr.IOReadCloser()}
	default:
		return false, nil
	}
	initializer.captured = nil
	resp.Body = &decompressedBody{reader: reader, closer: originalBody}
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length") // 解压后长度不确定
	resp.ContentLength = -1
	return true, nil
}

// decoderInitReader records only decoder construction reads for lossless error
// fallback; once initialized it stops retaining bytes (streaming stays bounded).
type decoderInitReader struct {
	io.Reader
	captured *bytes.Buffer
}

func (r *decoderInitReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if r.captured != nil {
		_, _ = r.captured.Write(p[:n])
	}
	return n, err
}

type zstdResponseReader struct {
	io.ReadCloser
	warnOnce sync.Once
}

func (r *zstdResponseReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.warnOnce.Do(func() {
			slog.Warn("zstd_decompress_failed", "error", err)
		})
	}
	return n, err
}

// decompressedBody 组合解压 reader 和原始 body 的 close。
type decompressedBody struct {
	reader io.Reader
	closer io.Closer
}

func (d *decompressedBody) Read(p []byte) (int, error) {
	return d.reader.Read(p)
}

func (d *decompressedBody) Close() error {
	// 如果 reader 本身也是 Closer（如 gzip.Reader），先关闭它
	if rc, ok := d.reader.(io.Closer); ok {
		_ = rc.Close()
	}
	return d.closer.Close()
}

// DecompressResponseBodyLazy advertises the decoded representation without
// reading the entity. In particular, merely receiving gzip/deflate/zstd headers
// must not wait for the first body byte or defeat ResponseHeaderTimeout.
// Unsupported/stacked encodings stay opaque and retain their original headers.
// A non-nil counter receives the decoded (original) byte count.
func DecompressResponseBodyLazy(resp *http.Response, counter *Counter) {
	if resp == nil || resp.Body == nil || resp.Body == http.NoBody || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return
	}
	encoding := strings.ToLower(strings.TrimSpace(ContentEncoding(resp.Header)))
	switch encoding {
	case "gzip", "x-gzip", "br", "deflate", "zstd":
	default:
		return
	}
	encoded := *resp
	encoded.Header = resp.Header.Clone()
	resp.Body = &lazyDecompressedBody{encoded: &encoded, counter: counter, encoding: encoding}
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.ContentLength = -1
}

// lazyDecompressedBody is not safe for concurrent Read and Close: callers
// serialize them (abort a blocked Read by cancelling the request context, then
// Close). Close here never initializes a decoder and never drains unread body
// data.
type lazyDecompressedBody struct {
	encoded     *http.Response
	counter     *Counter
	encoding    string
	body        io.ReadCloser
	initialized bool
	initErr     error
}

func (b *lazyDecompressedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !b.initialized {
		b.initialized = true
		if decoded, err := DecodeResponseBody(b.encoded); !decoded {
			// Supported but malformed coding must not be returned as raw bytes
			// with already-normalized decoded headers. Match implicit Go gzip:
			// decoding errors surface from Read, never from receiving headers.
			b.counter.FailOriginal()
			if err == nil {
				err = fmt.Errorf("invalid compressed body")
			}
			b.initErr = fmt.Errorf("decode Content-Encoding %q: %w", b.encoding, err)
			b.body = b.encoded.Body
		} else {
			b.body = ObserveOriginal(b.encoded.Body, b.counter)
		}
	}
	if b.initErr != nil {
		return 0, b.initErr
	}
	return b.body.Read(p)
}

func (b *lazyDecompressedBody) Close() error {
	if b.body != nil {
		return b.body.Close()
	}
	return b.encoded.Body.Close()
}
