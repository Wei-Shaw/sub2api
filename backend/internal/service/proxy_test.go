package service

import (
	"net/url"
	"strings"
	"testing"
)

func TestProxyURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		proxy Proxy
		want  string
	}{
		{
			name: "without auth",
			proxy: Proxy{
				Protocol: "http",
				Host:     "proxy.example.com",
				Port:     8080,
			},
			want: "http://proxy.example.com:8080",
		},
		{
			name: "with auth",
			proxy: Proxy{
				Protocol: "socks5",
				Host:     "socks.example.com",
				Port:     1080,
				Username: "user",
				Password: "pass",
			},
			want: "socks5://user:pass@socks.example.com:1080",
		},
		{
			name: "username only keeps no auth for compatibility",
			proxy: Proxy{
				Protocol: "http",
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user-only",
			},
			want: "http://proxy.example.com:8080",
		},
		{
			name: "with special characters in credentials",
			proxy: Proxy{
				Protocol: "http",
				Host:     "proxy.example.com",
				Port:     3128,
				Username: "first last@corp",
				Password: "p@ ss:#word",
			},
			want: "http://first%20last%40corp:p%40%20ss%3A%23word@proxy.example.com:3128",
		},
		{
			name: "resin proxy keeps real transport scheme and marker",
			proxy: Proxy{
				Protocol: ProxyProtocolResinHTTPS,
				Host:     "resin.example.com",
				Port:     443,
				Username: "openai",
				Password: "token123",
			},
			want: "https://openai:token123@resin.example.com:443/token123#resin",
		},
		{
			name: "resin proxy keeps base path for reverse routing",
			proxy: Proxy{
				Protocol: ProxyProtocolResinHTTP,
				Host:     "127.0.0.1",
				Port:     2260,
				Username: "Default",
				Password: "token123",
				BasePath: "/my-token",
			},
			want: "http://Default:token123@127.0.0.1:2260/my-token#resin",
		},
		{
			name: "resin socks5 proxy stays pathless",
			proxy: Proxy{
				Protocol: ProxyProtocolResinSOCKS,
				Host:     "resin.example.com",
				Port:     2260,
				Username: "openai",
				Password: "token123",
			},
			want: "socks5h://openai:token123@resin.example.com:2260#resin",
		},
		{
			name: "resin proxy keeps username when token is missing",
			proxy: Proxy{
				Protocol: ProxyProtocolResinHTTP,
				Host:     "resin.example.com",
				Port:     2260,
				Username: "openai",
			},
			want: "http://openai@resin.example.com:2260#resin",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.proxy.URL(); got != tc.want {
				t.Fatalf("Proxy.URL() mismatch: got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestValidateResinProxyCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol string
		username string
		password string
		wantErr  string
	}{
		{
			name:     "non resin proxy does not require credentials",
			protocol: ProxyProtocolHTTP,
		},
		{
			name:     "resin requires platform",
			protocol: ProxyProtocolResinHTTP,
			password: "token123",
			wantErr:  "Resin platform is required",
		},
		{
			name:     "resin allows empty token",
			protocol: ProxyProtocolResinHTTP,
			username: "openai",
		},
		{
			name:     "resin accepts both values",
			protocol: ProxyProtocolResinSOCKS,
			username: "openai",
			password: "token123",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateResinProxyCredentials(tc.protocol, tc.username, tc.password)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateResinProxyCredentials() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateResinProxyCredentials() expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateResinProxyCredentials() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestProxyResinConfig(t *testing.T) {
	t.Parallel()

	proxy := &Proxy{
		Protocol: ProxyProtocolResinHTTPS,
		Host:     "resin.example.com",
		Port:     443,
		Username: "openai",
		Password: "token123",
	}

	cfg, err := proxy.ResinConfig()
	if err != nil {
		t.Fatalf("Proxy.ResinConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("Proxy.ResinConfig() returned nil")
	}
	if got, want := cfg.ForwardProxyBaseURL(), "https://resin.example.com:443"; got != want {
		t.Fatalf("ForwardProxyBaseURL mismatch: got=%q want=%q", got, want)
	}
}

func TestProxyResinSocksConfig(t *testing.T) {
	t.Parallel()

	proxy := &Proxy{
		Protocol: ProxyProtocolResinSOCKS,
		Host:     "resin.example.com",
		Port:     2260,
		Username: "openai",
		Password: "token123",
		BasePath: "/ignored",
	}

	cfg, err := proxy.ResinConfig()
	if err != nil {
		t.Fatalf("Proxy.ResinConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("Proxy.ResinConfig() returned nil")
	}
	if got, want := cfg.ForwardProxyBaseURL(), "socks5h://resin.example.com:2260"; got != want {
		t.Fatalf("ForwardProxyBaseURL mismatch: got=%q want=%q", got, want)
	}
}

func TestProxyResinConfig_MissingTokenStillParses(t *testing.T) {
	t.Parallel()

	proxy := &Proxy{
		Protocol: ProxyProtocolResinHTTP,
		Host:     "resin.example.com",
		Port:     2260,
		Username: "openai",
	}

	cfg, err := proxy.ResinConfig()
	if err != nil {
		t.Fatalf("Proxy.ResinConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("Proxy.ResinConfig() returned nil")
	}
	if got, want := cfg.ForwardProxyBaseURL(), "http://resin.example.com:2260"; got != want {
		t.Fatalf("ForwardProxyBaseURL mismatch: got=%q want=%q", got, want)
	}
}

func TestProxyURL_SpecialCharactersRoundTrip(t *testing.T) {
	t.Parallel()

	proxy := Proxy{
		Protocol: "http",
		Host:     "proxy.example.com",
		Port:     3128,
		Username: "first last@corp",
		Password: "p@ ss:#word",
	}

	parsed, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatalf("parse proxy URL failed: %v", err)
	}
	if got := parsed.User.Username(); got != proxy.Username {
		t.Fatalf("username mismatch after parse: got=%q want=%q", got, proxy.Username)
	}
	pass, ok := parsed.User.Password()
	if !ok {
		t.Fatal("password missing after parse")
	}
	if pass != proxy.Password {
		t.Fatalf("password mismatch after parse: got=%q want=%q", pass, proxy.Password)
	}
}
