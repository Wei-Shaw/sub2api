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

func TestPlainTransportRoundTrip(t *testing.T) {
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

func TestPlainTransportInvalidProxy(t *testing.T) {
	_, err := NewPlainTransport("://bad").Do(context.Background(), &Request{
		Method: http.MethodGet, URL: "https://example.com",
	})
	require.ErrorContains(t, err, "invalid proxy url")
}

func TestPlainTransportTimeout(t *testing.T) {
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
