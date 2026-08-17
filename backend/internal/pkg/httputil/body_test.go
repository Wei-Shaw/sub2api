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
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/klauspost/compress/zstd"
)

const samplePayload = `{"model":"gpt-5.5","input":"hi","stream":false}`

type fixedErrorBody struct {
	payload []byte
	err     error
	read    bool
}

func (r *fixedErrorBody) Read(p []byte) (int, error) {
	if r.read {
		return 0, r.err
	}
	r.read = true
	return copy(p, r.payload), r.err
}

func (r *fixedErrorBody) Close() error { return nil }

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
	const unsafeEncoding = "secret-unsupported-encoding"
	req := newRequestWithBody(t, []byte(samplePayload), unsafeEncoding)
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Stage != requestBodyErrorStageDecompress {
		t.Fatalf("stage mismatch: got %q", diagnostics.Stage)
	}
	if diagnostics.Class != requestBodyErrorClassUnsupported {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
	if got := err.Error(); got != "request body decompression failed" {
		t.Fatalf("unexpected unsafe error text: %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsCorruptZstd(t *testing.T) {
	req := newRequestWithBody(t, []byte("not actually zstd"), "zstd")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for corrupt zstd body, got nil")
	}
}

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

func TestReadRequestBodyWithPrealloc_ReportsTransportUnexpectedEOF(t *testing.T) {
	payload := []byte("partial-body")
	req := newRequestWithBody(t, nil, "")
	req.Body = &fixedErrorBody{payload: payload, err: io.ErrUnexpectedEOF}
	req.ContentLength = int64(len(payload) + 4)

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected transport read error, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("errors.Is did not preserve unexpected EOF: %v", err)
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Stage != requestBodyErrorStageTransportRead {
		t.Fatalf("stage mismatch: got %q", diagnostics.Stage)
	}
	if diagnostics.Class != requestBodyErrorClassUnexpectedEOF {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
	if diagnostics.BytesRead != int64(len(payload)) {
		t.Fatalf("bytes mismatch: got %d want %d", diagnostics.BytesRead, len(payload))
	}
}

func TestReadRequestBodyWithPrealloc_ReportsWrappedConnectionReset(t *testing.T) {
	payload := []byte("partial-body")
	resetErr := fmt.Errorf("transport detail: %w", syscall.ECONNRESET)
	req := newRequestWithBody(t, nil, "")
	req.Body = &fixedErrorBody{payload: payload, err: resetErr}
	req.ContentLength = int64(len(payload) + 4)

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected transport read error, got nil")
	}
	if !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("errors.Is did not preserve connection reset: %v", err)
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Class != requestBodyErrorClassConnectionReset {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
	if diagnostics.BytesRead != int64(len(payload)) {
		t.Fatalf("bytes mismatch: got %d want %d", diagnostics.BytesRead, len(payload))
	}
}

func TestReadRequestBodyWithPrealloc_ReportsContextCancellation(t *testing.T) {
	payload := []byte("partial-body")
	req := newRequestWithBody(t, nil, "")
	req.Body = &fixedErrorBody{payload: payload, err: fmt.Errorf("read canceled: %w", context.Canceled)}

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected transport read error, got nil")
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Class != requestBodyErrorClassContextCanceled {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
}

func TestReadRequestBodyWithPrealloc_ReportsDeadlineExceeded(t *testing.T) {
	payload := []byte("partial-body")
	req := newRequestWithBody(t, nil, "")
	req.Body = &fixedErrorBody{payload: payload, err: fmt.Errorf("deadline reached: %w", context.DeadlineExceeded)}

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected transport read error, got nil")
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Class != requestBodyErrorClassDeadlineExceeded {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
}

func TestReadRequestBodyWithPrealloc_ReportsTruncatedCompressedBodies(t *testing.T) {
	for _, encoding := range []string{"gzip", "zstd", "deflate"} {
		t.Run(encoding, func(t *testing.T) {
			compressed := compressedPayload(t, encoding)
			if len(compressed) < 2 {
				t.Fatal("compressed payload is too short to truncate")
			}
			corrupted := compressed[:len(compressed)-1]
			req := newRequestWithBody(t, corrupted, encoding)

			_, err := ReadRequestBodyWithPrealloc(req)
			if err == nil {
				t.Fatal("expected decompression error, got nil")
			}
			diagnostics := RequestBodyErrorDiagnosticsFromError(err)
			if diagnostics.Stage != requestBodyErrorStageDecompress {
				t.Fatalf("stage mismatch: got %q", diagnostics.Stage)
			}
			if diagnostics.Class != requestBodyErrorClassDecompression {
				t.Fatalf("class mismatch: got %q", diagnostics.Class)
			}
			if diagnostics.BytesRead != int64(len(corrupted)) {
				t.Fatalf("bytes mismatch: got %d want %d", diagnostics.BytesRead, len(corrupted))
			}
		})
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_WrapsNormalizationFailure(t *testing.T) {
	payload := []byte("{\"input\":\"\x00\"}")
	req := newRequestWithBody(t, payload, "")

	_, err := ReadLenientJSONRequestBodyWithPrealloc(req, int64(len(payload)+4))
	if err == nil {
		t.Fatal("expected normalization size error, got nil")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError through diagnostic wrapper, got %T %v", err, err)
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Stage != requestBodyErrorStageNormalize {
		t.Fatalf("stage mismatch: got %q", diagnostics.Stage)
	}
	if diagnostics.Class != requestBodyErrorClassBodyTooLarge {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
	if diagnostics.BytesRead != int64(len(payload)) {
		t.Fatalf("bytes mismatch: got %d want %d", diagnostics.BytesRead, len(payload))
	}
}

func TestReadRequestBodyWithPrealloc_MaxBytesErrorKeepsClassification(t *testing.T) {
	recorder := httptest.NewRecorder()
	req := newRequestWithBody(t, []byte("12345678"), "")
	req.Body = http.MaxBytesReader(recorder, req.Body, 4)

	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected MaxBytesError, got nil")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError through diagnostic wrapper, got %T %v", err, err)
	}
	diagnostics := RequestBodyErrorDiagnosticsFromError(err)
	if diagnostics.Class != requestBodyErrorClassBodyTooLarge {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
}

func TestRequestBodyErrorDiagnosticsFromError_DefaultsForUnwrappedError(t *testing.T) {
	diagnostics := RequestBodyErrorDiagnosticsFromError(errors.New("raw secret error"))
	if diagnostics.Stage != requestBodyErrorStageTransportRead {
		t.Fatalf("stage mismatch: got %q", diagnostics.Stage)
	}
	if diagnostics.Class != requestBodyErrorClassReadFailed {
		t.Fatalf("class mismatch: got %q", diagnostics.Class)
	}
	if diagnostics.BytesRead != 0 {
		t.Fatalf("bytes mismatch: got %d", diagnostics.BytesRead)
	}
}

func compressedPayload(t *testing.T, encoding string) []byte {
	t.Helper()
	var buf bytes.Buffer
	switch encoding {
	case "gzip":
		writer := gzip.NewWriter(&buf)
		if _, err := writer.Write([]byte(samplePayload)); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
	case "zstd":
		writer, err := zstd.NewWriter(nil)
		if err != nil {
			t.Fatalf("zstd writer: %v", err)
		}
		compressed := writer.EncodeAll([]byte(samplePayload), nil)
		if err := writer.Close(); err != nil {
			t.Fatalf("zstd close: %v", err)
		}
		return compressed
	case "deflate":
		writer := zlib.NewWriter(&buf)
		if _, err := writer.Write([]byte(samplePayload)); err != nil {
			t.Fatalf("deflate write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("deflate close: %v", err)
		}
	default:
		t.Fatalf("unsupported test encoding %q", encoding)
	}
	return buf.Bytes()
}
