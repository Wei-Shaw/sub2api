package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestProxyConsoleURLIsAdminOnly(t *testing.T) {
	proxy := &service.Proxy{
		ID:         1,
		Name:       "managed-proxy",
		Protocol:   "socks5h",
		Host:       "127.0.0.1",
		Port:       1080,
		ConsoleURL: "http://127.0.0.1:9090/ui/",
		Status:     service.StatusActive,
	}

	publicJSON, err := json.Marshal(ProxyFromService(proxy))
	if err != nil {
		t.Fatalf("marshal public proxy: %v", err)
	}
	if strings.Contains(string(publicJSON), "console_url") {
		t.Fatalf("public proxy unexpectedly exposed console_url: %s", publicJSON)
	}

	adminJSON, err := json.Marshal(ProxyFromServiceAdmin(proxy))
	if err != nil {
		t.Fatalf("marshal admin proxy: %v", err)
	}
	if !strings.Contains(string(adminJSON), `"console_url":"http://127.0.0.1:9090/ui/"`) {
		t.Fatalf("admin proxy did not include console_url: %s", adminJSON)
	}
}
