package diagnostics

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisabledAndInvalidConfiguration(t *testing.T) {
	for _, value := range []string{"", "false", "0"} {
		t.Run("disabled_"+value, func(t *testing.T) {
			server, err := start(func(key string) string {
				if key == "SUB2API_PPROF_ENABLED" {
					return value
				}
				return "invalid-port-is-ignored-when-disabled"
			})
			if err != nil || server != nil {
				t.Fatalf("disabled diagnostics started: server=%v err=%v", server, err)
			}
		})
	}
	for _, env := range []map[string]string{
		{"SUB2API_PPROF_ENABLED": "invalid"},
		{"SUB2API_PPROF_ENABLED": "true", "SUB2API_PPROF_PORT": "0"},
		{"SUB2API_PPROF_ENABLED": "true", "SUB2API_PPROF_PORT": "65536"},
		{"SUB2API_PPROF_ENABLED": "true", "SUB2API_PPROF_PORT": "0.0.0.0:6060"},
	} {
		server, err := start(func(key string) string { return env[key] })
		if server != nil || err == nil {
			t.Fatalf("invalid configuration accepted: %v", env)
		}
	}
}

func TestAcceptNonLoopback(t *testing.T) {
	for _, remote := range []string{"192.0.2.10:1234", "[2001:db8::1]:1234"} {
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		recorder := httptest.NewRecorder()
		handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("remote %s rejected: %d", remote, recorder.Code)
		}
	}
}

func TestWildcardHeapProfileAndClose(t *testing.T) {
	server, err := listen(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	host, port, err := net.SplitHostPort(server.Addr)
	if err != nil || host != "0.0.0.0" {
		t.Fatalf("unexpected bind: %s", server.Addr)
	}
	resp, err := http.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/debug/pprof/heap?gc=1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("not a binary profile: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(zr)
	if err != nil || len(data) == 0 {
		t.Fatalf("invalid heap profile: bytes=%d err=%v", len(data), err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port))
	if err == nil {
		_ = conn.Close()
		t.Fatal("listener remained open after Close")
	}
}

func TestOnlyGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/debug/pprof/heap", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST accepted: %d", recorder.Code)
	}
}
