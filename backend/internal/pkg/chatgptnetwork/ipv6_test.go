package chatgptnetwork

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestPolicyScopeAndDialers(t *testing.T) {
	for _, value := range []string{"", "false", "true", "bad"} {
		for _, host := range []string{"chatgpt.com", "CHATGPT.COM.", "ws.chatgpt.com", "api.anthropic.com", "api.openai.com", "evilchatgpt.com", "chatgpt.com.evil.test"} {
			for _, proxied := range []bool{false, true} {
				t.Run(value+host+map[bool]string{true: "proxy", false: "direct"}[proxied], func(t *testing.T) {
					t.Setenv(EnvIPv6Only, value)
					called := false
					got := ""
					sentinel := errors.New("dial observed")
					dial := func(_ context.Context, network, addr string) (net.Conn, error) {
						called = true
						got = network
						return nil, sentinel
					}
					tr := &http.Transport{DialContext: dial, DialTLSContext: dial}
					Apply(tr, proxied)
					for _, fn := range []func(context.Context, string, string) (net.Conn, error){tr.DialContext, tr.DialTLSContext} {
						called = false
						_, err := fn(context.Background(), "tcp", net.JoinHostPort(host, "443"))
						blocked := isChatGPT(host) && value == "bad"
						if blocked {
							if called || err == nil {
								t.Fatal("must fail before dialing")
							}
							continue
						}
						if !called || !errors.Is(err, sentinel) {
							t.Fatal("dial not preserved")
						}
						want := "tcp"
						if value == "true" && isChatGPT(host) {
							want = "tcp6"
						}
						if got != want {
							t.Fatalf("got %s want %s", got, want)
						}
					}
				})
			}
		}
	}
}

func TestHTTPProxyUsesCustomTunnel(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	p, _ := url.Parse("http://127.0.0.1:1234")
	tr := &http.Transport{Proxy: http.ProxyURL(p)}
	Apply(tr, false)
	for _, host := range []string{"chatgpt.com", "api.anthropic.com"} {
		req, _ := http.NewRequest("GET", "https://"+host, nil)
		got, err := tr.Proxy(req)
		if host == "chatgpt.com" {
			if err != nil || got != nil {
				t.Fatal("ChatGPT must use the IPv6 tunnel dialer")
			}
		} else if err != nil || got != p {
			t.Fatal("other platform affected")
		}
	}
}

func TestIPv6SocketAndNoIPv4Fallback(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer func() { _ = ln.Close() }()
	tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, ln.Addr().String())
	}}
	Apply(tr, false)
	conn, err := tr.DialContext(context.Background(), "tcp", "chatgpt.com:443")
	if err != nil {
		t.Fatal(err)
	}
	defer func(c net.Conn) { _ = c.Close() }(conn)
	addr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		t.Fatal("unexpected address type")
	}
	if addr.IP.To4() != nil {
		t.Fatal("not IPv6")
	}
	tr = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:1")
	}}
	Apply(tr, false)
	if conn, err = tr.DialContext(context.Background(), "tcp", "chatgpt.com:443"); err == nil {
		_ = conn.Close()
		t.Fatal("unexpected IPv4 fallback")
	}
}

func TestApplyPreservesOtherHostCustomTLSWithProxy(t *testing.T) {
	t.Setenv(EnvIPv6Only, "true")
	observed := errors.New("custom TLS")
	p, _ := url.Parse("http://proxy.test:8080")
	tr := &http.Transport{Proxy: http.ProxyURL(p), DialTLSContext: func(_ context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" || addr != "api.anthropic.com:443" {
			t.Fatalf("unexpected dial %s %s", network, addr)
		}
		return nil, observed
	}}
	Apply(tr, true)
	if _, err := tr.DialTLSContext(context.Background(), "tcp", "api.anthropic.com:443"); !errors.Is(err, observed) {
		t.Fatalf("custom TLS changed: %v", err)
	}
}
