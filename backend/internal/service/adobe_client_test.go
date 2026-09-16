//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

func newTestAdobeClientCache() *adobeClientCache {
	return &adobeClientCache{newClient: func(string) *adobe.Client {
		return adobe.NewClient(adobe.ClientConfig{})
	}}
}

func TestAdobeClientCacheIsolatesCookieJarByAccount(t *testing.T) {
	cache := newTestAdobeClientCache()

	proxyID := int64(9)
	proxy := &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 8080}
	accountA := &Account{ID: 1, ProxyID: &proxyID, Proxy: proxy}
	accountB := &Account{ID: 2, ProxyID: &proxyID, Proxy: proxy}

	clientA := cache.clientForAccount(accountA)
	clientB := cache.clientForAccount(accountB)
	require.NotSame(t, clientA, clientB, "same proxy must not share a CookieJar across accounts")
	require.Same(t, clientA, cache.clientForAccount(accountA), "same account+proxy should reuse the client")
}

// 没有账号 ID 时不按代理地址共用 Client：不同的临时账号不能串 jar。
func TestAdobeClientCacheDoesNotCacheAccountsWithoutID(t *testing.T) {
	cache := newTestAdobeClientCache()

	require.NotSame(t, cache.clientForAccount(nil), cache.clientForAccount(nil))
	require.NotSame(t, cache.clientForAccount(&Account{}), cache.clientForAccount(&Account{}))
	require.Empty(t, cache.clients)
}

// 管理员重新导出 cookie 或改代理后必须换新 Client，旧 jar 里的 ims_sid 不能继续发出去；
// 同一账号始终只占一个条目。
func TestAdobeClientCacheRebuildsOnCookieOrProxyChange(t *testing.T) {
	cache := newTestAdobeClientCache()
	account := &Account{ID: 5, Credentials: map[string]any{"cookie": "aux_sid=old"}}

	first := cache.clientForAccount(account)
	account.Credentials = map[string]any{"cookie": "aux_sid=new"}
	second := cache.clientForAccount(account)
	require.NotSame(t, first, second, "cookie change must rebuild the client")

	proxyID := int64(3)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "10.0.0.1", Port: 3128}
	third := cache.clientForAccount(account)
	require.NotSame(t, second, third, "proxy change must rebuild the client")
	require.Same(t, third, cache.clientForAccount(account))
	require.Len(t, cache.clients, 1, "an account keeps a single cache entry")
}

func TestAdobeClientCacheEvictsIdleEntries(t *testing.T) {
	cache := newTestAdobeClientCache()
	stale := &Account{ID: 7}
	fresh := &Account{ID: 8}

	cache.clientForAccount(stale)
	cache.mu.Lock()
	cache.clients[stale.ID].lastUsed = time.Now().Add(-adobeClientIdleTTL - time.Minute)
	cache.lastSweep = time.Time{}
	cache.mu.Unlock()

	cache.clientForAccount(fresh)
	require.NotContains(t, cache.clients, stale.ID)
	require.Contains(t, cache.clients, fresh.ID)
}
