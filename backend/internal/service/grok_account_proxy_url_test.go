//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGrokOAuthService_AccountProxyURL_PrefersHydratedProxy(t *testing.T) {
	t.Parallel()

	proxyID := int64(99)
	groupID := int64(5)
	svc := NewGrokOAuthService(nil, nil)

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

	url, err = svc.accountProxyURL(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "", url)
}

func TestGrokOAuthService_AccountProxyURL_EmptyPlaceholderFallsBackToRepo(t *testing.T) {
	t.Parallel()
	proxyID := int64(41)
	stub := &grokCredentialProxyRepoStub{err: errors.New("database temporarily unavailable")}
	svc := NewGrokOAuthService(stub, &grokOAuthClientStub{})
	defer svc.Stop()
	account := &Account{ID: 1, ProxyID: &proxyID, Proxy: &Proxy{}}
	_, err := svc.accountProxyURL(context.Background(), account)
	require.Error(t, err)
	require.Equal(t, "GROK_OAUTH_PROXY_LOOKUP_FAILED", infraerrors.Reason(err))
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
