//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// advisorUpstream 是测试用 HTTPUpstream stub，按调用次数模拟上游响应：
//   - 第 1 次调用：返回 firstStatus + firstBody（用于触发 advisor 整流的 400 响应）
//   - 第 2 次调用：返回 200 + secondBody（advisor 整流后的成功响应），或在 failSecondCall 为 true 时返回错误
//
// 同时记录每次调用的 body / header，用于断言 advisor 整流是否正确改写了请求。
type advisorUpstream struct {
	calls          int
	firstStatus    int
	firstBody      string
	secondBody     string
	failSecondCall bool
	captured       []capturedReq
}

type capturedReq struct {
	body         string
	betaHeader   string
	contentType  string
	authHeader   string
	apiKeyHeader string
}

func (u *advisorUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	body, _ := io.ReadAll(req.Body)
	u.captured = append(u.captured, capturedReq{
		body:         string(body),
		betaHeader:   getHeaderRaw(req.Header, "anthropic-beta"),
		contentType:  req.Header.Get("Content-Type"),
		authHeader:   req.Header.Get("Authorization"),
		apiKeyHeader: req.Header.Get("x-api-key"),
	})
	if u.calls == 1 {
		return &http.Response{
			StatusCode: u.firstStatus,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(u.firstBody)),
		}, nil
	}
	if u.failSecondCall {
		return nil, io.ErrUnexpectedEOF
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(u.secondBody)),
	}, nil
}

func (u *advisorUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

var _ HTTPUpstream = (*advisorUpstream)(nil)

const (
	advisorUpstreamErrorBody = `{"error":{"message":"Unexpected value(s) ` + "`advisor-tool-2026-03-01`" + ` for the ` + "`anthropic-beta`" + ` header. Please consult our documentation."}}`
	advisorBodyWithAdvisor   = `{"model":"claude-sonnet-4","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"advisor_20260301","name":"advisor"},{"type":"web_search","name":"search"}],"stream":false}`
	advisorBodyNoAdvisor     = `{"model":"claude-sonnet-4","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":false}`
	advisorSuccessBody       = `{"id":"msg_1","type":"message","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
)

// newAdvisorTestService 构造 AntigravityGatewayService，注入指定 rectifier 配置 + httpUpstream stub
func newAdvisorTestService(rectifierJSON string, upstream HTTPUpstream) *AntigravityGatewayService {
	values := map[string]string{}
	if rectifierJSON != "" {
		values[SettingKeyRectifierSettings] = rectifierJSON
	}
	settingSvc := NewSettingService(&settingRepoStub{values: values}, nil)
	return &AntigravityGatewayService{
		settingService: settingSvc,
		httpUpstream:   upstream,
	}
}

// newAdvisorTestAccount 构造 type=upstream 账号（base_url + api_key 透传）
func newAdvisorTestAccount() *Account {
	return &Account{
		ID:       1,
		Name:     "test-upstream",
		Platform: "antigravity",
		Type:     "upstream",
		Credentials: map[string]any{
			"base_url": "https://example.test",
			"api_key":  "test-api-key",
		},
		Concurrency: 1,
	}
}

// newAdvisorTestContext 构造带 anthropic-beta header 的 gin.Context；返回 context + recorder
func newAdvisorTestContext(t *testing.T, betaHeader string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if betaHeader != "" {
		c.Request.Header.Set("anthropic-beta", betaHeader)
	}
	return c, rec
}

// TestForwardUpstreamAdvisorRetry_Success 验证：400 advisor 错误 → 整流重试 → 成功
// 重试请求：body 不含 advisor 工具，header 不含 advisor token，但保留其它 beta token
func TestForwardUpstreamAdvisorRetry_Success(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus: http.StatusBadRequest,
		firstBody:   advisorUpstreamErrorBody,
		secondBody:  advisorSuccessBody,
	}
	svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, upstream)
	account := newAdvisorTestAccount()
	c, _ := newAdvisorTestContext(t, "advisor-tool-2026-03-01,prompt-caching-2024-07-31")

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyWithAdvisor))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "claude-sonnet-4", result.Model)

	require.Equal(t, 2, upstream.calls, "expected first 400 + second retry call")

	// 第 1 次请求：原样透传（含 advisor 工具 + advisor beta token）
	first := upstream.captured[0]
	require.Contains(t, first.body, "advisor_20260301", "first request should include advisor tool")
	require.Contains(t, strings.ToLower(first.betaHeader), "advisor-tool-2026-03-01")

	// 第 2 次请求：advisor 整流后
	second := upstream.captured[1]
	require.NotContains(t, second.body, "advisor_20260301",
		"retry request must strip advisor tool from body")
	require.Contains(t, second.body, "web_search",
		"retry must keep other tools")
	require.NotContains(t, strings.ToLower(second.betaHeader), "advisor-tool-2026-03-01",
		"retry request must strip advisor token from anthropic-beta")
	require.Contains(t, strings.ToLower(second.betaHeader), "prompt-caching-2024-07-31",
		"retry must keep non-advisor beta tokens")
	// 透传认证不变
	require.Equal(t, "application/json", second.contentType)
	require.Equal(t, "Bearer test-api-key", second.authHeader)
	require.Equal(t, "test-api-key", second.apiKeyHeader)
}

// TestForwardUpstreamAdvisorRetry_NonAdvisorError 验证：400 但不是 advisor 错误 → 不触发整流
func TestForwardUpstreamAdvisorRetry_NonAdvisorError(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus: http.StatusBadRequest,
		firstBody:   `{"error":{"message":"max_tokens must be greater than 0"}}`,
	}
	svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, upstream)
	account := newAdvisorTestAccount()
	c, rec := newAdvisorTestContext(t, "advisor-tool-2026-03-01")

	_, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyWithAdvisor))
	require.NoError(t, err)

	require.Equal(t, 1, upstream.calls, "non-advisor 400 must not trigger retry")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "max_tokens must be greater than 0")
}

// TestForwardUpstreamAdvisorRetry_MasterSwitchOff 验证：advisor 总开关关闭 → 不触发整流
func TestForwardUpstreamAdvisorRetry_MasterSwitchOff(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus: http.StatusBadRequest,
		firstBody:   advisorUpstreamErrorBody,
	}
	svc := newAdvisorTestService(`{"enabled":false,"advisor_tool_enabled":true}`, upstream)
	account := newAdvisorTestAccount()
	c, rec := newAdvisorTestContext(t, "advisor-tool-2026-03-01")

	_, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyWithAdvisor))
	require.NoError(t, err)

	require.Equal(t, 1, upstream.calls, "master switch off must not trigger retry")
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestForwardUpstreamAdvisorRetry_SubswitchOff 验证：advisor 子开关关闭 → 不触发整流
func TestForwardUpstreamAdvisorRetry_SubswitchOff(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus: http.StatusBadRequest,
		firstBody:   advisorUpstreamErrorBody,
	}
	svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":false}`, upstream)
	account := newAdvisorTestAccount()
	c, _ := newAdvisorTestContext(t, "advisor-tool-2026-03-01")

	_, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyWithAdvisor))
	require.NoError(t, err)

	require.Equal(t, 1, upstream.calls, "advisor subswitch off must not trigger retry")
}

// TestForwardUpstreamAdvisorRetry_HeaderOnlyAdvisor 验证：body 不含 advisor 但 header 含 advisor token
// 客户端只设置了 beta header 没声明 advisor 工具时也要触发整流
func TestForwardUpstreamAdvisorRetry_HeaderOnlyAdvisor(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus: http.StatusBadRequest,
		firstBody:   advisorUpstreamErrorBody,
		secondBody:  advisorSuccessBody,
	}
	svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, upstream)
	account := newAdvisorTestAccount()
	c, _ := newAdvisorTestContext(t, "advisor-tool-2026-03-01")

	_, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyNoAdvisor))
	require.NoError(t, err)
	require.Equal(t, 2, upstream.calls, "header-only advisor token should still trigger retry")

	second := upstream.captured[1]
	require.NotContains(t, strings.ToLower(second.betaHeader), "advisor-tool-2026-03-01")
}

// TestForwardUpstreamAdvisorRetry_RetryRequestFails 验证：重试本身请求失败 → 透传原 400 响应
func TestForwardUpstreamAdvisorRetry_RetryRequestFails(t *testing.T) {
	upstream := &advisorUpstream{
		firstStatus:    http.StatusBadRequest,
		firstBody:      advisorUpstreamErrorBody,
		failSecondCall: true,
	}
	svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, upstream)
	account := newAdvisorTestAccount()
	c, rec := newAdvisorTestContext(t, "advisor-tool-2026-03-01")

	_, err := svc.ForwardUpstream(context.Background(), c, account, []byte(advisorBodyWithAdvisor))
	require.NoError(t, err, "retry network error must not surface; original 400 should pass through")

	require.Equal(t, 2, upstream.calls)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "advisor-tool-2026-03-01",
		"original advisor error body must be written when retry fails")
}

// TestShouldRectifyAdvisorToolError_Antigravity 验证 AntigravityGatewayService 自实现的判定与
// 主 Forward 路径 GatewayService.shouldRectifyAdvisorToolError 语义一致
func TestShouldRectifyAdvisorToolError_Antigravity(t *testing.T) {
	body := []byte(advisorUpstreamErrorBody)

	t.Run("nil setting service returns false", func(t *testing.T) {
		svc := &AntigravityGatewayService{}
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(), body))
	})

	t.Run("master off returns false", func(t *testing.T) {
		svc := newAdvisorTestService(`{"enabled":false,"advisor_tool_enabled":true}`, nil)
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(), body))
	})

	t.Run("subswitch off returns false", func(t *testing.T) {
		svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":false}`, nil)
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(), body))
	})

	t.Run("both on + builtin match returns true", func(t *testing.T) {
		svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, nil)
		require.True(t, svc.shouldRectifyAdvisorToolError(context.Background(), body))
	})

	t.Run("non-advisor error returns false", func(t *testing.T) {
		svc := newAdvisorTestService(`{"enabled":true,"advisor_tool_enabled":true}`, nil)
		require.False(t, svc.shouldRectifyAdvisorToolError(context.Background(),
			[]byte(`{"error":{"message":"max_tokens too small"}}`)))
	})
}
