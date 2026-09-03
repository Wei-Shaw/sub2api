//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var _ HTTPUpstream = (*antigravityUpstreamCallRecorder)(nil)

// antigravityUpstreamCallRecorder 记录 HTTPUpstream 调用次数。
// SSRF 用例要求 ForwardUpstream 在发起任何请求之前就失败，因此 calls 必须为 0。
type antigravityUpstreamCallRecorder struct {
	calls int
	urls  []string
	resp  *http.Response
	err   error
}

func (r *antigravityUpstreamCallRecorder) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	r.calls++
	if req != nil && req.URL != nil {
		r.urls = append(r.urls, req.URL.String())
	}
	if r.resp == nil && r.err == nil {
		return nil, errors.New("unexpected upstream call")
	}
	return r.resp, r.err
}

func (r *antigravityUpstreamCallRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return r.Do(req, proxyURL, accountID, concurrency)
}

func newAntigravityUpstreamTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1/messages", strings.NewReader(""))
	return c, recorder
}

func newAntigravityUpstreamAccount(baseURL string) *Account {
	return &Account{
		ID:       7,
		Name:     "upstream-test",
		Platform: PlatformAntigravity,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": baseURL,
			"api_key":  "sk-test",
		},
	}
}

const antigravityUpstreamTestBody = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`

// TestForwardUpstreamRejectsSSRFBaseURL 验证 base_url 指向内网/元数据地址时，
// ForwardUpstream 在构造并发送上游请求之前即失败（不产生任何出网请求）。
//
// 注意：security.url_allowlist.allow_insecure_http 的运行时默认值为 true，
// 关闭白名单时走的是降级校验分支；该分支现在同样执行私网阻断，
// 见 TestForwardUpstreamAllowlistDisabledFallbackBlocksPrivateTargets。
func TestForwardUpstreamRejectsSSRFBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *config.Config
		baseURL string
	}{
		{
			name:    "allowlist enabled blocks link-local metadata host",
			cfg:     antigravityAllowlistConfig(true, "api.anthropic.com"),
			baseURL: "https://169.254.169.254",
		},
		{
			name:    "allowlist enabled blocks host outside allowlist",
			cfg:     antigravityAllowlistConfig(true, "api.anthropic.com"),
			baseURL: "https://evil.example.com",
		},
		{
			name:    "allowlist enabled blocks plaintext http",
			cfg:     antigravityAllowlistConfig(true, "169.254.169.254"),
			baseURL: "http://169.254.169.254",
		},
		{
			name:    "allowlist disabled and insecure http forbidden rejects plaintext http",
			cfg:     antigravityInsecureHTTPConfig(false),
			baseURL: "http://169.254.169.254",
		},
		{
			name:    "nil config falls back to https-only format check",
			cfg:     nil,
			baseURL: "http://169.254.169.254",
		},
		{
			name:    "allowlist disabled rejects unparsable base_url",
			cfg:     antigravityInsecureHTTPConfig(true),
			baseURL: "://169.254.169.254",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &antigravityUpstreamCallRecorder{}
			svc := &AntigravityGatewayService{
				httpUpstream:   upstream,
				settingService: &SettingService{cfg: tc.cfg},
			}
			c, recorder := newAntigravityUpstreamTestContext()

			result, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount(tc.baseURL), []byte(antigravityUpstreamTestBody))

			require.Error(t, err)
			require.Nil(t, result)
			require.Contains(t, err.Error(), "invalid base_url")
			require.Equal(t, 0, upstream.calls, "no upstream request may be issued for a rejected base_url")
			require.Empty(t, recorder.Body.String())
		})
	}
}

// TestForwardUpstreamAllowsAllowlistedBaseURL 反向用例：白名单内的 base_url 必须放行，
// 保证 SSRF 校验没有把合法上游一并拦死。
func TestForwardUpstreamAllowsAllowlistedBaseURL(t *testing.T) {
	upstream := &antigravityUpstreamCallRecorder{err: errors.New("boom")}
	svc := &AntigravityGatewayService{
		httpUpstream:   upstream,
		settingService: &SettingService{cfg: antigravityAllowlistConfig(true, "api.anthropic.com")},
	}
	c, _ := newAntigravityUpstreamTestContext()

	_, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount("https://api.anthropic.com/"), []byte(antigravityUpstreamTestBody))

	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream request failed")
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, []string{"https://api.anthropic.com/v1/messages"}, upstream.urls)
}

// TestForwardUpstreamDoesNotEchoUpstreamErrorBody 验证上游错误不再原样回显给调用方，
// 避免 base_url 被篡改时上游响应体（如云厂商 IAM 凭证）直接外泄。
func TestForwardUpstreamDoesNotEchoUpstreamErrorBody(t *testing.T) {
	secret := "ASIAEXAMPLESECRETTOKEN"
	upstream := &antigravityUpstreamCallRecorder{
		resp: &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"AccessKeyId":"` + secret + `"}`)),
		},
	}
	svc := &AntigravityGatewayService{
		httpUpstream:   upstream,
		settingService: &SettingService{cfg: antigravityAllowlistConfig(true, "api.anthropic.com")},
	}
	c, recorder := newAntigravityUpstreamTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount("https://api.anthropic.com"), []byte(antigravityUpstreamTestBody))

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, upstream.calls)
	require.NotContains(t, recorder.Body.String(), secret)
	require.Contains(t, recorder.Body.String(), "permission_error")
}

// TestForwardUpstreamLimitsResponseBody 验证非流式响应读取受 UpstreamResponseReadMaxBytes 限制。
func TestForwardUpstreamLimitsResponseBody(t *testing.T) {
	cfg := antigravityAllowlistConfig(true, "api.anthropic.com")
	cfg.Gateway.UpstreamResponseReadMaxBytes = 64

	upstream := &antigravityUpstreamCallRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"padding":"` + strings.Repeat("a", 4096) + `"}`)),
		},
	}
	svc := &AntigravityGatewayService{
		httpUpstream:   upstream,
		settingService: &SettingService{cfg: cfg},
	}
	c, _ := newAntigravityUpstreamTestContext()

	result, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount("https://api.anthropic.com"), []byte(antigravityUpstreamTestBody))

	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
}

// TestForwardUpstreamAllowlistDisabledFallbackBlocksPrivateTargets 固化 M2 修复后的行为：
// security.url_allowlist.enabled=false（默认，主机白名单仍是 opt-in）时走降级校验分支，
// 但该分支不再只做格式校验——私网/环回/链路本地目标由独立开关
// security.url_allowlist.allow_private_hosts 控制，默认阻断，
// 因此 http://169.254.169.254 在发起任何出网请求前即被拒绝。
//
// 该用例同时固化「运营者可以主动放行私网上游」这条出路：显式允许私网后，
// 指向 LAN/localhost 的自建上游必须恢复可达，否则等于把自托管部署打死。
//
// 注意：单测直接构造 config.Config，不经过 config.Load，所以要显式注入
// Load 在运行时注入的同一策略（urlvalidator.SetBlockPrivateUpstreams）。
func TestForwardUpstreamAllowlistDisabledFallbackBlocksPrivateTargets(t *testing.T) {
	forward := func(t *testing.T, blockPrivate bool, baseURL string) (*antigravityUpstreamCallRecorder, error) {
		t.Helper()
		prev := urlvalidator.BlockPrivateUpstreams()
		urlvalidator.SetBlockPrivateUpstreams(blockPrivate)
		t.Cleanup(func() { urlvalidator.SetBlockPrivateUpstreams(prev) })

		upstream := &antigravityUpstreamCallRecorder{err: errors.New("boom")}
		svc := &AntigravityGatewayService{
			httpUpstream:   upstream,
			settingService: &SettingService{cfg: antigravityInsecureHTTPConfig(true)},
		}
		c, _ := newAntigravityUpstreamTestContext()

		_, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount(baseURL), []byte(antigravityUpstreamTestBody))
		return upstream, err
	}

	t.Run("默认阻断云元数据地址", func(t *testing.T) {
		upstream, err := forward(t, true, "http://169.254.169.254")

		require.Error(t, err)
		require.ErrorIs(t, err, urlvalidator.ErrPrivateHostBlocked)
		require.Contains(t, err.Error(), "invalid base_url")
		// 报错必须点名放行开关，否则运营者不知道去哪里开。
		require.Contains(t, err.Error(), urlvalidator.AllowPrivateHostsSettingKey)
		require.Equal(t, 0, upstream.calls, "M2: 私网目标必须在出网前被拒绝")
	})

	t.Run("默认阻断 localhost", func(t *testing.T) {
		upstream, err := forward(t, true, "http://localhost:8080")

		require.Error(t, err)
		require.ErrorIs(t, err, urlvalidator.ErrPrivateHostBlocked)
		require.Equal(t, 0, upstream.calls)
	})

	t.Run("显式放行后私网上游恢复可达", func(t *testing.T) {
		upstream, err := forward(t, false, "http://192.168.1.10:3000")

		require.Error(t, err) // 来自 stub 上游的 boom，而非校验失败
		require.NotContains(t, err.Error(), "invalid base_url")
		require.Equal(t, 1, upstream.calls, "allow_private_hosts=true 时自建 LAN 上游必须仍然可用")
	})

	t.Run("公网目标不受影响", func(t *testing.T) {
		upstream, err := forward(t, true, "http://relay.example.com")

		require.Error(t, err)
		require.NotContains(t, err.Error(), "invalid base_url")
		require.Equal(t, 1, upstream.calls)
	})
}

func antigravityInsecureHTTPConfig(allowInsecureHTTP bool) *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = allowInsecureHTTP
	return cfg
}

func antigravityAllowlistConfig(enabled bool, hosts ...string) *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = enabled
	cfg.Security.URLAllowlist.UpstreamHosts = hosts
	return cfg
}
