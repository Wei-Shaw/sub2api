//go:build unit

package plugin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestGetOrCreateProxy_HostAndPathRewrite 验证 Director->Rewrite 迁移后:
//   - 请求被正确路由到 target host:port
//   - Out.Host 头被重写为 target.Host (NewSingleHostReverseProxy 默认不会, 我们手动开)
//   - X-Forwarded-For 行为保持 "追加 client IP"
func TestGetOrCreateProxy_HostAndPathRewrite(t *testing.T) {
	// 上游 server 收到请求后回写 Host / X-Forwarded-For 头让我们断言。
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Recv-Host", r.Host)
		w.Header().Set("X-Recv-XFF", r.Header.Get("X-Forwarded-For"))
		_, _ = io.WriteString(w, "hi from "+r.URL.Path)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}

	r := &PluginRouter{}
	proxy := r.getOrCreateProxy(upstream.URL, target)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugin-path", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // simulate already-proxied client
	req.RemoteAddr = "5.6.7.8:54321"

	proxy.ServeHTTP(rr, req)

	if got := rr.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "/plugin-path") {
		t.Fatalf("upstream did not see correct path, body=%q", string(body))
	}
	if got := rr.Result().Header.Get("X-Recv-Host"); got != target.Host {
		t.Fatalf("upstream Host = %q, want %q", got, target.Host)
	}
	xff := rr.Result().Header.Get("X-Recv-XFF")
	if !strings.Contains(xff, "1.2.3.4") {
		t.Fatalf("XFF lost original client: %q", xff)
	}
	if !strings.Contains(xff, "5.6.7.8") {
		t.Fatalf("XFF did not append proxy client: %q", xff)
	}
}
