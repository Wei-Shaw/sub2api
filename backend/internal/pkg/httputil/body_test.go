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
	"testing"

	"github.com/klauspost/compress/zstd"
)

const samplePayload = `{"model":"gpt-5.5","input":"hi","stream":false}`

func newRequestWithBody(t *testing.T, body []byte, encoding string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.ContentLength = int64(len(body))
	return req
}

func TestReadRequestBodyWithPrealloc_PassesThroughIdentity(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesZstd(t *testing.T) {
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte(samplePayload), nil)
	_ = enc.Close()

	req := newRequestWithBody(t, compressed, "zstd")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be cleared after decoding")
	}
	if req.ContentLength != int64(len(samplePayload)) {
		t.Fatalf("ContentLength not updated: %d", req.ContentLength)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "gzip")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesDeflate(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "deflate")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsUnsupportedEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "br")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
	if !strings.Contains(err.Error(), "br") {
		t.Fatalf("error should mention encoding, got %v", err)
	}
	diagnostic := RequestBodyDiagnostics(err)
	if diagnostic.Kind != RequestBodyErrorUnsupportedEncoding {
		t.Fatalf("unexpected error kind: %s", diagnostic.Kind)
	}
	if diagnostic.Encoding != "br" {
		t.Fatalf("unexpected encoding: %q", diagnostic.Encoding)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsCorruptZstd(t *testing.T) {
	req := newRequestWithBody(t, []byte("not actually zstd"), "zstd")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for corrupt zstd body, got nil")
	}
	if got := RequestBodyDiagnostics(err).Kind; got != RequestBodyErrorInvalidCompression {
		t.Fatalf("unexpected error kind: %s", got)
	}
}

func TestReadRequestBodyWithPrealloc_DetectsShortBody(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "")
	req.ContentLength += 10

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected short body error, got nil")
	}
	diagnostic := RequestBodyDiagnostics(err)
	if diagnostic.Kind != RequestBodyErrorUnexpectedEOF {
		t.Fatalf("unexpected error kind: %s", diagnostic.Kind)
	}
	if diagnostic.BytesRead != int64(len(samplePayload)) {
		t.Fatalf("unexpected bytes read: %d", diagnostic.BytesRead)
	}
}

type canceledBodyReader struct{}

func (canceledBodyReader) Read([]byte) (int, error) { return 0, context.Canceled }
func (canceledBodyReader) Close() error             { return nil }

func TestReadRequestBodyWithPrealloc_ClassifiesCanceledRead(t *testing.T) {
	req := newRequestWithBody(t, nil, "")
	req.Body = canceledBodyReader{}
	req.ContentLength = -1

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected canceled read error, got nil")
	}
	if got := RequestBodyDiagnostics(err).Kind; got != RequestBodyErrorReadCanceled {
		t.Fatalf("unexpected error kind: %s", got)
	}
}

func TestReadDecompressedBody_RejectsOverflow(t *testing.T) {
	_, err := readDecompressedBody(bytes.NewReader([]byte("12345")), 4)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) || maxErr.Limit != 4 {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadDecompressedBody_PreservesReadError(t *testing.T) {
	wantErr := io.ErrClosedPipe
	_, err := readDecompressedBody(errorReader{err: wantErr}, 4)
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadRequestBodyWithPrealloc_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil body, got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RespectsIdentityEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "identity")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}
