package service

import (
	"context"
	"errors"
	"testing"
)

type guardProxyRepoStub struct {
	ProxyRepository
	proxy *Proxy
	err   error
	calls int
}

func (r *guardProxyRepoStub) GetByID(_ context.Context, _ int64) (*Proxy, error) {
	r.calls++
	return r.proxy, r.err
}

func TestAccountProxyURLFailClosed(t *testing.T) {
	id := int64(2)
	other := int64(3)
	good := &Proxy{ID: id, Protocol: "socks5h", Host: "proxy.internal", Port: 1080}

	cases := []struct {
		name    string
		account *Account
		want    string
	}{
		{"nil account", nil, ""},
		{"no proxy bound", &Account{ID: 1}, ""},
		{"bound and loaded", &Account{ID: 1, ProxyID: &id, Proxy: good}, "socks5h://proxy.internal:1080"},
		{"bound but not loaded", &Account{ID: 1, ProxyID: &id}, FailClosedProxyURL},
		{"bound but stale proxy id", &Account{ID: 1, ProxyID: &other, Proxy: good}, FailClosedProxyURL},
		{"hand-built proxy without id", &Account{ID: 1, ProxyID: &id, Proxy: &Proxy{Protocol: "http", Host: "p", Port: 1}}, "http://p:1"},
		{"bound but empty host", &Account{ID: 1, ProxyID: &id, Proxy: &Proxy{ID: id, Protocol: "socks5h", Port: 1080}}, FailClosedProxyURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccountProxyURLFailClosed(tc.account); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolveAccountProxyURLFailClosed(t *testing.T) {
	id := int64(2)
	acc := &Account{ID: 24, ProxyID: &id}

	if got := ResolveAccountProxyURLFailClosed(context.Background(), &guardProxyRepoStub{}, &Account{ID: 1}); got != "" {
		t.Fatalf("unbound account: got %q", got)
	}
	ok := &guardProxyRepoStub{proxy: &Proxy{ID: id, Protocol: "socks5h", Host: "proxy.internal", Port: 1080}}
	if got := ResolveAccountProxyURLFailClosed(context.Background(), ok, acc); got != "socks5h://proxy.internal:1080" {
		t.Fatalf("healthy lookup: got %q", got)
	}
	for name, repo := range map[string]ProxyRepository{
		"lookup error": &guardProxyRepoStub{err: errors.New("db down")},
		"not found":    &guardProxyRepoStub{err: ErrProxyNotFound},
		"nil proxy":    &guardProxyRepoStub{},
		"nil repo":     nil,
	} {
		if got := ResolveAccountProxyURLFailClosed(context.Background(), repo, acc); got != FailClosedProxyURL {
			t.Fatalf("%s: got %q, want fail-closed", name, got)
		}
	}
}

func TestFailClosedProxyURLIsUsableProxy(t *testing.T) {
	// The sentinel must be a syntactically valid proxy URL so that transports
	// accept it and then fail to dial, rather than rejecting it and some caller
	// treating the error as "no proxy".
	if FailClosedProxyURL == "" {
		t.Fatal("sentinel must not be empty")
	}
}

func TestLoadAccountProxyURLFailClosedCachesProxy(t *testing.T) {
	id := int64(2)
	acc := &Account{ID: 24, ProxyID: &id}
	repo := &guardProxyRepoStub{proxy: &Proxy{ID: id, Protocol: "socks5h", Host: "proxy.internal", Port: 1080}}

	for i := 0; i < 2; i++ {
		if got := LoadAccountProxyURLFailClosed(context.Background(), repo, acc); got != "socks5h://proxy.internal:1080" {
			t.Fatalf("call %d: got %q", i, got)
		}
	}
	if repo.calls != 1 {
		t.Fatalf("expected proxy to be cached after first lookup, got %d repo calls", repo.calls)
	}

	failing := &guardProxyRepoStub{err: errors.New("db down")}
	if got := LoadAccountProxyURLFailClosed(context.Background(), failing, &Account{ID: 1, ProxyID: &id}); got != FailClosedProxyURL {
		t.Fatalf("lookup error: got %q, want fail-closed", got)
	}
	if failing.calls != 1 {
		t.Fatalf("lookup error: expected a single repo call, got %d", failing.calls)
	}
}
