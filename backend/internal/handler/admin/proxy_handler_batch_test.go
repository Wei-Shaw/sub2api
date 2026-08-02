package admin

import (
	"testing"
)

func TestDefaultBatchProxyName(t *testing.T) {
	got := defaultBatchProxyName("socks5h", "127.0.0.1", 1080)
	want := "socks5h://127.0.0.1:1080"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Long host truncated to 100 chars (schema MaxLen).
	longHost := ""
	for i := 0; i < 120; i++ {
		longHost += "a"
	}
	name := defaultBatchProxyName("http", longHost, 8080)
	if len(name) > 100 {
		t.Fatalf("name len=%d > 100", len(name))
	}
}
