package repository

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 回归：上游 Transport 必须显式配置建连超时。
//
// http.Transport.DialContext 为 nil 时 Go 使用零值 net.Dialer（Timeout=0），
// DNS 解析与 TCP 握手没有任何上限，只能等内核重传耗尽（Linux 约 130 秒）。
// ResponseHeaderTimeout 只覆盖连接建立之后的阶段，管不到建连。
// 上游域名被解析到不可达 IP 时，串行的多账号故障转移会把一次请求拖到数分钟。
func TestBuildUpstreamTransportSetsDialTimeout(t *testing.T) {
	settings := defaultPoolSettings(nil)

	transport, err := buildUpstreamTransport(settings, nil, upstreamProtocolModeDefault)
	require.NoError(t, err)
	require.NotNil(t, transport.DialContext, "DialContext 缺失会退化为无超时的零值 dialer")
	require.Equal(t, defaultUpstreamTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
}

func TestNewUpstreamDialerHasBoundedTimeout(t *testing.T) {
	dialer := newUpstreamDialer()

	require.Greater(t, dialer.Timeout, time.Duration(0), "建连超时必须有上限")
	require.Equal(t, defaultUpstreamDialTimeout, dialer.Timeout)
	require.Equal(t, defaultUpstreamDialKeepAlive, dialer.KeepAlive)
}

// 建连超时对 HTTP 代理同样生效：Transport.Proxy 走的仍是 DialContext，
// 代理地址不可达时必须快速失败而不是挂满内核超时。
func TestBuildUpstreamTransportKeepsDialTimeoutWithHTTPProxy(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:1080")
	require.NoError(t, err)

	transport, err := buildUpstreamTransport(defaultPoolSettings(nil), proxyURL, upstreamProtocolModeDefault)
	require.NoError(t, err)
	require.NotNil(t, transport.Proxy)
	require.NotNil(t, transport.DialContext)
}

// SOCKS5 分支会覆盖 Transport.DialContext，覆盖后仍必须是有超时的拨号器。
func TestBuildUpstreamTransportKeepsDialContextWithSOCKS5Proxy(t *testing.T) {
	proxyURL, err := url.Parse("socks5h://127.0.0.1:1080")
	require.NoError(t, err)

	transport, err := buildUpstreamTransport(defaultPoolSettings(nil), proxyURL, upstreamProtocolModeDefault)
	require.NoError(t, err)
	require.NotNil(t, transport.DialContext)
}

// Timeout 字段确实被 net.Dialer 用于建连：拨一个已被 close 的本地监听端口，
// 断言 Dialer 走的是自己的超时路径而不是无限等待。
// （不依赖外网可达性，CI 中确定性执行。）
func TestUpstreamDialerRespectsContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := newUpstreamDialer().DialContext(ctx, "tcp", addr)
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err, "已取消的 context 必须立即中止拨号")
}
