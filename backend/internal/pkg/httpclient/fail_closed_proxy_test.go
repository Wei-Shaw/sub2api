package httpclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Mirrors service.FailClosedProxyURL (kept literal to avoid an import cycle).
const failClosedProxyURL = "socks5h://127.0.0.1:1"

func TestFailClosedProxyNeverReachesTargetDirectly(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	for _, validate := range []bool{false, true} {
		client, err := GetClient(Options{ProxyURL: failClosedProxyURL, Timeout: 3 * time.Second, ValidateResolvedIP: validate, AllowPrivateHosts: false})
		if err != nil {
			t.Fatalf("validate=%v: sentinel proxy must be accepted, got %v", validate, err)
		}
		resp, err := client.Get(srv.URL)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("validate=%v: request through fail-closed proxy unexpectedly succeeded", validate)
		}
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("target received %d direct request(s); fail-closed proxy leaked", n)
	}
}
