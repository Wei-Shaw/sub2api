//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

func TestAdobeClientCacheIsolatesCookieJarByAccount(t *testing.T) {
	cache := &adobeClientCache{}
	cache.newClient = func(string) *adobe.Client {
		return adobe.NewClient(adobe.ClientConfig{})
	}

	proxyID := int64(9)
	proxy := &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 8080}
	accountA := &Account{ID: 1, ProxyID: &proxyID, Proxy: proxy}
	accountB := &Account{ID: 2, ProxyID: &proxyID, Proxy: proxy}

	clientA := cache.clientForAccount(accountA)
	clientB := cache.clientForAccount(accountB)
	require.NotSame(t, clientA, clientB, "same proxy must not share a CookieJar across accounts")
	require.Same(t, clientA, cache.clientForAccount(accountA), "same account+proxy should reuse the client")
}

func TestAdobeClientCacheNilAccountKeysByProxy(t *testing.T) {
	cache := &adobeClientCache{}
	cache.newClient = func(string) *adobe.Client {
		return adobe.NewClient(adobe.ClientConfig{})
	}

	first := cache.clientForAccount(nil)
	second := cache.clientForAccount(nil)
	require.Same(t, first, second)
}
