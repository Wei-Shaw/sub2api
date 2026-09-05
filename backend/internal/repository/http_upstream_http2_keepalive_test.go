package repository

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func http2KeepAliveTestPoolSettings() poolSettings {
	return poolSettings{
		maxIdleConns:          10,
		maxIdleConnsPerHost:   5,
		maxConnsPerHost:       10,
		idleConnTimeout:       90 * time.Second,
		responseHeaderTimeout: time.Minute,
	}
}

// requireHTTP2Configured 断言 http2 已显式挂到 http.Transport 上。
// x/net/http2 在 go1.27 && !http2legacy 下是标准库 HTTP/2 的包装：ConfigureTransports 通过
// Transport.RegisterProtocol("http/2") 注册配置并打开 Protocols.HTTP2（TLSNextProto 不承载 h2 入口），
// ReadIdleTimeout/PingTimeout 在建连时映射为 http.HTTP2Config.SendPingTimeout/PingTimeout。
func requireHTTP2Configured(t *testing.T, tr *http.Transport, msg string) {
	t.Helper()
	require.NotNil(t, tr.Protocols, msg)
	require.True(t, tr.Protocols.HTTP2(), msg)
}

// Codex/OpenAI 上游改走 HTTP/2 后，池化连接被代理/NAT 静默掐断会成为“死连接”：
// 两端都以为连接存活，请求落上去会挂到 TCP 重传超时（分钟级）才失败。Go 的
// http2.Transport 默认 ReadIdleTimeout=0（不发健康 PING），无法检测这种死连接。
// 必须显式启用主动 PING 探测，让死连接被提前剔除，而不是只靠 ResponseHeaderTimeout
// 事后兜底。
func TestEnableOpenAIHTTP2KeepAlive_EnablesPingHealthCheck(t *testing.T) {
	tr := &http.Transport{}

	h2, err := enableOpenAIHTTP2KeepAlive(tr)
	require.NoError(t, err)
	require.NotNil(t, h2, "必须返回已配置的 *http2.Transport")

	require.Positive(t, h2.ReadIdleTimeout, "必须启用空闲 PING 探测以剔除死连接")
	require.Equal(t, openAIHTTP2ReadIdleTimeout, h2.ReadIdleTimeout)
	require.Equal(t, openAIHTTP2PingTimeout, h2.PingTimeout, "PING 无响应必须有超时判定")
	requireHTTP2Configured(t, tr, "http2 必须已挂到底层 http.Transport 上")
}

// openai_h2 模式构建的 Transport 必须带上 H2 PING 健康探测，从源头剔除死连接。
func TestBuildUpstreamTransport_OpenAIH2_EnablesPingHealthCheck(t *testing.T) {
	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeOpenAIH2)
	require.NoError(t, err)
	require.True(t, tr.ForceAttemptHTTP2, "openai_h2 必须启用 HTTP/2")
	requireHTTP2Configured(t, tr, "openai_h2 必须显式配置 http2 以启用 ReadIdleTimeout")
}

// 非 H2 模式（default/h1）不应因本次改动被误配置：default 走 Go 自动 H2（惰性配置，
// 构建时 Protocols/TLSNextProto 仍为空），h1 模式显式禁用 H2。避免波及 Claude/Gemini 热路径。
func TestBuildUpstreamTransport_NonOpenAIH2_NotEagerlyConfigured(t *testing.T) {
	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeDefault)
	require.NoError(t, err)
	require.Nil(t, tr.Protocols, "default 模式不应在构建期主动配置 http2 keepalive")
	require.Nil(t, tr.TLSNextProto["h2"], "default 模式不应在构建期主动配置 http2 keepalive")
}

// openai_h2 模式构建的 Transport 必须真正以 HTTP/2 与上游通信，PING 健康探测才有载体：
// 自定义 DialContext 下 Go 不会自动启用 H2，全靠 enableOpenAIHTTP2KeepAlive 的显式配置。
func TestBuildUpstreamTransport_OpenAIH2_NegotiatesHTTP2(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeOpenAIH2)
	require.NoError(t, err)
	defer tr.CloseIdleConnections()
	require.NotNil(t, tr.TLSClientConfig)
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	tr.TLSClientConfig.RootCAs = roots

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, resp.ProtoMajor, "openai_h2 必须协商到 HTTP/2")
}

// 死连接在经 HTTP 代理（CONNECT 隧道）时最高发，这是带 proxy 账号的真实生产路径：
// 显式 http2 配置须与 Transport.Proxy 同时正确生效，不能相互干扰。
func TestBuildUpstreamTransport_OpenAIH2_WithHTTPProxy_EnablesKeepAlive(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)

	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), proxyURL, upstreamProtocolModeOpenAIH2)
	require.NoError(t, err)
	require.True(t, tr.ForceAttemptHTTP2)
	requireHTTP2Configured(t, tr, "经代理的 openai_h2 也必须启用 http2 keepalive")
	require.NotNil(t, tr.Proxy, "HTTP 代理仍须通过 Transport.Proxy 生效")
}
