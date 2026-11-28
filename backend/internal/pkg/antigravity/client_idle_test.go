//go:build unit

package antigravity

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 回归：经代理的 Client 必须设置空闲连接超时，否则请求结束后连接会永久留在连接池里。
func TestNewClient_代理Transport设置了空闲超时(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("配置代理时应使用自定义 *http.Transport，实际 %T", c.httpClient.Transport)
	}
	if tr.IdleConnTimeout <= 0 {
		t.Fatalf("IdleConnTimeout 必须大于 0，实际 %v", tr.IdleConnTimeout)
	}
	if tr.MaxIdleConnsPerHost <= 0 || tr.MaxIdleConnsPerHost > 2 {
		t.Fatalf("MaxIdleConnsPerHost 应为 1~2，实际 %d", tr.MaxIdleConnsPerHost)
	}
}

// 回归：请求结束后，空闲连接应在 IdleConnTimeout 后被关闭，而不是一直保持。
func TestNewClient_代理空闲连接会被关闭(t *testing.T) {
	var opened, closed atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		switch s {
		case http.StateNew:
			opened.Add(1)
		case http.StateClosed:
			closed.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	// 被测服务器本身充当 HTTP 代理（绝对 URI 的 GET 会直接打到它）
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	tr := c.httpClient.Transport.(*http.Transport)
	tr.IdleConnTimeout = 100 * time.Millisecond // 缩短以便测试；生产值由上一个用例保证大于 0

	resp, err := c.httpClient.Get("http://example.invalid/ping")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_ = resp.Body.Close()
	if opened.Load() != 1 {
		t.Fatalf("应只建立 1 条连接，实际 %d", opened.Load())
	}

	deadline := time.Now().Add(3 * time.Second)
	for closed.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if closed.Load() < 1 {
		t.Fatal("空闲连接在超时后仍未关闭")
	}
}
