//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ============================================================================
// 背景
// ============================================================================
//
// Anthropic 上游对 body.fallbacks / body.fallback_credit_token 字段实施
// Pydantic extra='forbid' 校验：当且仅当 anthropic-beta header 含
// server-side-fallback-2026-07-01（fallback_credit_token 额外接受
// fallback-credit-2026-07-01 / fallback-credit-2026-06-01）时接受。
// 否则报：
//   "fallbacks: Extra inputs are not permitted"
//
// fallbacks 是 beta Messages API 的 server-side refusal fallback 字段；本仓
// 不写入该字段，全部来自客户端（Claude Code / SDK / OpenCode 等）透传。
// OAuth mimic 用 FullClaudeCodeMimicryBetas 覆盖客户端 beta（不含 fallback
// beta），因此必须在出口按最终 beta header 条件 strip，与 context_management
// 的对称约束同构。策略是"剥字段，不注入 beta"：fallback 会换模型、改计费，
// 不允许当默认打开。
//
// 本文件覆盖：
//   1) sanitizeAnthropicBodyForBetaTokens 对 fallbacks / fallback_credit_token
//      的条件 strip（以及与 context_management 的组合行为）
//   2) buildUpstreamRequest OAuth mimic / API-key passthrough 端到端
//   3) Bedrock 路径的对称 strip（PrepareBedrockRequestBodyWithTokens /
//      sanitizeBedrockCCFields）

// ============================================================================
// sanitizeAnthropicBodyForBetaTokens — fallbacks / fallback_credit_token
// ============================================================================

func TestSanitizeAnthropicBodyForBetaTokens_NoFallbackFieldsNoChange(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body, "oauth-2025-04-20")
	require.False(t, changed)
	require.Equal(t, string(body), string(out))
}

func TestSanitizeAnthropicBodyForBetaTokens_FallbacksKeptWhenBetaPresent(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","fallbacks":"default","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body,
		"claude-code-20250219,oauth-2025-04-20,server-side-fallback-2026-07-01")
	require.False(t, changed, "客户端 header 已带 server-side-fallback beta → 字段保留（不过度删除）")
	require.True(t, gjson.GetBytes(out, "fallbacks").Exists())
	require.Equal(t, "default", gjson.GetBytes(out, "fallbacks").String())
}

func TestSanitizeAnthropicBodyForBetaTokens_FallbacksStrippedWhenBetaMissing(t *testing.T) {
	// 客户端透传的两种形态：字符串 "default" 与模型数组
	for name, fallbacks := range map[string]string{
		"string_default": `"fallbacks":"default"`,
		"model_array":    `"fallbacks":["claude-opus-4-6","claude-sonnet-4-6"]`,
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"claude-haiku-4-5",` + fallbacks + `,"messages":[]}`)
			// 模拟 OAuth mimic / 默认 API-key beta：只有 oauth/interleaved，无 fallback beta
			out, changed := sanitizeAnthropicBodyForBetaTokens(body,
				"oauth-2025-04-20,interleaved-thinking-2025-05-14")
			require.True(t, changed)
			require.False(t, gjson.GetBytes(out, "fallbacks").Exists(),
				"header 不含 server-side-fallback beta 时必须 strip fallbacks，否则上游 400")
			require.True(t, gjson.GetBytes(out, "messages").Exists(), "strip 不得误伤其他字段")
		})
	}
}

func TestSanitizeAnthropicBodyForBetaTokens_FallbacksStrippedWhenHeaderEmpty(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","fallbacks":"default","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body, "")
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "fallbacks").Exists())
}

func TestSanitizeAnthropicBodyForBetaTokens_FallbackCreditTokenStrippedWhenCreditBetaMissing(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","fallback_credit_token":"tok_123","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body, "oauth-2025-04-20,interleaved-thinking-2025-05-14")
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "fallback_credit_token").Exists(),
		"缺 credit/fallback beta 时必须 strip fallback_credit_token")
}

func TestSanitizeAnthropicBodyForBetaTokens_FallbackCreditTokenKeptWithAnyAcceptedBeta(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","fallback_credit_token":"tok_123","messages":[]}`)
	// 三个 beta token 任意一个在 header 中都必须保留字段
	for _, beta := range []string{
		claude.BetaServerSideFallback,
		claude.BetaFallbackCredit,
		claude.BetaFallbackCreditLegacy,
	} {
		out, changed := sanitizeAnthropicBodyForBetaTokens(body, "oauth-2025-04-20,"+beta)
		require.Falsef(t, changed, "header 含 %s 时 fallback_credit_token 必须保留", beta)
		require.Truef(t, gjson.GetBytes(out, "fallback_credit_token").Exists(),
			"header 含 %s 时 fallback_credit_token 必须保留", beta)
	}
}

// ★ 组合场景：只带 context-management beta → 剥 fallbacks，保留 context_management
// （守住"早退导致 fallbacks 漏洗"与"过度删除 context_management"两个方向的回归）
func TestSanitizeAnthropicBodyForBetaTokens_StripsFallbacksKeepsContextManagement(t *testing.T) {
	body := []byte(`{"model":"claude-haiku-4-5","context_management":{"edits":[{"type":"clear_thinking_20251015"}]},"fallbacks":"default","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body, "context-management-2025-06-27")
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "fallbacks").Exists(),
		"header 只有 context-management beta → fallbacks 必须 strip")
	require.True(t, gjson.GetBytes(out, "context_management").Exists(),
		"context-management beta 在 header 中 → context_management 不得被误删")
}

func TestSanitizeAnthropicBodyForBetaTokens_KeepsBothWhenBothBetasPresent(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","context_management":{"edits":[{"type":"clear_thinking_20251015"}]},"fallbacks":"default","fallback_credit_token":"tok_123","messages":[]}`)
	out, changed := sanitizeAnthropicBodyForBetaTokens(body,
		"context-management-2025-06-27,server-side-fallback-2026-07-01")
	require.False(t, changed, "两个 beta 都在 header 中 → 所有字段保留")
	require.True(t, gjson.GetBytes(out, "context_management").Exists())
	require.True(t, gjson.GetBytes(out, "fallbacks").Exists())
	require.True(t, gjson.GetBytes(out, "fallback_credit_token").Exists())
}

func TestSanitizeAnthropicBodyForBetaTokens_EmptyBodyUnchanged(t *testing.T) {
	out, changed := sanitizeAnthropicBodyForBetaTokens([]byte{}, "server-side-fallback-2026-07-01")
	require.False(t, changed)
	require.Empty(t, out)

	out, changed = sanitizeAnthropicBodyForBetaTokens(nil, "server-side-fallback-2026-07-01")
	require.False(t, changed)
	require.Empty(t, out)
}

// ============================================================================
// buildUpstreamRequest 端到端
// 挡住未来某人忘调 sanitize / 将 sanitize 挪到 CCH 之后 等 regression。
// ============================================================================

// OAuth mimic：FullClaudeCodeMimicryBetas 不含 fallback beta → body.fallbacks
// 必须被 strip，且 outgoing anthropic-beta 不得注入 server-side-fallback beta
// （剥字段，不注入 beta）。
func TestBuildUpstreamRequest_OAuthMimicHaiku_StripsFallbacksEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	account := &Account{ID: 601, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-tok"},
		Status:      StatusActive,
		Schedulable: true,
	}
	// 客户端默认透传 "fallbacks":"default"（Claude Code / SDK / OpenCode 等）
	body := []byte(`{"model":"claude-haiku-4-5","fallbacks":"default","messages":[]}`)
	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body,
		"oauth-tok", "oauth", "claude-haiku-4-5", false, true, // mimicClaudeCode=true
	)
	require.NoError(t, err)

	outBody := readUpstreamBodyForTest(t, req)
	outBeta := getHeaderRaw(req.Header, "anthropic-beta")

	require.False(t, gjson.GetBytes(outBody, "fallbacks").Exists(),
		"OAuth mimic 端到端：mimic beta 集合不含 fallback beta → outgoing body 必须没有 fallbacks，"+
			"否则上游报 fallbacks: Extra inputs are not permitted")
	require.False(t, anthropicBetaTokensContains(outBeta, claude.BetaServerSideFallback),
		"修复策略是剥字段而非注入 beta：outgoing anthropic-beta 不得含 server-side-fallback beta")
	require.True(t, anthropicBetaTokensContains(outBeta, claude.BetaContextManagement),
		"mimic beta 集合本身不受影响")
}

// API-key passthrough + 客户端 header 未带 fallback beta → strip
func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_StripsFallbacksWhenClientHeaderMissingBeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	// 客户端仅带 oauth beta，不带 server-side-fallback-2026-07-01
	c.Request.Header.Set("Anthropic-Beta", "oauth-2025-04-20")

	body := []byte(`{"model":"claude-haiku-4-5","fallbacks":"default","messages":[]}`)
	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(), body, "token",
	)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(readUpstreamBodyForTest(t, req), "fallbacks").Exists(),
		"API-key passthrough + 客户端未带 fallback beta → strip body 字段")
}

// API-key passthrough + 客户端 header 带 fallback beta → 保留（不过度删除）
func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_PreservesFallbacksWhenClientHeaderHasBeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Anthropic-Beta", "oauth-2025-04-20,server-side-fallback-2026-07-01")

	// 模型数组形态：有 beta 时必须原样保留
	body := []byte(`{"model":"claude-opus-4-7","fallbacks":["claude-opus-4-6","claude-sonnet-4-6"],"messages":[]}`)
	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, newAnthropicAPIKeyPassthroughAccountForBetaTest(), body, "token",
	)
	require.NoError(t, err)

	outBody := readUpstreamBodyForTest(t, req)
	require.True(t, gjson.GetBytes(outBody, "fallbacks").Exists(),
		"客户端 header 带 server-side-fallback beta → 字段保留（不过度删除）")
	fallbacks := gjson.GetBytes(outBody, "fallbacks").Array()
	require.Len(t, fallbacks, 2)
	require.Equal(t, "claude-opus-4-6", fallbacks[0].String())
	require.Equal(t, "claude-sonnet-4-6", fallbacks[1].String())
}

// ============================================================================
// Bedrock 对称 strip
// ============================================================================

// fallback beta token 不在 bedrockSupportedBetaTokens 白名单内（会被
// filterBedrockBetaTokens 过滤），因此条件 strip 实际总会剥除——这是预期。
func TestPrepareBedrockRequestBodyWithTokens_FallbacksRequireSupportedBeta(t *testing.T) {
	modelID := "us.anthropic.claude-opus-4-6-v1"

	t.Run("strips fallbacks when final tokens omit server-side-fallback beta", func(t *testing.T) {
		input := `{
			"messages":[{"role":"user","content":"hi"}],
			"max_tokens":100,
			"fallbacks":"default",
			"fallback_credit_token":"tok_123"
		}`
		betaTokens := []string{"context-1m-2025-08-07"}

		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), modelID, betaTokens, false)
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "fallbacks").Exists())
		assert.False(t, gjson.GetBytes(result, "fallback_credit_token").Exists())
		assert.Equal(t, "hi", gjson.GetBytes(result, "messages.0.content").String())
		assert.Equal(t, int64(100), gjson.GetBytes(result, "max_tokens").Int())
	})

	t.Run("strips fallbacks even when client passes server-side-fallback token (not whitelisted)", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100,"fallbacks":"default"}`

		result, err := PrepareBedrockRequestBodyWithTokens(
			[]byte(input), modelID, []string{claude.BetaServerSideFallback}, false,
		)
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "fallbacks").Exists(),
			"Bedrock 白名单不含 fallback beta → 条件 strip 总会剥除（预期）")
		for _, name := range bedrockAnthropicBetaNames(result) {
			assert.NotEqual(t, claude.BetaServerSideFallback, name,
				"fallback beta 不得进入 Bedrock anthropic_beta 白名单输出")
		}
	})

	t.Run("leaves body without fallback fields otherwise intact", func(t *testing.T) {
		input := `{"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`

		result, err := PrepareBedrockRequestBodyWithTokens([]byte(input), modelID, nil, false)
		require.NoError(t, err)

		assert.False(t, gjson.GetBytes(result, "fallbacks").Exists())
		assert.False(t, gjson.GetBytes(result, "fallback_credit_token").Exists())
		assert.False(t, gjson.GetBytes(result, "context_management").Exists())
		assert.Equal(t, "hi", gjson.GetBytes(result, "messages.0.content").String())
	})
}

// sanitizeBedrockCCFields：fallbacks / fallback_credit_token 是 Anthropic 直连
// beta API 专有字段，Bedrock 无对应 beta → 无条件剥除。
func TestSanitizeBedrockCCFields_StripsFallbacksUnconditionally(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-6","context_management":{"edits":[]},"fallbacks":"default","fallback_credit_token":"tok_123","messages":[]}`)
	result := sanitizeBedrockCCFields(body)

	assert.False(t, gjson.GetBytes(result, "fallbacks").Exists())
	assert.False(t, gjson.GetBytes(result, "fallback_credit_token").Exists())
	assert.False(t, gjson.GetBytes(result, "context_management").Exists())
	assert.True(t, gjson.GetBytes(result, "messages").Exists())
}
