package repository

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func newCompressionTestService(enabled bool, minBytes int64, allow []int64) *httpUpstreamService {
	cfg := &config.Config{}
	cfg.Gateway.UpstreamRequestCompression = config.GatewayUpstreamRequestCompressionConfig{
		Enabled:    enabled,
		MinBytes:   minBytes,
		AccountIDs: allow,
	}
	return &httpUpstreamService{cfg: cfg, clients: map[string]*upstreamClientEntry{}}
}

func newGzipTestRequest(t *testing.T, url string, size int) *http.Request {
	t.Helper()
	raw := bytes.Repeat([]byte("compressible-payload-"), size/21+1)[:size]
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func readAllGzip(t *testing.T, r io.Reader) []byte {
	t.Helper()
	zr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer zr.Close() //nolint:errcheck // 测试辅助函数，只读解压流；Close 的错误不影响断言
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return out
}

func TestPrepareUpstreamBodyCompression_AppliesAndRewritesHeaders(t *testing.T) {
	s := newCompressionTestService(true, 1024, nil)
	raw := bytes.Repeat([]byte("A"), 8192)
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	comp := s.prepareUpstreamBodyCompression(req, 39)
	if comp == nil {
		t.Fatal("expected compression to apply")
	}
	if got := req.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if req.ContentLength <= 0 || req.ContentLength >= int64(len(raw)) {
		t.Fatalf("ContentLength = %d, want 0 < n < %d", req.ContentLength, len(raw))
	}
	if got := req.Header.Get("Content-Length"); got == "" {
		t.Fatal("Content-Length header not rewritten")
	}
	if got := readAllGzip(t, req.Body); !bytes.Equal(got, raw) {
		t.Fatalf("gzip payload mismatch: got %d bytes, want %d", len(got), len(raw))
	}
}

func TestPrepareUpstreamBodyCompression_SkipsWhenNotApplicable(t *testing.T) {
	raw := bytes.Repeat([]byte("A"), 8192)
	cases := []struct {
		name      string
		svc       *httpUpstreamService
		accountID int64
		size      int
	}{
		{"disabled", newCompressionTestService(false, 1024, nil), 39, 8192},
		{"below min bytes", newCompressionTestService(true, 1<<20, nil), 39, 8192},
		{"account not allowlisted", newCompressionTestService(true, 1024, []int64{7, 8}), 39, 8192},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://upstream.example/x", bytes.NewReader(raw[:tc.size]))
			if err != nil {
				t.Fatal(err)
			}
			if comp := tc.svc.prepareUpstreamBodyCompression(req, tc.accountID); comp != nil {
				t.Fatal("expected no compression")
			}
			if got := req.Header.Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding = %q, want empty", got)
			}
			if req.ContentLength != int64(tc.size) {
				t.Fatalf("ContentLength = %d, want %d (must stay untouched)", req.ContentLength, tc.size)
			}
		})
	}
}

func TestPrepareUpstreamBodyCompression_SkipsAlreadyEncoded(t *testing.T) {
	s := newCompressionTestService(true, 1024, nil)
	req := newGzipTestRequest(t, "https://upstream.example/x", 8192)
	req.Header.Set("Content-Encoding", "br")

	if comp := s.prepareUpstreamBodyCompression(req, 39); comp != nil {
		t.Fatal("expected no double compression")
	}
	if got := req.Header.Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br untouched", got)
	}
}

// 上游明确拒绝 gzip 请求体（400/415）时，必须自动用未压缩请求体重试一次。
func TestDoUpstreamRequestWithOptionalCompression_FallsBackOnRejection(t *testing.T) {
	var mu sync.Mutex
	var sawEncodings []string
	var sawSizes []int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		enc := r.Header.Get("Content-Encoding")
		mu.Lock()
		sawEncodings = append(sawEncodings, enc)
		sawSizes = append(sawSizes, len(body))
		mu.Unlock()

		if enc == "gzip" {
			w.WriteHeader(http.StatusBadRequest) // 上游不接受压缩请求体
			_, _ = w.Write([]byte(`{"error":"unsupported content-encoding"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := newCompressionTestService(true, 1024, nil)
	raw := bytes.Repeat([]byte("B"), 16384)
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := s.doUpstreamRequestWithOptionalCompression(srv.Client(), req, 39)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 测试收尾，响应体关闭失败不影响断言

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after fallback retry", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sawEncodings) != 2 {
		t.Fatalf("upstream saw %d requests, want 2 (gzip then plain)", len(sawEncodings))
	}
	if sawEncodings[0] != "gzip" {
		t.Fatalf("first attempt encoding = %q, want gzip", sawEncodings[0])
	}
	if sawEncodings[1] != "" {
		t.Fatalf("retry encoding = %q, want empty", sawEncodings[1])
	}
	if sawSizes[1] != len(raw) {
		t.Fatalf("retry body size = %d, want %d", sawSizes[1], len(raw))
	}
}

// 上游接受 gzip 请求体时，只发一次，且服务端解压后拿到的内容与原文一致。
func TestDoUpstreamRequestWithOptionalCompression_AcceptedSendsOnce(t *testing.T) {
	var mu sync.Mutex
	var calls int
	var decoded []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if r.Header.Get("Content-Encoding") == "gzip" {
			decoded = readAllGzip(t, r.Body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := newCompressionTestService(true, 1024, nil)
	raw := bytes.Repeat([]byte("C"), 16384)
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := s.doUpstreamRequestWithOptionalCompression(srv.Client(), req, 39)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 测试收尾，响应体关闭失败不影响断言

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatalf("decoded body mismatch: got %d bytes, want %d", len(decoded), len(raw))
	}
}

// 未启用时不得改动请求（回归保护）。
func TestDoUpstreamRequestWithOptionalCompression_DisabledIsPassthrough(t *testing.T) {
	var gotEncoding string
	var gotLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		b, _ := io.ReadAll(r.Body)
		gotLen = len(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newCompressionTestService(false, 1024, nil)
	raw := bytes.Repeat([]byte("D"), 16384)
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.doUpstreamRequestWithOptionalCompression(srv.Client(), req, 39)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 测试收尾，响应体关闭失败不影响断言

	if gotEncoding != "" {
		t.Fatalf("Content-Encoding = %q, want empty", gotEncoding)
	}
	if gotLen != len(raw) {
		t.Fatalf("body size = %d, want %d", gotLen, len(raw))
	}
}
