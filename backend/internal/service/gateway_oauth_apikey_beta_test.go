package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// 真 Claude Code 客户端用 API-key 认证打 OAuth 账号时，上游必须看到订阅号的完整
// beta 集合（客户端在 api-key 模式下会少发 oauth / dangerous-tool-use / afk-mode /
// extended-cache-ttl，缺了就是"假客户端"）。顺序必须与真机 OAuth 抓包一致。
func TestComputeFinalAnthropicBetaAPIKeyClientOnOAuthAccount(t *testing.T) {
	s := &GatewayService{}
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.311 (external, sdk-cli)")
	h.Set("anthropic-beta", "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,thinking-token-count-2026-05-13,advanced-tool-use-2025-11-20,effort-2025-11-24")

	body := []byte(`{"model":"claude-sonnet-5","thinking":{"type":"disabled"}}`)
	beta, shouldSet := s.computeFinalAnthropicBeta("oauth", false, "claude-sonnet-5", h, body, nil)
	if !shouldSet {
		t.Fatal("beta should be set")
	}
	// 真机 thinking disabled 抓包顺序：两个 per-turn token 缺席，其余原样。
	want := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,thinking-token-count-2026-05-13,advanced-tool-use-2025-11-20,effort-2025-11-24,dangerous-tool-use-2026-09-03,afk-mode-2026-01-31,extended-cache-ttl-2025-04-11"
	if beta != want {
		t.Fatalf("beta mismatch\n got=%s\nwant=%s", beta, want)
	}
	for _, tok := range []string{claude.BetaOAuth, claude.BetaDangerousToolUse, claude.BetaAfkMode, claude.BetaExtendedCacheTTL} {
		if !containsBetaToken(beta, tok) {
			t.Fatalf("missing %s", tok)
		}
	}
	// thinking disabled 时 per-turn-control / mid-conversation 必须缺席
	for _, tok := range []string{claude.BetaPerTurnControl, claude.BetaMidConversationToolChanges} {
		if containsBetaToken(beta, tok) {
			t.Fatalf("unexpected %s while thinking disabled", tok)
		}
	}
}

// thinking 打开的 api-key 客户端，两个 thinking 相关 token 必须保留且位置在 canonical 处。
func TestComputeFinalAnthropicBetaAPIKeyClientThinkingOn(t *testing.T) {
	s := &GatewayService{}
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.311 (external, sdk-cli)")
	h.Set("anthropic-beta", "claude-code-20250219,per-turn-control-2026-07-01,mid-conversation-tool-changes-2026-07-01")

	body := []byte(`{"model":"claude-sonnet-5","thinking":{"type":"adaptive"}}`)
	beta, _ := s.computeFinalAnthropicBeta("oauth", false, "claude-sonnet-5", h, body, nil)
	if beta != strings.Join(claude.FullClaudeCodeMimicryBetas(), ",") {
		t.Fatalf("beta mismatch: %s", beta)
	}
}

// 客户端本来就走 OAuth（beta 里已有 oauth token）时必须原样透传，不被改写。
func TestComputeFinalAnthropicBetaOAuthClientPassthrough(t *testing.T) {
	s := &GatewayService{}
	h := http.Header{}
	h.Set("User-Agent", "claude-cli/2.1.311 (external, sdk-cli)")
	client := "claude-code-20250219,oauth-2025-04-20,effort-2025-11-24"
	h.Set("anthropic-beta", client)

	body := []byte(`{"model":"claude-sonnet-5","thinking":{"type":"disabled"}}`)
	beta, _ := s.computeFinalAnthropicBeta("oauth", false, "claude-sonnet-5", h, body, nil)
	if beta != client {
		t.Fatalf("passthrough broken: %s", beta)
	}
}
