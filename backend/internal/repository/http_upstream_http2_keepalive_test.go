package repository

import (
	"context"
	"crypto/tls"
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

// 长流 / OpenAI 上游改走 HTTP/2 后，池化连接被代理/NAT 静默掐断会成为“死连接”：
// 两端都以为连接存活，请求落上去会挂到 TCP 重传超时（分钟级）才失败。Go 的
// http2.Transport 默认 ReadIdleTimeout=0（不发健康 PING），无法检测这种死连接。
// 必须显式启用主动 PING 探测，让死连接被提前剔除，而不是只靠 ResponseHeaderTimeout
// 事后兜底。
func TestEnableHTTP2KeepAlive_EnablesPingHealthCheck(t *testing.T) {
	for _, tc := range []struct {
		mode            string
		readIdleTimeout time.Duration
		pingTimeout     time.Duration
	}{
		{upstreamProtocolModeOpenAIH2, 15 * time.Second, 15 * time.Second},
		{upstreamProtocolModeLongStreamH2, 10 * time.Second, 5 * time.Second},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			tr := &http.Transport{}
			h2, err := enableHTTP2KeepAlive(tr, tc.mode)
			require.NoError(t, err)
			require.NotNil(t, h2, "必须返回已配置的 *http2.Transport")
			require.Equal(t, tc.readIdleTimeout, h2.ReadIdleTimeout)
			require.Equal(t, tc.pingTimeout, h2.PingTimeout, "各模式应使用独立的 PING 应答期限")
			requireHTTP2Configured(t, tr, "http2 必须已挂到底层 http.Transport 上")
		})
	}
}

// long_stream_h2 模式构建的 Transport 必须带上 H2 PING 健康探测，从源头剔除死连接。
func TestBuildUpstreamTransport_LongStreamH2_EnablesPingHealthCheck(t *testing.T) {
	tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, upstreamProtocolModeLongStreamH2)
	require.NoError(t, err)
	require.True(t, tr.ForceAttemptHTTP2, "long_stream_h2 必须启用 HTTP/2")
	requireHTTP2Configured(t, tr, "long_stream_h2 必须显式配置 http2 以启用 ReadIdleTimeout")
}

// 默认、Grok 和显式 H1 模式不应主动启用 HTTP/2 保活，避免影响其他平台的传输策略。
func TestBuildUpstreamTransport_NonHTTP2_NotEagerlyConfigured(t *testing.T) {
	for _, mode := range []string{upstreamProtocolModeDefault, upstreamProtocolModeGrok, upstreamProtocolModeOpenAIH1, upstreamProtocolModeOpenAIH1Fallback} {
		t.Run(mode, func(t *testing.T) {
			tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, mode)
			require.NoError(t, err)
			require.Nil(t, tr.Protocols, "非 H2 模式不应主动配置 http2 keepalive")
			require.Nil(t, tr.TLSNextProto["h2"], "非 H2 模式不应主动配置 http2 keepalive")
		})
	}
}

// 使用同时支持 H1/H2 的 TLS 上游核对实际协议，确保保活配置不会改变其他模式的协商行为。
func TestBuildUpstreamTransport_NegotiatesExpectedProtocol(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	for _, tc := range []struct {
		mode      string
		wantMajor int
	}{
		{upstreamProtocolModeOpenAIH2, 2},
		{upstreamProtocolModeLongStreamH2, 2},
		{upstreamProtocolModeDefault, 1},
		{upstreamProtocolModeGrok, 1},
		{upstreamProtocolModeOpenAIH1, 1},
		{upstreamProtocolModeOpenAIH1Fallback, 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), nil, tc.mode)
			require.NoError(t, err)
			defer tr.CloseIdleConnections()
			roots := x509.NewCertPool()
			roots.AddCert(srv.Certificate())
			if tr.TLSClientConfig == nil {
				tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			}
			tr.TLSClientConfig.RootCAs = roots
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			resp, err := tr.RoundTrip(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, tc.wantMajor, resp.ProtoMajor)
		})
	}
}

// 死连接在经 HTTP 代理（CONNECT 隧道）时最高发，这是带 proxy 账号的真实生产路径：
// 显式 http2 配置须与 Transport.Proxy 同时正确生效，不能相互干扰。
func TestBuildUpstreamTransport_HTTP2_WithHTTPProxy_EnablesKeepAlive(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	for _, mode := range []string{upstreamProtocolModeOpenAIH2, upstreamProtocolModeLongStreamH2} {
		t.Run(mode, func(t *testing.T) {
			tr, err := buildUpstreamTransport(http2KeepAliveTestPoolSettings(), proxyURL, mode)
			require.NoError(t, err)
			require.True(t, tr.ForceAttemptHTTP2)
			requireHTTP2Configured(t, tr, "经代理的 H2 模式也必须启用 http2 keepalive")
			require.NotNil(t, tr.Proxy, "HTTP 代理仍须通过 Transport.Proxy 生效")
		})
	}
}
