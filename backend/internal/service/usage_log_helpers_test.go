package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsedProxyID(t *testing.T) {
	t.Parallel()

	proxyID := int64(42)
	zeroID := int64(0)
	proxy := &Proxy{ID: proxyID, Name: "p"}

	t.Run("nil_account", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, usedProxyID(nil))
	})

	t.Run("no_proxy", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, usedProxyID(&Account{}))
	})

	t.Run("proxy_id_without_object", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, usedProxyID(&Account{ProxyID: &proxyID}))
	})

	t.Run("zero_proxy_id", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, usedProxyID(&Account{ProxyID: &zeroID, Proxy: proxy}))
	})

	t.Run("uses_bound_proxy", func(t *testing.T) {
		t.Parallel()
		got := usedProxyID(&Account{ProxyID: &proxyID, Proxy: proxy})
		require.NotNil(t, got)
		require.Equal(t, proxyID, *got)
	})

	t.Run("skips_custom_base_url", func(t *testing.T) {
		t.Parallel()
		got := usedProxyID(&Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			ProxyID:  &proxyID,
			Proxy:    proxy,
			Extra: map[string]any{
				"custom_base_url_enabled": true,
				"custom_base_url":         "https://relay.example",
			},
		})
		require.Nil(t, got)
	})

	t.Run("keeps_proxy_when_custom_base_url_empty", func(t *testing.T) {
		t.Parallel()
		got := usedProxyID(&Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			ProxyID:  &proxyID,
			Proxy:    proxy,
			Extra: map[string]any{
				"custom_base_url_enabled": true,
			},
		})
		require.NotNil(t, got)
		require.Equal(t, proxyID, *got)
	})
}
