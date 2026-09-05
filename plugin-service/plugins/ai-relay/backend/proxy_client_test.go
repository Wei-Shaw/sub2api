package backend

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeProxyResolver struct {
	proxy ProxyConfig
	err   error
	calls int
}

func (r *fakeProxyResolver) Resolve(context.Context, int64) (ProxyConfig, error) {
	r.calls++
	return r.proxy, r.err
}

func TestRelayClientProviderReturnsDirectClientWithoutProxyID(t *testing.T) {
	direct := &http.Client{}
	resolver := &fakeProxyResolver{err: errors.New("must not be called")}
	provider := NewProxyClientProvider(direct, resolver)

	client, err := provider.ClientFor(context.Background(), "")
	if err != nil {
		t.Fatalf("ClientFor() error = %v", err)
	}
	if client != direct || resolver.calls != 0 {
		t.Fatalf("client = %p, direct = %p, resolver calls = %d", client, direct, resolver.calls)
	}
}

func TestRelayClientProviderRejectsMalformedOrUnavailableProxy(t *testing.T) {
	provider := NewProxyClientProvider(http.DefaultClient, &fakeProxyResolver{err: ErrProxyNotAvailable})
	if _, err := provider.ClientFor(context.Background(), "bad"); !errors.Is(err, ErrInvalidProxyID) {
		t.Fatalf("malformed ClientFor() error = %v", err)
	}
	if _, err := provider.ClientFor(context.Background(), "42"); !errors.Is(err, ErrProxyNotAvailable) {
		t.Fatalf("unavailable ClientFor() error = %v", err)
	}
}

func TestRelayClientProviderBuildsSupportedProxyTransports(t *testing.T) {
	for _, protocol := range []string{"http", "https", "socks5", "socks5h"} {
		t.Run(protocol, func(t *testing.T) {
			resolver := &fakeProxyResolver{proxy: ProxyConfig{
				ID: 42, Protocol: protocol, Host: "proxy.internal", Port: 8080,
				Username: "alice", Password: "secret", UpdatedAt: time.Unix(1, 0),
			}}
			client, err := NewProxyClientProvider(http.DefaultClient, resolver).ClientFor(context.Background(), "42")
			if err != nil {
				t.Fatalf("ClientFor() error = %v", err)
			}
			transport, ok := client.Transport.(*http.Transport)
			if !ok || transport.Proxy == nil {
				t.Fatalf("transport = %T, proxy configured = %v", client.Transport, ok && transport.Proxy != nil)
			}
			resolved, err := transport.Proxy(&http.Request{})
			if err != nil {
				t.Fatalf("transport.Proxy() error = %v", err)
			}
			if resolved.Scheme != protocol || resolved.Host != "proxy.internal:8080" {
				t.Fatalf("resolved proxy = %s", resolved)
			}
			if password, ok := resolved.User.Password(); !ok || resolved.User.Username() != "alice" || password != "secret" {
				t.Fatalf("resolved proxy credentials = %v", resolved.User)
			}
		})
	}
}

func TestRelayClientProviderReusesAndRefreshesCachedClient(t *testing.T) {
	resolver := &fakeProxyResolver{proxy: ProxyConfig{
		ID: 42, Protocol: "http", Host: "proxy.internal", Port: 8080, UpdatedAt: time.Unix(1, 0),
	}}
	provider := NewProxyClientProvider(http.DefaultClient, resolver)

	first, err := provider.ClientFor(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.ClientFor(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("unchanged proxy did not reuse cached client")
	}

	resolver.proxy.UpdatedAt = time.Unix(2, 0)
	third, err := provider.ClientFor(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if third == second {
		t.Fatal("updated proxy reused stale cached client")
	}
}
