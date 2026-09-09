package repository

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// streamResetTestTransport 同时实现 http.RoundTripper 与 idleConnectionCloser，
// 用于断言“重放仍失败时剔除空闲连接”这一行为真的发生了。
type streamResetTestTransport struct {
	roundTrip          func(*http.Request) (*http.Response, error)
	closeIdleCallCount int
}

func (t *streamResetTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.roundTrip(req)
}

func (t *streamResetTestTransport) CloseIdleConnections() { t.closeIdleCallCount++ }

// peerStreamReset 构造与生产日志同形的错误：对端在响应头之前重置了这条流。
func peerStreamReset(streamID uint32, code http2.ErrCode) error {
	return http2.StreamError{StreamID: streamID, Code: code, Cause: errors.New("received from peer")}
}

func openAIH2TestService(t *testing.T) *httpUpstreamService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIHTTP2.Enabled = true
	svc, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
	require.True(t, ok)
	return svc
}

// seedOpenAIH2Entry 预置一个 openai_h2 模式的客户端条目，让 Do 走到重试逻辑而不真正建连。
func seedOpenAIH2Entry(t *testing.T, svc *httpUpstreamService, accountID int64, transport http.RoundTripper) {
	t.Helper()
	isolation := svc.getIsolationMode()
	profile := service.HTTPUpstreamProfileOpenAI
	proxyKey := directProxyKey
	protocolMode := svc.resolveProtocolMode(profile, proxyKey, nil)
	require.Equal(t, upstreamProtocolModeOpenAIH2, protocolMode,
		"用例前提：直连 + OpenAI profile 必须解析为 openai_h2")
	settings := svc.applyProfilePoolSettings(svc.resolvePoolSettings(isolation, 1), profile)
	svc.clients[buildCacheKey(isolation, proxyKey, accountID, protocolMode)] = &upstreamClientEntry{
		client:       &http.Client{Transport: transport},
		proxyKey:     proxyKey,
		poolKey:      buildPoolKey(settings, protocolMode),
		protocolMode: protocolMode,
	}
}

func newUpstreamPostRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://chatgpt.com/backend-api/codex/responses", bytes.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody, "bytes.Reader 构造的请求必须自带 GetBody，否则重放不成立")
	return req
}

// 真实 HTTP/2 对端在响应头之前重置流时，错误必须能被识别出来。
//
// 这条用例锁的是整条链路上最脆弱的一环：go1.27 起实际生效的是 net/http 内建的
// HTTP/2 实现，它产生的 StreamError 位于 internal 包、类型未导出，只有其自带的
// As 方法才能转换成 x/net/http2.StreamError。一旦该转换失效，类型断言会静默返回
// false，重试跟着静默消失——而单测若只用手工构造的错误是发现不了的。
func TestIsUpstreamHTTP2StreamReset_RealPeerReset(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// 在写响应头之前中止 → 服务端发 RST_STREAM，客户端 Do 直接返回错误。
		panic(http.ErrAbortHandler)
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
	resp, err := (&http.Client{Transport: tr}).Do(req) //nolint:bodyclose // 出错路径没有响应体
	require.Nil(t, resp)
	require.Error(t, err)
	// 只有 HTTP/2 才会产生 stream error，这同时确认了用例确实跑在 h2 上。
	require.Contains(t, err.Error(), "stream error", "用例前提：必须协商到 HTTP/2")
	require.True(t, isUpstreamHTTP2StreamReset(err), "真实对端重置必须被识别，实际错误：%v", err)
}

func TestIsUpstreamHTTP2StreamReset_Classification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			// 现场原始错误：
			// Post "https://chatgpt.com/backend-api/codex/responses":
			// stream error: stream ID 35; PROTOCOL_ERROR; received from peer
			name: "对端 PROTOCOL_ERROR 需要重试",
			err:  peerStreamReset(35, http2.ErrCodeProtocol),
			want: true,
		},
		{
			name: "对端 INTERNAL_ERROR 需要重试",
			err:  peerStreamReset(7, http2.ErrCodeInternal),
			want: true,
		},
		{
			// REFUSED_STREAM 已由 Go 传输层自己重试（canRetryError 覆盖），
			// 这里再补一次会变成重复重试。
			name: "REFUSED_STREAM 已由 Go 自行重试，不重复处理",
			err:  peerStreamReset(9, http2.ErrCodeRefusedStream),
			want: false,
		},
		{name: "空错误", err: nil, want: false},
		{name: "非 HTTP/2 错误", err: errors.New("connection refused"), want: false},
		{name: "超时错误不属于流重置", err: context.DeadlineExceeded, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isUpstreamHTTP2StreamReset(tc.err))
		})
	}
}

func TestShouldRetryUpstreamStreamReset_Gating(t *testing.T) {
	resetErr := peerStreamReset(35, http2.ErrCodeProtocol)

	t.Run("openai_h2 下的流重置需要重试", func(t *testing.T) {
		require.True(t, shouldRetryUpstreamStreamReset(
			upstreamProtocolModeOpenAIH2, newUpstreamPostRequest(t, []byte(`{"model":"gpt-5.4"}`)), resetErr))
	})

	t.Run("long_stream_h2 同样需要重试", func(t *testing.T) {
		require.True(t, shouldRetryUpstreamStreamReset(
			upstreamProtocolModeLongStreamH2, newUpstreamPostRequest(t, []byte(`{}`)), resetErr))
	})

	t.Run("非显式 h2 模式不介入", func(t *testing.T) {
		for _, mode := range []string{
			upstreamProtocolModeDefault,
			upstreamProtocolModeOpenAIH1,
			upstreamProtocolModeOpenAIH1Fallback,
			upstreamProtocolModeGrok,
		} {
			require.False(t, shouldRetryUpstreamStreamReset(
				mode, newUpstreamPostRequest(t, []byte(`{}`)), resetErr), "mode=%s", mode)
		}
	})

	t.Run("请求体不可重放时不重试", func(t *testing.T) {
		req := newUpstreamPostRequest(t, []byte(`{}`))
		req.GetBody = nil
		require.False(t, shouldRetryUpstreamStreamReset(upstreamProtocolModeOpenAIH2, req, resetErr))
	})

	t.Run("上下文已取消时不重试", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := newUpstreamPostRequest(t, []byte(`{}`)).WithContext(ctx)
		require.False(t, shouldRetryUpstreamStreamReset(upstreamProtocolModeOpenAIH2, req, resetErr))
	})
}

// 单条流被对端重置属于传输层抖动：必须在同一个账号上原地重放，而不是直接换账号
// 故障转移——后者会拿另一个账号的额度和限流预算去补一次网络抖动。
func TestHTTPUpstreamDo_RetriesStreamResetOnSameAccount(t *testing.T) {
	svc := openAIH2TestService(t)
	const accountID int64 = 7781
	payload := []byte(`{"model":"gpt-5.4","input":"hello"}`)

	var seenBodies [][]byte
	transport := &streamResetTestTransport{}
	transport.roundTrip = func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		seenBodies = append(seenBodies, body)
		if len(seenBodies) == 1 {
			return nil, peerStreamReset(35, http2.ErrCodeProtocol)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"resp-ok"}`))),
			Request:    req,
		}, nil
	}
	seedOpenAIH2Entry(t, svc, accountID, transport)

	req := newUpstreamPostRequest(t, payload)
	req = req.WithContext(service.WithHTTPUpstreamProfile(req.Context(), service.HTTPUpstreamProfileOpenAI))

	resp, err := svc.Do(req, "", accountID, 1)
	require.NoError(t, err, "重放成功后调用方不应看到错误")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Len(t, seenBodies, 2, "必须恰好重放一次")
	require.Equal(t, payload, seenBodies[0])
	require.Equal(t, payload, seenBodies[1], "重放必须携带完整且一致的请求体")
	require.Zero(t, transport.closeIdleCallCount, "重放成功时不应丢弃连接池")
}

// 重放仍失败说明问题多半在连接级状态上：必须剔除空闲连接，让下一个请求换新连接，
// 否则后续请求会继续落在同一条已被上游标记的连接上。
func TestHTTPUpstreamDo_StreamResetRetryFailureEvictsIdleConnections(t *testing.T) {
	svc := openAIH2TestService(t)
	const accountID int64 = 7782

	calls := 0
	transport := &streamResetTestTransport{}
	transport.roundTrip = func(req *http.Request) (*http.Response, error) {
		calls++
		_, _ = io.Copy(io.Discard, req.Body)
		return nil, peerStreamReset(uint32(33+2*calls), http2.ErrCodeProtocol) //nolint:gosec // G115: 测试用小整数
	}
	seedOpenAIH2Entry(t, svc, accountID, transport)

	req := newUpstreamPostRequest(t, []byte(`{"model":"gpt-5.4"}`))
	req = req.WithContext(service.WithHTTPUpstreamProfile(req.Context(), service.HTTPUpstreamProfileOpenAI))

	resp, err := svc.Do(req, "", accountID, 1) //nolint:bodyclose // 出错路径没有响应体
	require.Nil(t, resp)
	require.Error(t, err)
	require.True(t, isUpstreamHTTP2StreamReset(err), "最终仍应透出流重置错误，便于上层分类")
	require.Equal(t, 2, calls, "最多重放一次")
	require.Equal(t, 1, transport.closeIdleCallCount, "重放失败后必须剔除空闲连接")
}

// 重试只针对显式 HTTP/2 的上游路径，不得波及 Claude/Gemini 等其它上游的热路径。
func TestHTTPUpstreamDo_DoesNotRetryOutsideExplicitHTTP2(t *testing.T) {
	svc := openAIH2TestService(t)
	const accountID int64 = 7783

	isolation := svc.getIsolationMode()
	profile := service.HTTPUpstreamProfileDefault
	protocolMode := svc.resolveProtocolMode(profile, directProxyKey, nil)
	require.Equal(t, upstreamProtocolModeDefault, protocolMode)
	settings := svc.applyProfilePoolSettings(svc.resolvePoolSettings(isolation, 1), profile)

	calls := 0
	transport := &streamResetTestTransport{}
	transport.roundTrip = func(req *http.Request) (*http.Response, error) {
		calls++
		_, _ = io.Copy(io.Discard, req.Body)
		return nil, peerStreamReset(35, http2.ErrCodeProtocol)
	}
	svc.clients[buildCacheKey(isolation, directProxyKey, accountID, protocolMode)] = &upstreamClientEntry{
		client:       &http.Client{Transport: transport},
		proxyKey:     directProxyKey,
		poolKey:      buildPoolKey(settings, protocolMode),
		protocolMode: protocolMode,
	}

	req := newUpstreamPostRequest(t, []byte(`{"model":"claude"}`))
	resp, err := svc.Do(req, "", accountID, 1) //nolint:bodyclose // 出错路径没有响应体
	require.Nil(t, resp)
	require.Error(t, err)
	require.Equal(t, 1, calls, "非显式 h2 路径不得重试")
	require.Zero(t, transport.closeIdleCallCount)
}
