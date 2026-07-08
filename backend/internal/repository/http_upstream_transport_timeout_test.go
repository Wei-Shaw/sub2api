package repository

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func timeoutTestPoolSettings() poolSettings {
	return poolSettings{
		maxIdleConns:          10,
		maxIdleConnsPerHost:   5,
		maxConnsPerHost:       10,
		idleConnTimeout:       90 * time.Second,
		responseHeaderTimeout: time.Minute,
	}
}

// 上游 Transport 必须带建连阶段超时（拨号 + TLS 握手），否则上游/代理黑洞时
// 连接会无限挂起，只能靠客户端断开兜底。这些超时只约束建连，不影响流式长响应。
func TestBuildUpstreamTransport_SetsConnectionEstablishmentTimeouts(t *testing.T) {
	tr, err := buildUpstreamTransport(timeoutTestPoolSettings(), nil, upstreamProtocolModeDefault)
	require.NoError(t, err)

	require.NotNil(t, tr.DialContext, "直连必须使用带超时的 DialContext")
	require.Equal(t, upstreamTLSHandshakeTimeout, tr.TLSHandshakeTimeout, "TLS 握手必须有独立超时")
	// 设置了自定义 DialContext 后 Go 会静默禁用自动 HTTP/2，
	// 必须显式打开以保持 Claude/Gemini 默认路径原有的 H2 行为。
	require.True(t, tr.ForceAttemptHTTP2, "默认直连路径必须保持自动 HTTP/2")
}

func TestBuildUpstreamTransport_HTTPProxyKeepsAutoHTTP2(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)

	tr, err := buildUpstreamTransport(timeoutTestPoolSettings(), proxyURL, upstreamProtocolModeDefault)
	require.NoError(t, err)

	require.NotNil(t, tr.Proxy, "HTTP 代理应通过 Transport.Proxy 配置")
	require.NotNil(t, tr.DialContext, "连接代理的 TCP 拨号也要有超时")
	require.Equal(t, upstreamTLSHandshakeTimeout, tr.TLSHandshakeTimeout)
	require.True(t, tr.ForceAttemptHTTP2, "HTTP 代理(CONNECT)路径此前即为自动 H2，保持不变")
}

func TestBuildUpstreamTransport_SOCKS5KeepsHTTP1(t *testing.T) {
	proxyURL, err := url.Parse("socks5://127.0.0.1:1080")
	require.NoError(t, err)

	tr, err := buildUpstreamTransport(timeoutTestPoolSettings(), proxyURL, upstreamProtocolModeDefault)
	require.NoError(t, err)

	require.NotNil(t, tr.DialContext, "SOCKS5 经由 DialContext 建立隧道")
	// SOCKS5 此前就因自定义 DialContext 走 HTTP/1.1，保持行为不变。
	require.False(t, tr.ForceAttemptHTTP2, "SOCKS5 路径不应改变协议行为")
}

func TestBuildUpstreamTransport_OpenAIH1ModesStillDisableHTTP2(t *testing.T) {
	for _, mode := range []string{upstreamProtocolModeOpenAIH1, upstreamProtocolModeOpenAIH1Fallback} {
		tr, err := buildUpstreamTransport(timeoutTestPoolSettings(), nil, mode)
		require.NoError(t, err)
		require.False(t, tr.ForceAttemptHTTP2, "mode=%s 必须禁用 H2", mode)
		require.NotNil(t, tr.TLSNextProto, "mode=%s 必须清空 TLSNextProto 强制 HTTP/1.1", mode)
		require.Empty(t, tr.TLSNextProto)
	}
}

func TestBuildUpstreamTransportWithTLSFingerprint_SetsDialTimeouts(t *testing.T) {
	tr, err := buildUpstreamTransportWithTLSFingerprint(timeoutTestPoolSettings(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, tr.DialTLSContext, "指纹路径使用自定义 DialTLSContext（内部自带建连包络超时）")
	require.False(t, tr.ForceAttemptHTTP2)
}
