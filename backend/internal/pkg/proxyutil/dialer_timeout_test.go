package proxyutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var errStub = errors.New("stub dial")

// 回归：SOCKS5 分支覆盖了调用方在 Transport 上设置的 DialContext，
// 底层 forward dialer 必须自带建连超时。proxy.Direct 是零值 net.Dialer，
// 代理不可达时会一直卡到内核 TCP 重传耗尽（Linux 约 130 秒）。
func TestSOCKS5ForwardDialerHasBoundedTimeout(t *testing.T) {
	require.Greater(t, socks5ForwardDialer.Timeout, time.Duration(0))
	require.Equal(t, socks5DialTimeout, socks5ForwardDialer.Timeout)
	require.Equal(t, socks5DialKeepAlive, socks5ForwardDialer.KeepAlive)
}

func TestConfigureTransportProxySOCKS5SetsDialContext(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			proxyURL, err := url.Parse(scheme + "://127.0.0.1:1080")
			require.NoError(t, err)

			transport := &http.Transport{}
			require.NoError(t, ConfigureTransportProxy(transport, proxyURL))
			require.NotNil(t, transport.DialContext)
			require.Nil(t, transport.Proxy, "SOCKS5 不应设置 Transport.Proxy")
		})
	}
}

// HTTP 代理走 Transport.Proxy，不得覆盖调用方设置的 DialContext。
func TestConfigureTransportProxyHTTPPreservesDialContext(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)

	called := false
	transport := &http.Transport{}
	transport.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) {
		called = true
		return nil, errStub
	}

	require.NoError(t, ConfigureTransportProxy(transport, proxyURL))
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.DialContext)

	_, _ = transport.DialContext(context.Background(), "tcp", "127.0.0.1:1")
	require.True(t, called, "HTTP 代理分支不应替换调用方的 DialContext")
}
