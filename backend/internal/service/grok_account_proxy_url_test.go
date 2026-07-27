//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokOAuthService_AccountProxyURL_PrefersHydratedProxy(t *testing.T) {
	t.Parallel()

	proxyID := int64(99)
	groupID := int64(5)
	svc := NewGrokOAuthService(nil, nil)

	// 池账号：Proxy 已 hydrate，ProxyID 仍为 nil —— 必须走 hydrate 结果，不能查库。
	account := &Account{
		ID:           42,
		ProxyGroupID: &groupID,
		Proxy: &Proxy{
			ID:       7,
			Protocol: "http",
			Host:     "pool-member.example.com",
			Port:     8080,
		},
	}
	url, err := svc.accountProxyURL(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "http://pool-member.example.com:8080", url)

	// 显式单代理 hydrate：同样优先 Proxy 字段。
	account2 := &Account{
		ID:      1,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "socks5",
			Host:     "single.example.com",
			Port:     1080,
		},
	}
	url, err = svc.accountProxyURL(context.Background(), account2)
	require.NoError(t, err)
	require.Equal(t, "socks5://single.example.com:1080", url)

	// nil account
	url, err = svc.accountProxyURL(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "", url)
}

func TestAccountHasConfiguredProxy_CoversGroup(t *testing.T) {
	t.Parallel()
	groupID := int64(3)
	proxyID := int64(1)

	require.False(t, accountHasConfiguredProxy(nil))
	require.False(t, accountHasConfiguredProxy(&Account{}))
	require.True(t, accountHasConfiguredProxy(&Account{ProxyID: &proxyID}))
	require.True(t, accountHasConfiguredProxy(&Account{ProxyGroupID: &groupID}))
	require.True(t, accountHasConfiguredProxy(&Account{
		Proxy: &Proxy{ID: 9, Protocol: "http", Host: "x", Port: 1},
	}))
}
