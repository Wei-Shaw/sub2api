package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 验证：Vertex service_account 路径下，BetaPolicy filter 能从客户端透传的
// anthropic-beta header 中剥掉指定 token。
// 复刻线上 400 报错：upstream 拒绝 prompt-caching-scope-2026-01-05 / redact-thinking-2026-02-12。
func TestBetaPolicyFilter_StripsTokensOnVertexServiceAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	// 客户端（Claude Code CLI）透传过来的完整 anthropic-beta header
	c.Request.Header.Set("Anthropic-Beta",
		"claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,"+
			"prompt-caching-scope-2026-01-05,redact-thinking-2026-02-12,context-management-2025-06-27")

	// Forward() 正式路径里由 evaluateBetaPolicy 读 DB 后缓存到 gin.Context；
	// 这里直接预填，避免单测依赖 settingService。
	c.Set(betaPolicyFilterSetKey, map[string]struct{}{
		"prompt-caching-scope-2026-01-05": {},
		"redact-thinking-2026-02-12":      {},
	})

	account := &Account{
		ID:       401,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")

	require.False(t, anthropicBetaTokensContains(outBeta, "prompt-caching-scope-2026-01-05"),
		"filter 必须剥掉 prompt-caching-scope-2026-01-05；实际 outgoing beta=%q", outBeta)
	require.False(t, anthropicBetaTokensContains(outBeta, "redact-thinking-2026-02-12"),
		"filter 必须剥掉 redact-thinking-2026-02-12；实际 outgoing beta=%q", outBeta)

	// 未被 filter 的 token 必须保留，确认 strip 是精确剥离而非全清。
	for _, keep := range []string{
		"claude-code-20250219",
		"oauth-2025-04-20",
		"interleaved-thinking-2025-05-14",
		"context-management-2025-06-27",
	} {
		require.True(t, anthropicBetaTokensContains(outBeta, keep),
			"token %q 不应被 filter 影响；实际 outgoing beta=%q", keep, outBeta)
	}
}

// 反面对照：filter 集为空时（即未配置 BetaPolicy）必须完全透传，
// 防止后续重构在默认路径上误加 strip。
func TestBetaPolicyFilter_PassesThroughWhenEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta",
		"interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,redact-thinking-2026-02-12")
	// 注意：不 Set betaPolicyFilterSetKey，模拟 BetaPolicy 未配置。

	account := &Account{
		ID:       402,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")
	for _, want := range []string{
		"interleaved-thinking-2025-05-14",
		"prompt-caching-scope-2026-01-05",
		"redact-thinking-2026-02-12",
	} {
		require.True(t, anthropicBetaTokensContains(outBeta, want),
			"未配置 filter 时所有客户端 token 必须透传；缺失 %q（outgoing=%q）", want, outBeta)
	}
}

// 验证：OAuth mimicry 路径下，BetaPolicy filter 同样能剥掉 mimicry **自己注入**
// 的 token（如 prompt-caching-scope-2026-01-05 来自 FullClaudeCodeMimicryBetas）。
// 这是 Anthropic 直连 OAuth 账号（非 Vertex）的对称场景。
func TestBetaPolicyFilter_StripsMimicryInjectedTokenOnOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	// 客户端 header 故意不带任何 anthropic-beta，证明被注入的 token 也会被 filter。

	c.Set(betaPolicyFilterSetKey, map[string]struct{}{
		"prompt-caching-scope-2026-01-05": {},
	})

	account := &Account{
		ID:       403,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
	}
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	// mimicClaudeCode=true → 触发 FullClaudeCodeMimicryBetas 注入
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"oauth-token", "oauth", "claude-opus-4-7", false, false,
	)
	require.NoError(t, err)

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")
	require.False(t, anthropicBetaTokensContains(outBeta, "prompt-caching-scope-2026-01-05"),
		"OAuth mimicry 路径下，filter 必须能从 mimicry 注入的 beta 中剥掉指定 token；"+
			"实际 outgoing beta=%q", outBeta)
	// 同路径其它 mimic 注入的 token 必须保留。
	require.True(t, anthropicBetaTokensContains(outBeta, "claude-code-20250219"),
		"未被 filter 的 mimic token 必须保留；outgoing=%q", outBeta)
}

// 验证：Vertex 路径上 context_management body sanitize 按【最终】outgoing beta
// 决定，而不是按客户端原始 beta。具体场景：客户端 header 带了
// context-management-2025-06-27、body 带了 context_management 字段，
// 管理员把该 beta 设为 filter → outgoing header 应该不含 beta，body 也应该
// strip 掉同名字段（保持 header / body 对称，避免上游报
// `context_management: Extra inputs are not permitted`）。
func TestBetaPolicyFilter_ContextManagementBodySanitizeUsesFinalBeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta",
		"interleaved-thinking-2025-05-14,context-management-2025-06-27")
	c.Set(betaPolicyFilterSetKey, map[string]struct{}{
		"context-management-2025-06-27": {},
	})

	account := &Account{
		ID:       405,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
	body := []byte(`{"model":"claude-opus-4-7","context_management":{"edits":[{"type":"clear_thinking_20251015"}]},"messages":[{"role":"user","content":"hi"}]}`)

	svc := &GatewayService{}
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"vertex-token", "service_account", "claude-opus-4-7@20260417", false, false,
	)
	require.NoError(t, err)

	outBeta := getHeaderRaw(req.Header, "anthropic-beta")
	require.False(t, anthropicBetaTokensContains(outBeta, "context-management-2025-06-27"),
		"filter 应剥掉 context-management-2025-06-27；outgoing=%q", outBeta)

	gotBody := readRequestBodyForTest(t, req)
	require.False(t, gjson.GetBytes(gotBody, "context_management").Exists(),
		"outgoing header 已不含 context-management beta 时，body.context_management 必须同步 strip，"+
			"否则上游会报 `context_management: Extra inputs are not permitted`")
}
