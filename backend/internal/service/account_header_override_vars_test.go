//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
		"x-trace":    "…user_id=7…api_key_id=31…group_id=9…request_id=req-abc",
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

	t.Run("legacy invalid template still blocks the client header", func(t *testing.T) {
		// 未经保存校验的存量数据：模板不合法的条目不发送，但同名客户端头照样删除
		legacy := headerOverrideVarsAccount(PlatformAnthropic, AccountTypeAPIKey, map[string]any{
			"x-legacy": "{{user_email}}",
			"x-open":   "id-{{user_id",
		})
		h := http.Header{}
		h.Set("X-Legacy", "spoofed-by-client")
		h.Set("X-Open", "spoofed-by-client")
		legacy.ApplyHeaderOverrides(headerOverrideUserContext(), h)
		require.Empty(t, h)
	})
}

func TestHeaderOverrideSharedFetchCarriesNoIdentity(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-end-user": "{{user_id}}",
		"x-static":   "plain",
	})
	userCtx := headerOverrideUserContext()
	h := http.Header{}
	h.Set("X-End-User", "spoofed-by-client")
	acc.ApplyHeaderOverrides(withoutHeaderOverrideRequestScope(userCtx), h)
	require.Equal(t, http.Header{"x-static": []string{"plain"}}, h)
	require.Nil(t, headerOverrideReportedHeaders(userCtx, acc.ID))

	// 无身份的 ctx 原样返回
	plain := context.Background()
	require.Equal(t, plain, withoutHeaderOverrideRequestScope(plain))
}

func TestHeaderOverrideReportedHeadersRedactLiterals(t *testing.T) {
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-relay-auth": "sk-relay-secret-{{user_id}}",
		"x-trace":      "key={{api_key_id}};req={{request_id}}",
		"x-end-user":   "{{user_id}}{{api_key_id}}",
	})
	ctx := headerOverrideUserContext()
	h := http.Header{}
	acc.ApplyHeaderOverrides(ctx, h)

	require.Equal(t, []string{"sk-relay-secret-7"}, h["x-relay-auth"], "upstream still gets the full value")
	require.Equal(t, map[string]string{
		"x-relay-auth": "…user_id=7",
		"x-trace":      "…api_key_id=31…request_id=req-abc",
		"x-end-user":   "731",
	}, headerOverrideReportedHeaders(ctx, acc.ID))
}

func TestHeaderOverrideRequestIDIsClientControlledAndSanitized(t *testing.T) {
	// request_id 可由客户端通过 X-Request-ID 自带：不得借分隔符伪造其它字段、
	// 不得被二次解析成变量、不得注入请求头，也不能让上报头整条消失。
	acc := headerOverrideVarsAccount(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		"x-trace": "u={{user_id}};r={{request_id}}",
	})
	for name, tc := range map[string]struct{ requestID, want string }{
		"uuid":           {"6f1c2d3e-8a4b-4c5d-9e0f-a1b2c3d4e5f6", "u=7;r=6f1c2d3e-8a4b-4c5d-9e0f-a1b2c3d4e5f6"},
		"field forgery":  {"x;u=8", "u=7;r=x_u_8"},
		"looks variable": {"{{user_id}}", "u=7;r=__user_id__"},
		"crlf":           {"bad\r\nx-injected: 1", "u=7;r=bad__x-injected__1"},
		"non ascii":      {"请求-1", "u=7;r=__-1"},
		"oversized":      {strings.Repeat("a", 500), "u=7;r=" + strings.Repeat("a", maxHeaderOverrideRequestIDLength)},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), ctxkey.RequestID, tc.requestID)
			ctx = WithHeaderOverrideRequestScope(ctx, 7, 31, nil)
			h := http.Header{}
			acc.ApplyHeaderOverrides(ctx, h)
			require.Equal(t, []string{tc.want}, h["x-trace"])
		})
	}

	// 没有请求 ID 时仍按"取不到值"处理
	ctx := WithHeaderOverrideRequestScope(context.Background(), 7, 31, nil)
	h := http.Header{}
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

// 真实转发构建器：变量头随 ctx 中的入站身份写入出站请求，客户端同名头被替换。
func TestAnthropicUpstreamBuildersReportIdentity(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["base_url"] = "https://anthropic.example.com"
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{
		"x-gateway-user-id":    "{{user_id}}",
		"x-gateway-request-id": "{{request_id}}",
	}
	body := []byte(`{"model":"claude-haiku-4-5","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	svc := &GatewayService{cfg: &config.Config{}}

	build := map[string]func(ctx context.Context, c *gin.Context) (*http.Request, error){
		"forward": func(ctx context.Context, c *gin.Context) (*http.Request, error) {
			req, _, err := svc.buildUpstreamRequest(ctx, c, account, body, "upstream-anthropic-key", "apikey", "claude-haiku-4-5", false, false)
			return req, err
		},
		"passthrough": func(ctx context.Context, c *gin.Context) (*http.Request, error) {
			req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(ctx, c, account, body, "upstream-anthropic-key")
			return req, err
		},
	}
	for name, fn := range build {
		t.Run(name, func(t *testing.T) {
			c := gatewayAnthropicBaseURLContext(t)
			c.Request.Header.Set("X-Gateway-User-Id", "spoofed-by-client")

			req, err := fn(headerOverrideUserContext(), c)
			require.NoError(t, err)
			require.Equal(t, []string{"7"}, headerValuesFold(req.Header, "x-gateway-user-id"))
			require.Equal(t, []string{"req-abc"}, headerValuesFold(req.Header, "x-gateway-request-id"))

			// 无用户上下文：不上报，也不放行客户端伪造的同名头
			req, err = fn(context.Background(), c)
			require.NoError(t, err)
			require.Empty(t, headerValuesFold(req.Header, "x-gateway-user-id"))
			require.Empty(t, headerValuesFold(req.Header, "x-gateway-request-id"))
		})
	}
}

func TestAdminUpdateAccountRejectsVariablesOnNonAPIKeyAccounts(t *testing.T) {
	newRepo := func(accountType string) *upstreamBillingProbeAccountRepo {
		return &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			301: {
				ID:       301,
				Name:     "grok",
				Platform: PlatformGrok,
				Type:     accountType,
				Status:   StatusActive,
				Credentials: map[string]any{
					"api_key":                    "xai-key",
					credKeyHeaderOverrideEnabled: true,
					credKeyHeaderOverrides:       map[string]any{"x-end-user": "{{user_id}}"},
				},
			},
		}}
	}

	// 只改类型、不带 credentials：也必须拦住
	_, err := (&adminServiceImpl{accountRepo: newRepo(AccountTypeAPIKey)}).UpdateAccount(
		context.Background(), 301, &UpdateAccountInput{Type: AccountTypeOAuth})
	require.Error(t, err)

	// OAuth 账号提交带变量的覆写
	_, err = (&adminServiceImpl{accountRepo: newRepo(AccountTypeOAuth)}).UpdateAccount(
		context.Background(), 301, &UpdateAccountInput{Credentials: map[string]any{
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"x-end-user": "{{user_id}}"},
		}})
	require.Error(t, err)

	// api_key 账号的无关编辑不受影响
	updated, err := (&adminServiceImpl{accountRepo: newRepo(AccountTypeAPIKey)}).UpdateAccount(
		context.Background(), 301, &UpdateAccountInput{Name: "renamed"})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
}
