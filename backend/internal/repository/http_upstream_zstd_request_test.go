package repository

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type zstdCapture struct {
	mu       sync.Mutex
	encoding []string
	wire     []int64
	decoded  [][]byte
}

// newZstdUpstream 模拟上游 sub2api：用与 handler 相同的 ReadRequestBodyWithPrealloc 解码请求体。
func newZstdUpstream(t *testing.T, tlsServer bool) (*httptest.Server, *zstdCapture) {
	t.Helper()
	got := &zstdCapture{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc := r.Header.Get("Content-Encoding")
		wire := r.ContentLength
		body, err := httputil.ReadRequestBodyWithPrealloc(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got.mu.Lock()
		got.encoding = append(got.encoding, enc)
		got.wire = append(got.wire, wire)
		got.decoded = append(got.decoded, body)
		got.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	var srv *httptest.Server
	if tlsServer {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)
	return srv, got
}

func zstdTestService(t *testing.T, hosts []string) *httpUpstreamService {
	t.Helper()
	cfg := &config.Config{Gateway: config.GatewayConfig{UpstreamZstdRequestHosts: hosts}}
	s, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)
	return s
}

func compressibleBody(n int) []byte {
	var b bytes.Buffer
	_, _ = b.WriteString(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"`)
	for b.Len() < n {
		_, _ = b.WriteString("请对比这两段代码的差异并给出修改建议。func main() { fmt.Println(\\\"hello\\\") } ")
	}
	_, _ = b.WriteString(`"}]}`)
	return b.Bytes()
}

func hostOf(t *testing.T, raw string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	require.NoError(t, err)
	return req.URL.Hostname()
}

func TestHTTPUpstreamZstdRequestBodyRoundTrip(t *testing.T) {
	srv, got := newZstdUpstream(t, false)
	s := zstdTestService(t, []string{hostOf(t, srv.URL)})

	body := compressibleBody(400 << 10)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Do(req, "", 1, 1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Len(t, got.decoded, 1)
	require.Equal(t, "zstd", got.encoding[0])
	require.Less(t, got.wire[0], int64(len(body))/3, "wire bytes should be far below raw size")
	require.Equal(t, body, got.decoded[0], "upstream must decode exactly the original body")

	// 调用方手里的 req 不能被改（重试、日志、审计都还会用它）。
	require.Empty(t, req.Header.Get("Content-Encoding"))
	require.Equal(t, int64(len(body)), req.ContentLength)
	again, err := req.GetBody()
	require.NoError(t, err)
	orig, err := io.ReadAll(again)
	require.NoError(t, err)
	require.Equal(t, body, orig)
}

func TestHTTPUpstreamZstdRequestBodyWithTLSRoundTrip(t *testing.T) {
	srv, got := newZstdUpstream(t, true)
	s := zstdTestService(t, []string{hostOf(t, srv.URL)})
	profile := &tlsfingerprint.Profile{Name: "zstd-test"}
	entry, err := s.getClientEntryWithTLS("", 1, 1, profile, service.HTTPUpstreamProfileDefault, false, true)
	require.NoError(t, err)
	entry.client = srv.Client()

	body := compressibleBody(200 << 10)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	resp, err := s.DoWithTLS(req, "", 1, 1, profile)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Len(t, got.decoded, 1)
	require.Equal(t, "zstd", got.encoding[0])
	require.Equal(t, body, got.decoded[0])
}

func TestHTTPUpstreamZstdRequestBodySkips(t *testing.T) {
	big := compressibleBody(100 << 10)
	cases := []struct {
		name   string
		hosts  func(srvHost string) []string
		body   func() io.Reader
		header map[string]string
	}{
		{
			name:  "host not listed",
			hosts: func(string) []string { return []string{"other.example"} },
			body:  func() io.Reader { return bytes.NewReader(big) },
		},
		{
			name:  "feature off",
			hosts: func(string) []string { return nil },
			body:  func() io.Reader { return bytes.NewReader(big) },
		},
		{
			name:  "body below threshold",
			hosts: func(h string) []string { return []string{h} },
			body:  func() io.Reader { return bytes.NewReader(big[:4<<10]) },
		},
		{
			name:   "already encoded by caller",
			hosts:  func(h string) []string { return []string{h} },
			body:   func() io.Reader { return bytes.NewReader(big) },
			header: map[string]string{"Content-Encoding": "identity-test"},
		},
		{
			name:  "incompressible body",
			hosts: func(h string) []string { return []string{h} },
			body: func() io.Reader {
				b := make([]byte, 64<<10)
				for i := range b {
					b[i] = byte((i*2654435761)>>13 ^ i)
				}
				return bytes.NewReader(b)
			},
		},
		{
			name:  "streaming body without GetBody",
			hosts: func(h string) []string { return []string{h} },
			body:  func() io.Reader { return io.NopCloser(strings.NewReader(string(big))) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotEnc string
			var gotLen int64
			var gotBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotEnc = r.Header.Get("Content-Encoding")
				gotLen = r.ContentLength
				gotBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)
			s := zstdTestService(t, tc.hosts(hostOf(t, srv.URL)))

			src := tc.body()
			want, _ := io.ReadAll(src)
			var reader io.Reader = bytes.NewReader(want)
			if tc.name == "streaming body without GetBody" {
				reader = io.NopCloser(bytes.NewReader(want))
			}
			req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", reader)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}

			resp, err := s.Do(req, "", 1, 1)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			if tc.header["Content-Encoding"] != "" {
				require.Equal(t, tc.header["Content-Encoding"], gotEnc)
			} else {
				require.Empty(t, gotEnc)
			}
			require.Equal(t, want, gotBody, "body must reach upstream unchanged")
			if req.ContentLength > 0 {
				require.Equal(t, int64(len(want)), gotLen)
			}
		})
	}
}

// 传输层在连接被复用失败时会调 GetBody 重放；重放出去的必须仍是 zstd 字节。
func TestHTTPUpstreamZstdRequestBodyGetBodyReplaysCompressed(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{UpstreamZstdRequestHosts: []string{"relay.example"}}}
	s := &httpUpstreamService{cfg: cfg}
	body := compressibleBody(64 << 10)
	req, err := http.NewRequest(http.MethodPost, "https://relay.example/v1/chat/completions", bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	out := s.compressUpstreamRequestBody(req, 0)
	require.NotSame(t, req, out)
	first, err := io.ReadAll(out.Body)
	require.NoError(t, err)
	require.Equal(t, out.ContentLength, int64(len(first)))
	replay, err := out.GetBody()
	require.NoError(t, err)
	second, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, first, second)

	decoded, err := httputil.ReadRequestBodyWithPrealloc(&http.Request{
		Header:        http.Header{"Content-Encoding": []string{"zstd"}},
		Body:          io.NopCloser(bytes.NewReader(second)),
		ContentLength: int64(len(second)),
	})
	require.NoError(t, err)
	require.Equal(t, body, decoded)
}

func TestHTTPUpstreamZstdRequestBodyCrossHostRedirect(t *testing.T) {
	destination, got := newZstdUpstream(t, false)
	firstEncoding := make(chan string, 1)
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstEncoding <- r.Header.Get("Content-Encoding")
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(entry.Close)
	s := zstdTestService(t, []string{"localhost"})
	s.zstdAccounts = zstdAccountsWith(7)
	body := compressibleBody(32 << 10)
	req, err := http.NewRequest(http.MethodPost, strings.Replace(entry.URL, "127.0.0.1", "localhost", 1), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Do(req, "", 7, 1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "zstd", <-firstEncoding)
	require.Equal(t, []string{""}, got.encoding)
	require.Equal(t, body, got.decoded[0])
}

func TestHTTPUpstreamZstdRequestBodyEligibility(t *testing.T) {
	s := newHTTPUpstream(&config.Config{Gateway: config.GatewayConfig{UpstreamZstdRequestHosts: []string{"RELAY.EXAMPLE"}}})
	body := compressibleBody(32 << 10)
	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"too large", func(r *http.Request) { r.ContentLength = upstreamZstdMaxBodyBytes + 1 }},
		{"stale length", func(r *http.Request) { r.ContentLength-- }},
		{"encoded with noncanonical casing", func(r *http.Request) { r.Header["content-encoding"] = []string{"gzip"} }},
		{"multipart", func(r *http.Request) { r.Header.Set("Content-Type", "multipart/form-data; boundary=test") }},
		{"signed body", func(r *http.Request) { r.Header.Set("X-Amz-Content-Sha256", "signed") }},
		{"explicit identity test", func(r *http.Request) {
			*r = *r.WithContext(service.WithUpstreamRequestZstdProbe(r.Context(), &service.UpstreamRequestZstdProbe{Compress: false}))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://relay.example:8443", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tc.mutate(req)
			require.Same(t, req, s.compressUpstreamRequestBody(req, 0))
			raw, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, raw)
		})
	}
}
