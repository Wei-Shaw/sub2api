package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestNormalizeClaudeOAuthRequestBody_HaikuModelDoesNotReceiveContextManagementAutoInjection
// pins the Haiku exemption added by PR fixing #2506. Anthropic /v1/messages rejects
// context_management with HTTP 400 for claude-haiku-4-5-* models even when thinking is
// enabled, so sub2api must NOT auto-inject it for Haiku-targeted requests.
func TestNormalizeClaudeOAuthRequestBody_HaikuModelDoesNotReceiveContextManagementAutoInjection(t *testing.T) {
	bodyWithThinkingEnabledButNoContextManagement := []byte(`{` +
		`"model":"claude-haiku-4-5-20251001",` +
		`"messages":[{"role":"user","content":"hi"}],` +
		`"thinking":{"type":"enabled","budget_tokens":1024}` +
		`}`)

	out, _ := normalizeClaudeOAuthRequestBody(
		bodyWithThinkingEnabledButNoContextManagement,
		"claude-haiku-4-5-20251001",
		claudeOAuthNormalizeOptions{},
	)

	require.False(t,
		gjson.GetBytes(out, "context_management").Exists(),
		"Haiku 模型不应被自动注入 context_management —— Anthropic 上游会拒绝该字段返回 400",
	)
}

// TestNormalizeClaudeOAuthRequestBody_SonnetModelStillReceivesContextManagementAutoInjection
// regression-pins the pre-existing behavior for Sonnet (and by extension Opus). The Haiku
// exemption must not regress the Sonnet/Opus path which legitimately needs the injection
// to match real Claude CLI byte-level behavior.
func TestNormalizeClaudeOAuthRequestBody_SonnetModelStillReceivesContextManagementAutoInjection(t *testing.T) {
	bodyWithThinkingEnabledButNoContextManagement := []byte(`{` +
		`"model":"claude-sonnet-4-5-20250929",` +
		`"messages":[{"role":"user","content":"hi"}],` +
		`"thinking":{"type":"enabled","budget_tokens":1024}` +
		`}`)

	out, _ := normalizeClaudeOAuthRequestBody(
		bodyWithThinkingEnabledButNoContextManagement,
		"claude-sonnet-4-5-20250929",
		claudeOAuthNormalizeOptions{},
	)

	require.True(t,
		gjson.GetBytes(out, "context_management").Exists(),
		"Sonnet 模型应继续被自动注入 context_management 以匹配真实 CLI 字节级行为",
	)
	require.Equal(t,
		"clear_thinking_20251015",
		gjson.GetBytes(out, "context_management.edits.0.type").String(),
		"注入的 edit type 应保持为 clear_thinking_20251015",
	)
}

// TestNormalizeClaudeOAuthRequestBody_HaikuModelExplicitContextManagementStillPassesThrough
// confirms the exemption does NOT strip an explicit client-provided context_management —
// only skips the AUTO-injection. If a client deliberately sends context_management for
// Haiku (e.g. to test or to override), the field passes through verbatim. The Anthropic
// upstream will still reject it, but that's the client's responsibility.
func TestNormalizeClaudeOAuthRequestBody_HaikuModelExplicitContextManagementStillPassesThrough(t *testing.T) {
	bodyWithExplicitContextManagementFromClient := []byte(`{` +
		`"model":"claude-haiku-4-5-20251001",` +
		`"messages":[],` +
		`"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}` +
		`}`)

	out, _ := normalizeClaudeOAuthRequestBody(
		bodyWithExplicitContextManagementFromClient,
		"claude-haiku-4-5-20251001",
		claudeOAuthNormalizeOptions{},
	)

	require.True(t,
		gjson.GetBytes(out, "context_management").Exists(),
		"Haiku 例外只跳过自动注入，不应剥离客户端显式传入的 context_management",
	)
	require.Equal(t,
		"clear_tool_uses_20250919",
		gjson.GetBytes(out, "context_management.edits.0.type").String(),
		"客户端显式 edits 必须原样保留",
	)
}

// TestNormalizeClaudeOAuthRequestBody_HaikuMatchIsCaseInsensitive matches the convention
// established by the neighboring beta-header logic at line ~6093 which uses
// strings.Contains(strings.ToLower(modelID), "haiku"). Uppercased or mixed-case Haiku
// model IDs MUST also be exempt.
func TestNormalizeClaudeOAuthRequestBody_HaikuMatchIsCaseInsensitive(t *testing.T) {
	for _, modelIDInVariousCasings := range []string{
		"claude-haiku-4-5-20251001",
		"CLAUDE-HAIKU-4-5-20251001",
		"Claude-Haiku-4-5-20251001",
		"claude-3-5-haiku-20241022", // legacy variant
	} {
		bodyWithThinkingEnabled := []byte(`{` +
			`"model":"` + modelIDInVariousCasings + `",` +
			`"messages":[],` +
			`"thinking":{"type":"adaptive"}` +
			`}`)

		out, _ := normalizeClaudeOAuthRequestBody(
			bodyWithThinkingEnabled,
			modelIDInVariousCasings,
			claudeOAuthNormalizeOptions{},
		)

		require.False(t,
			gjson.GetBytes(out, "context_management").Exists(),
			"case-insensitive Haiku 匹配在 modelID=%q 时应豁免 context_management 注入",
			modelIDInVariousCasings,
		)
		// Confirm the strings.ToLower call is intentional, not accidental
		require.True(t,
			strings.Contains(strings.ToLower(modelIDInVariousCasings), "haiku"),
			"测试输入 %q 在 lowercase 后必须包含 haiku 子串",
			modelIDInVariousCasings,
		)
	}
}
