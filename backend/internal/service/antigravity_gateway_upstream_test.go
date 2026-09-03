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
// 注意：security.url_allowlist.allow_insecure_http 的运行时默认值为 true，因此
// 「关闭白名单 + 仅格式校验」这条降级分支并不能拦住 http 内网地址，
// 见 TestForwardUpstreamAllowlistDisabledFallbackIsFormatOnly。
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

// TestForwardUpstreamAllowlistDisabledFallbackIsFormatOnly 固化当前行为：
// security.url_allowlist.enabled=false（默认）时只做格式校验，配合默认开启的
// allow_insecure_http，http://169.254.169.254 仍会被放行。
// 这是安全审计 M2（降级校验过弱）的范围，不在本次修复内；此用例用于让 M2 一旦被修复
// 时能立刻暴露出来，而不是假装该路径已经安全。
func TestForwardUpstreamAllowlistDisabledFallbackIsFormatOnly(t *testing.T) {
	upstream := &antigravityUpstreamCallRecorder{err: errors.New("boom")}
	svc := &AntigravityGatewayService{
		httpUpstream:   upstream,
		settingService: &SettingService{cfg: antigravityInsecureHTTPConfig(true)},
	}
	c, _ := newAntigravityUpstreamTestContext()

	_, err := svc.ForwardUpstream(context.Background(), c, newAntigravityUpstreamAccount("http://169.254.169.254"), []byte(antigravityUpstreamTestBody))

	require.Error(t, err)
	require.NotContains(t, err.Error(), "invalid base_url")
	require.Equal(t, 1, upstream.calls, "M2: 关闭白名单时仅格式校验，内网地址仍可达（待 M2 修复后此断言应改为 0）")
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
