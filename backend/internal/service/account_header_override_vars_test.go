//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func headerOverrideVarsAccount(platform, accountType string, overrides map[string]any) *Account {
	acc := headerOverrideTestAccount(platform, accountType, map[string]any{
		credKeyHeaderOverrideEnabled: true,
		credKeyHeaderOverrides:       overrides,
	})
	acc.ID = 42
	return acc
}

func headerOverrideUserContext() context.Context {
	groupID := int64(9)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-abc")
	return WithHeaderOverrideRequestScope(ctx, 7, 31, &groupID)
}

func TestApplyHeaderOverridesExpandsVariables(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-end-user":   "{{user_id}}",
		"x-trace":      "u={{ user_id }};k={{api_key_id}};g={{group_id}};r={{request_id}}",
		"x-static":     "plain",
		"x-json-value": `{"team":"a"}`,
	})
	ctx := headerOverrideUserContext()
	h := http.Header{}
	h.Set("X-End-User", "spoofed-by-client")

	acc.ApplyHeaderOverrides(ctx, h)

	require.Equal(t, []string{"7"}, h["x-end-user"])
	require.Equal(t, []string{"u=7;k=31;g=9;r=req-abc"}, h["x-trace"])
	require.Equal(t, []string{"plain"}, h["x-static"])
	require.Equal(t, []string{`{"team":"a"}`}, h["x-json-value"])
	require.Empty(t, h.Values("X-End-User"), "client-supplied casing must be replaced, not kept alongside")

	// 只记录变量生成的请求头，静态覆写值（可能是中转密钥）不进记录
	require.Equal(t, map[string]string{
		"x-end-user": "7",
		"x-trace":    "u=7;k=31;g=9;r=req-abc",
	}, headerOverrideReportedHeaders(ctx, acc.ID))
	require.Nil(t, headerOverrideReportedHeaders(ctx, acc.ID+1))
}

func TestApplyHeaderOverridesNeverExpandsForOAuth(t *testing.T) {
	// Grok OAuth 账号支持静态覆写，但变量绝不解析、也不把模板原文发给上游
	acc := headerOverrideVarsAccount(PlatformGrok, AccountTypeOAuth, map[string]any{
		"x-end-user": "{{user_id}}",
		"x-static":   "plain",
	})
	ctx := headerOverrideUserContext()
	h := http.Header{}
	h.Set("X-End-User", "spoofed-by-client")

	acc.ApplyHeaderOverrides(ctx, h)

	require.Empty(t, headerValuesFold(h, "x-end-user"))
	require.Equal(t, []string{"plain"}, h["x-static"])
	require.Nil(t, headerOverrideReportedHeaders(ctx, acc.ID))
	require.Empty(t, acc.headerOverrideReportedIdentity(h))

	// 不具备覆写资格的 OAuth 账号整体 no-op
	anthropicOAuth := headerOverrideVarsAccount(PlatformAnthropic, AccountTypeOAuth, map[string]any{
		"x-end-user": "{{user_id}}",
	})
	h = http.Header{}
	anthropicOAuth.ApplyHeaderOverrides(ctx, h)
	require.Empty(t, h)
}

func TestApplyHeaderOverridesOmitsUnresolvedVariables(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]any{
		"x-end-user": "{{user_id}}",
		"x-group":    "{{group_id}}",
		"x-request":  "{{request_id}}",
		"x-static":   "plain",
	})

	t.Run("no user context", func(t *testing.T) {
		// 账号测试/探针等后台请求：即使 ctx 带 request_id 也不上报
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, "admin-req")
		h := http.Header{}
		h.Set("X-End-User", "spoofed-by-client")
		acc.ApplyHeaderOverrides(ctx, h)
		require.Equal(t, http.Header{"x-static": []string{"plain"}}, h)
	})

	t.Run("key without group", func(t *testing.T) {
		ctx := WithHeaderOverrideRequestScope(context.Background(), 7, 31, nil)
		h := http.Header{}
		acc.ApplyHeaderOverrides(ctx, h)
		require.Equal(t, []string{"7"}, h["x-end-user"])
		require.Empty(t, headerValuesFold(h, "x-group"))
		require.Empty(t, headerValuesFold(h, "x-request"))
	})

	t.Run("connection scope drops request_id only", func(t *testing.T) {
		ctx := withHeaderOverrideConnectionScope(headerOverrideUserContext())
		h := http.Header{}
		acc.ApplyHeaderOverrides(ctx, h)
		require.Equal(t, []string{"7"}, h["x-end-user"])
		require.Equal(t, []string{"9"}, h["x-group"])
		require.Empty(t, headerValuesFold(h, "x-request"))
	})

	t.Run("legacy unknown variable is dropped", func(t *testing.T) {
		legacy := headerOverrideVarsAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]any{
			"x-legacy": "{{user_email}}",
			"x-open":   "{{user_id",
		})
		require.Nil(t, legacy.GetHeaderOverrides(), "entries that fail validation never reach the apply path")
	})
}

func TestHeaderOverrideTemplateIsNotReexpanded(t *testing.T) {
	// request_id 来自入站请求：其内容即使形如变量也只作为字面量输出
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-request": "{{request_id}}",
	})
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "{{user_id}}")
	ctx = WithHeaderOverrideRequestScope(ctx, 7, 31, nil)
	h := http.Header{}
	acc.ApplyHeaderOverrides(ctx, h)
	require.Equal(t, []string{"{{user_id}}"}, h["x-request"])

	// 解析结果含非法字符时整条不发送
	ctx = context.WithValue(context.Background(), ctxkey.RequestID, "bad\r\nx-injected: 1")
	ctx = WithHeaderOverrideRequestScope(ctx, 7, 31, nil)
	h = http.Header{}
	acc.ApplyHeaderOverrides(ctx, h)
	require.Empty(t, h)
}

func TestNormalizeHeaderOverrideCredentialsValidatesVariables(t *testing.T) {
	valid := map[string]any{credKeyHeaderOverrides: map[string]any{
		"X-End-User": " {{ user_id }} ",
		"x-meta":     `{"u":"{{user_id}}","k":"{{api_key_id}}"}`,
	}}
	require.NoError(t, NormalizeHeaderOverrideCredentials(valid))
	require.Equal(t, map[string]any{
		"x-end-user": "{{ user_id }}",
		"x-meta":     `{"u":"{{user_id}}","k":"{{api_key_id}}"}`,
	}, valid[credKeyHeaderOverrides])

	for name, value := range map[string]string{
		"unknown":       "{{user_email}}",
		"typo":          "{{userid}}",
		"empty":         "{{}}",
		"unterminated":  "id-{{user_id",
		"upper":         "{{USER_ID}}",
		"nested":        "{{ {{user_id}} }}",
		"triple braces": "{{{user_id}}}",
	} {
		t.Run(name, func(t *testing.T) {
			err := NormalizeHeaderOverrideCredentials(map[string]any{
				credKeyHeaderOverrides: map[string]any{"x-end-user": value},
			})
			require.Error(t, err)
		})
	}
}

func TestValidateHeaderOverrideVariableUsage(t *testing.T) {
	withVars := map[string]any{credKeyHeaderOverrides: map[string]any{"x-end-user": "{{user_id}}"}}
	static := map[string]any{credKeyHeaderOverrides: map[string]any{"x-relay": "token"}}

	require.NoError(t, ValidateHeaderOverrideVariableUsage(AccountTypeAPIKey, withVars))
	require.Error(t, ValidateHeaderOverrideVariableUsage(AccountTypeOAuth, withVars))
	require.Error(t, ValidateHeaderOverrideVariableUsage(AccountTypeSetupToken, withVars))
	require.NoError(t, ValidateHeaderOverrideVariableUsage(AccountTypeOAuth, static))
	require.NoError(t, ValidateHeaderOverrideVariableUsage(AccountTypeOAuth, nil))
}

func TestHeaderOverrideValueIgnoresVariableEntries(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]any{
		"anthropic-beta": "beta-{{user_id}}",
		"x-static":       "plain",
	})
	_, ok := acc.HeaderOverrideValue("anthropic-beta")
	require.False(t, ok)
	value, ok := acc.HeaderOverrideValue("x-static")
	require.True(t, ok)
	require.Equal(t, "plain", value)
}

func TestOpenAIWSHandshakeCompatibilitySeparatesReportedIdentity(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-end-user": "{{user_id}}",
		"x-static":   "plain",
	})
	build := func(ctx context.Context) openAIWSHandshakeCompatibilityKey {
		h := http.Header{}
		acc.ApplyHeaderOverrides(withHeaderOverrideConnectionScope(ctx), h)
		return normalizeOpenAIWSHandshakeCompatibility(acc, h)
	}
	userA := build(WithHeaderOverrideRequestScope(context.Background(), 7, 31, nil))
	userAOtherKey := build(WithHeaderOverrideRequestScope(context.Background(), 7, 32, nil))
	userB := build(WithHeaderOverrideRequestScope(context.Background(), 8, 40, nil))
	prewarm := build(context.Background())

	require.Equal(t, userA, userAOtherKey, "same reported identity may share a connection")
	require.NotEqual(t, userA, userB, "different users must never share a handshake")
	require.NotEqual(t, userA, prewarm, "identity-less connections are not reusable for a user")

	// 没有变量覆写的账号不受影响
	plain := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"x-static": "plain"})
	require.Empty(t, plain.headerOverrideReportedIdentity(http.Header{"x-static": []string{"plain"}}))
}

func TestAppendOpsUpstreamErrorCapturesReportedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-end-user": "{{user_id}}",
		"x-static":   "relay-secret",
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(headerOverrideUserContext())

	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://upstream.example/v1/responses", nil)
	require.NoError(t, err)
	acc.ApplyHeaderOverrides(upstreamReq.Context(), upstreamReq.Header)

	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: acc.ID, UpstreamStatusCode: 400, Kind: "http_error"})
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{AccountID: acc.ID + 1, UpstreamStatusCode: 500, Kind: "http_error"})

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 2)
	require.Equal(t, map[string]string{"x-end-user": "7"}, events[0].ReportedHeaders)
	require.Nil(t, events[1].ReportedHeaders, "another account's attempt reported nothing")
}
