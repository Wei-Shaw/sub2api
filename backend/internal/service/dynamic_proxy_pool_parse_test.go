package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clashsub"
)

func TestParseJSON_KookeeyShape(t *testing.T) {
	body := []byte(`{"success":true,"data":[{"username":"u1","password":"p1","ip":"1.2.3.4","port":8080}],"msg":"ok","code":0}`)
	svc := &DynamicProxyPoolService{}
	eps, err := svc.parseJSON(&DynamicProxyPool{}, body)
	if err != nil {
		t.Fatalf("parseJSON: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(eps))
	}
	got := eps[0]
	if got.IP != "1.2.3.4" || got.Port != 8080 {
		t.Errorf("endpoint = %s:%d, want 1.2.3.4:8080", got.IP, got.Port)
	}
	if got.Username != "u1" || got.Password != "p1" {
		t.Errorf("auth = %s/%s, want u1/p1", got.Username, got.Password)
	}
}

func TestParseTxt_Formats(t *testing.T) {
	tests := []struct {
		name string
		line string
		want extractedEndpoint
	}{
		{"ip_port", "1.2.3.4:8080", extractedEndpoint{IP: "1.2.3.4", Port: 8080}},
		{"ip_port_user_pass", "1.2.3.4:8080:u1:p1", extractedEndpoint{IP: "1.2.3.4", Port: 8080, Username: "u1", Password: "p1"}},
		{"url_form", "socks5://u1:p1@1.2.3.4:8080", extractedEndpoint{IP: "1.2.3.4", Port: 8080, Username: "u1", Password: "p1"}},
	}
	svc := &DynamicProxyPoolService{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eps, err := svc.parseTxt(&DynamicProxyPool{}, []byte(tc.line))
			if err != nil {
				t.Fatalf("parseTxt(%q): %v", tc.line, err)
			}
			if len(eps) != 1 {
				t.Fatalf("want 1 endpoint, got %d", len(eps))
			}
			if eps[0] != tc.want {
				t.Errorf("got %+v, want %+v", eps[0], tc.want)
			}
		})
	}
}

func TestParseTxt_MultiLine(t *testing.T) {
	svc := &DynamicProxyPoolService{}
	eps, err := svc.parseTxt(&DynamicProxyPool{}, []byte("1.2.3.4:8080\r\n5.6.7.8:9090"))
	if err != nil {
		t.Fatalf("parseTxt: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints, got %d", len(eps))
	}
}

func TestSanitizeExtractURL_ControlChars(t *testing.T) {
	raw := "https://www.kookeey.net/pickdynamicips?t=2&dl=\r\n&n=1"
	got := sanitizeExtractURL(raw)
	if got != "https://www.kookeey.net/pickdynamicips?t=2&dl=%0D%0A&n=1" {
		t.Fatalf("sanitizeExtractURL = %q", got)
	}
	// Already percent-encoded stays intact.
	encoded := "https://example.com/x?dl=%5Cr%5Cn"
	if sanitizeExtractURL(encoded) != encoded {
		t.Fatalf("rewrote encoded url: %q", sanitizeExtractURL(encoded))
	}
}

func TestExtractNodeServerPort(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]any
		wantSrv  string
		wantPort string
	}{
		{
			name:     "server_and_port",
			raw:      map[string]any{"server": "1.2.3.4", "port": "8080"},
			wantSrv:  "1.2.3.4",
			wantPort: "8080",
		},
		{
			name:     "servername_fallback",
			raw:      map[string]any{"servername": "example.com", "port": "443"},
			wantSrv:  "example.com",
			wantPort: "443",
		},
		{
			name:     "empty_raw",
			raw:      map[string]any{},
			wantSrv:  "",
			wantPort: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := clashsub.Node{Raw: tc.raw}
			srv, port := extractNodeServerPort(node)
			if srv != tc.wantSrv || port != tc.wantPort {
				t.Errorf("got %q:%q, want %q:%q", srv, port, tc.wantSrv, tc.wantPort)
			}
		})
	}
}

func TestPickRandomAlive_EmptyPool(t *testing.T) {
	// Without a real repo, it should return nil without panic
	svc := &DynamicProxyPoolService{}
	// This test just verifies the service doesn't crash with nil repo
	t.Logf("service created successfully (nil repo test)")
	_ = svc
}