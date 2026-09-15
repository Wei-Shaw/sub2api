//go:build unit

package adobe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/stretchr/testify/require"
)

// header 顺序是浏览器指纹的一部分：Go 的 map 迭代顺序随机，若不显式固定，
// 同一请求每次发出的顺序都不同，本身就是可被识别的特征。
func TestApplyHeaderOrder(t *testing.T) {
	t.Run("按声明顺序发送", func(t *testing.T) {
		req := &fhttp.Request{}
		applyHeaderOrder(req,
			map[string]string{"user-agent": "UA", "accept": "*/*", "origin": "O"},
			[]string{"user-agent", "origin", "accept"})

		require.Equal(t, []string{"user-agent", "origin", "accept"}, req.Header[fhttp.HeaderOrderKey])
		require.Equal(t, []string{"UA"}, req.Header["user-agent"])
	})

	t.Run("顺序里没列到的头补在后面而不是丢弃", func(t *testing.T) {
		req := &fhttp.Request{}
		applyHeaderOrder(req,
			map[string]string{"a": "1", "b": "2", "c": "3"},
			[]string{"a"})

		order := req.Header[fhttp.HeaderOrderKey]
		require.Equal(t, "a", order[0])
		require.Len(t, order, 3)
		require.Equal(t, []string{"2"}, req.Header["b"])
		require.Equal(t, []string{"3"}, req.Header["c"])
	})

	t.Run("顺序里的名字大小写不敏感", func(t *testing.T) {
		req := &fhttp.Request{}
		applyHeaderOrder(req, map[string]string{"User-Agent": "UA"}, []string{"user-agent"})
		require.Equal(t, []string{"UA"}, req.Header["user-agent"])
		require.Equal(t, []string{"user-agent"}, req.Header[fhttp.HeaderOrderKey])
	})

	t.Run("顺序里引用了不存在的头则跳过", func(t *testing.T) {
		req := &fhttp.Request{}
		applyHeaderOrder(req, map[string]string{"a": "1"}, []string{"missing", "a"})
		require.Equal(t, []string{"a"}, req.Header[fhttp.HeaderOrderKey])
	})

	t.Run("无顺序时不丢头", func(t *testing.T) {
		req := &fhttp.Request{}
		applyHeaderOrder(req, map[string]string{"a": "1", "b": "2"}, nil)
		require.Len(t, req.Header[fhttp.HeaderOrderKey], 2)
	})
}

// Identity 里配的 profile 名必须真实存在，否则要到第一次发请求才暴露。
func TestDefaultIdentityTLSProfileExists(t *testing.T) {
	_, ok := profiles.MappedTLSClients[DefaultIdentity.TLSProfile]
	require.True(t, ok, "未知的 tls-client profile: %s", DefaultIdentity.TLSProfile)
}

func TestDefaultIdentityMatchesFireflyCapture(t *testing.T) {
	require.Equal(t, "clio-playground-web", DefaultIdentity.IMSClientID)
	require.Equal(t, "clio-playground-web", DefaultIdentity.FireflyAPIKey)
	require.Equal(t, "https://firefly.adobe.com", DefaultIdentity.Origin)
	require.Equal(t, "https://firefly.adobe.com/", DefaultIdentity.Referer)
	require.Contains(t, DefaultIdentity.IMSRefreshURL, "jslVersion=v2-v0.54.0-3-g58cfcb7")
	require.Contains(t, DefaultIdentity.IMSScope, "firefly_api")
	require.Contains(t, DefaultIdentity.IMSScope, "profile")
	require.Contains(t, DefaultIdentity.IMSScope, "tk_platform")
	require.NotEqual(t, "AdobeID,firefly_api,openid", DefaultIdentity.IMSScope,
		"scope 不能缩回三项，Firefly IMS 需要完整列表")
	require.Equal(t, "SunbreakWebUI1", DefaultIdentity.CreditsAPIKey)
	require.Contains(t, DefaultIdentity.UserAgent, "Chrome/145")
	require.Equal(t, "chrome_146", DefaultIdentity.TLSProfile)
}

func TestIdentityWithDefaults(t *testing.T) {
	filled := Identity{Origin: "https://custom.example"}.withDefaults()
	require.Equal(t, "https://custom.example", filled.Origin, "显式值应保留")
	require.Equal(t, DefaultIdentity.UserAgent, filled.UserAgent, "零值应补齐")
	require.Equal(t, DefaultIdentity.TLSProfile, filled.TLSProfile)
	require.Equal(t, DefaultIdentity.CreditsAPIKey, filled.CreditsAPIKey)
}

func TestImpersonateTransportRejectsUnknownProfile(t *testing.T) {
	transport := NewImpersonateTransport(Identity{TLSProfile: "chrome_does_not_exist"}, "")
	_, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, URL: "https://example.com"})
	require.ErrorContains(t, err, "unknown tls-client profile")
	require.False(t, IsRotatable(err), "配置错误换号也无用")
}

// allowLoopbackDownloads 让直连下载在本测试内可以连 httptest 的回环地址。
func allowLoopbackDownloads(t *testing.T) {
	t.Helper()
	previous := plainTransportDialControl
	plainTransportDialControl = nil
	t.Cleanup(func() { plainTransportDialControl = previous })
}

func TestPlainTransportRoundTrip(t *testing.T) {
	allowLoopbackDownloads(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "*/*", r.Header.Get("accept"))
		w.Header().Set("X-Task-Status", "COMPLETED")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("BODY"))
	}))
	defer server.Close()

	resp, err := NewPlainTransport("").Do(context.Background(), &Request{
		Method:  http.MethodGet,
		URL:     server.URL,
		Headers: map[string]string{"accept": "*/*"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []byte("BODY"), resp.Body)
	// 响应头键统一小写，且 Header() 大小写不敏感。
	require.Equal(t, "COMPLETED", resp.Headers["x-task-status"])
	require.Equal(t, "COMPLETED", resp.Header("X-Task-Status"))
	require.Empty(t, resp.Header("missing"))
}

// 产物直链可以 302 到内网主机名：每一跳都要重新校验，而不只是首个 URL。
func TestPlainTransportRejectsUnsafeRedirects(t *testing.T) {
	allowLoopbackDownloads(t)
	for name, location := range map[string]string{
		"localhost": "https://localhost/secret",
		"ip 字面量":    "https://10.0.0.1/secret",
		"降级到 http":  "http://media.example.com/secret",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, location, http.StatusFound)
			}))
			defer server.Close()

			_, err := NewPlainTransport("").Do(context.Background(), &Request{Method: http.MethodGet, URL: server.URL})
			require.ErrorContains(t, err, "destination is not allowed")
			require.False(t, IsRotatable(err), "目标被策略拒绝，换号也无用")
		})
	}
}

func TestPlainTransportRedirectLimit(t *testing.T) {
	transport := NewPlainTransport("").(*plainTransport)
	_, err := transport.ensureClient()
	require.NoError(t, err)
	via := make([]*http.Request, maxDownloadRedirects)
	next, _ := http.NewRequest(http.MethodGet, "https://media.example.com/next", nil)
	require.ErrorIs(t, transport.checkRedirect(next, via), ErrBlockedDestination)
	require.NoError(t, transport.checkRedirect(next, via[:1]))
}

// 经代理时拨号层看到的是代理地址，改为发请求前本地解析主机名。
func TestPlainTransportProxyResolveGuard(t *testing.T) {
	stubResolver := func(t *testing.T, ip string) {
		t.Helper()
		previous := downloadResolver
		downloadResolver = func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
		}
		t.Cleanup(func() { downloadResolver = previous })
	}

	t.Run("解析到内网被拒且不发请求", func(t *testing.T) {
		stubResolver(t, "10.0.0.1")
		_, err := NewPlainTransport("http://127.0.0.1:1").Do(context.Background(), &Request{
			Method: http.MethodGet, URL: "https://media.example.com/img.png",
		})
		require.ErrorContains(t, err, "destination is not allowed")
		require.False(t, IsRotatable(err))
	})

	t.Run("解析到公网放行", func(t *testing.T) {
		stubResolver(t, "93.184.216.34")
		// 代理地址不可达：放行后应得到代理连接失败，而不是被策略拒绝。
		_, err := NewPlainTransport("http://127.0.0.1:1").Do(context.Background(), &Request{
			Method: http.MethodGet, URL: "https://media.example.com/img.png", Timeout: 2 * time.Second,
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "not allowed")
		require.True(t, IsRotatable(err))
	})
}

func TestPlainTransportInvalidProxy(t *testing.T) {
	_, err := NewPlainTransport("://bad").Do(context.Background(), &Request{
		Method: http.MethodGet, URL: "https://example.com",
	})
	require.ErrorContains(t, err, "invalid proxy url")
}

func TestPlainTransportTimeout(t *testing.T) {
	allowLoopbackDownloads(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := NewPlainTransport("").Do(context.Background(), &Request{
		Method:  http.MethodGet,
		URL:     server.URL,
		Timeout: 10 * time.Millisecond,
	})
	var temporary *UpstreamTemporaryError
	require.True(t, errors.As(err, &temporary))
	require.Equal(t, ErrorTypeTimeout, temporary.ErrorType)
	require.True(t, IsRotatable(err))
}

func TestResponseBodyPreview(t *testing.T) {
	long := make([]byte, maxErrorBodyBytes+50)
	for i := range long {
		long[i] = 'x'
	}
	require.Len(t, (&Response{Body: long}).BodyPreview(), maxErrorBodyBytes)
	require.Equal(t, "short", (&Response{Body: []byte("short")}).BodyPreview())
}

func TestClassifyTransportError(t *testing.T) {
	t.Run("超时", func(t *testing.T) {
		err := classifyTransportError(context.DeadlineExceeded, false)
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, ErrorTypeTimeout, temporary.ErrorType)
	})

	t.Run("连接失败", func(t *testing.T) {
		err := classifyTransportError(&net.OpError{Op: "dial", Err: errors.New("connection refused")}, false)
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, ErrorTypeConnection, temporary.ErrorType)
	})

	t.Run("配了代理时区分代理故障", func(t *testing.T) {
		err := classifyTransportError(errors.New("socks connect failed"), true)
		var temporary *UpstreamTemporaryError
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, ErrorTypeProxy, temporary.ErrorType)

		// 没配代理时同样的消息不应误判成代理故障。
		err = classifyTransportError(errors.New("socks connect failed"), false)
		require.True(t, errors.As(err, &temporary))
		require.Equal(t, ErrorTypeNetwork, temporary.ErrorType)
	})

	// 调用方主动取消不是上游故障，不能被当成可换号重试的错误。
	t.Run("ctx 取消原样上抛", func(t *testing.T) {
		err := classifyTransportError(context.Canceled, false)
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, IsRotatable(err))
	})

	require.NoError(t, classifyTransportError(nil, false))
}

func TestDefaultModels(t *testing.T) {
	require.Len(t, DefaultModels, len(ImageFamilyModelIDs)+len(VideoModelIDs))

	ids := DefaultModelIDs()
	require.Equal(t, ImageFamilyModelIDs[0], ids[0], "图像族级 id 排在前面")

	seen := make(map[string]bool, len(ids))
	for _, model := range DefaultModels {
		require.NotEmpty(t, model.ID)
		require.NotEmpty(t, model.DisplayName)
		require.Equal(t, "model", model.Type)
		require.False(t, seen[model.ID], "模型 id 重复: %s", model.ID)
		seen[model.ID] = true
	}
}

// 直连下载在拨号层拒绝回环/私网地址：上游给的产物直链不能被用来探测内网。
func TestPlainTransportBlocksPrivateDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("loopback server must not be reached")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := NewPlainTransport("").Do(context.Background(), &Request{Method: http.MethodGet, URL: server.URL})
	require.ErrorContains(t, err, "not allowed")
	require.False(t, IsRotatable(err), "blocked destination is terminal, not a retryable network error")
}

func TestPlainTransportRejectsOversizedBody(t *testing.T) {
	allowLoopbackDownloads(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 64))
	}))
	defer server.Close()

	_, err := NewPlainTransport("").Do(context.Background(), &Request{
		Method: http.MethodGet, URL: server.URL, MaxBodyBytes: 16,
	})
	require.ErrorContains(t, err, "exceeds 16 bytes")
	require.False(t, IsRotatable(err))

	resp, err := NewPlainTransport("").Do(context.Background(), &Request{
		Method: http.MethodGet, URL: server.URL, MaxBodyBytes: 64,
	})
	require.NoError(t, err)
	require.Len(t, resp.Body, 64)
}

func TestPlainTransportInvalidProxyDoesNotLeakCredentials(t *testing.T) {
	_, err := NewPlainTransport("http://user:s3cret@[::1").Do(context.Background(), &Request{
		Method: http.MethodGet, URL: "https://example.com",
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cret")
}

func TestRedactProxyURL(t *testing.T) {
	proxy := "http://user:s3cret@proxy.example.com:8080"
	msg := redactProxyURL("dial "+proxy+" failed for user:s3cret", proxy)
	require.NotContains(t, msg, "s3cret")
	require.Contains(t, msg, "http://proxy.example.com:8080")
}

// 客户端只构造一次：首个请求的短超时不能锁死后续请求的时限。
func TestImpersonateTransportTimeoutIsPerRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(1500 * time.Millisecond)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewImpersonateTransport(Identity{}, "")
	_, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, URL: server.URL + "/fast", Timeout: time.Second})
	require.NoError(t, err)

	resp, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, URL: server.URL + "/slow", Timeout: 5 * time.Second})
	require.NoError(t, err, "a later request with a longer timeout must not inherit the first request's 1s limit")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
