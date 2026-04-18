// Package claude provides constants and helpers for Claude API integration.
package claude

import "math/rand/v2"

// Claude Code 客户端相关常量

// Beta header 常量
const (
	BetaOAuth                    = "oauth-2025-04-20"
	BetaClaudeCode               = "claude-code-20250219"
	BetaInterleavedThinking      = "interleaved-thinking-2025-05-14"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
	BetaTokenCounting            = "token-counting-2024-11-01"
	BetaContext1M                = "context-1m-2025-08-07"
	BetaFastMode                 = "fast-mode-2026-02-01"
	// Additions captured from Claude Code 2.1.109 (see capture_fingerprint tool).
	BetaContextManagement20250627  = "context-management-2025-06-27"
	BetaPromptCachingScope20260105 = "prompt-caching-scope-2026-01-05"
	BetaAdvisorTool20260301        = "advisor-tool-2026-03-01"
)

// DroppedBetas 是转发时需要从 anthropic-beta header 中移除的 beta token 列表。
// 这些 token 是客户端特有的，不应透传给上游 API。
var DroppedBetas = []string{}

// clientExtraBetas 是 Claude Code 基线抓包里额外出现的 beta token。
// 真实客户端稳定携带这些 beta；缺失的话上游可以靠 "UA=版本号 但 beta 不符"
// 一条规则批量扫出 mimic 流量。保持和 DefaultHeaders 的抓包版本同步。
const clientExtraBetas = "," + BetaContextManagement20250627 + "," + BetaPromptCachingScope20260105 + "," + BetaAdvisorTool20260301

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header.
//
// 2.1.111 capture (2026-04-17) no longer advertises fine-grained-tool-streaming
// — Anthropic rolled the behavior into the baseline API. 2.1.112 (latest on
// npm) is a patch bump with no observed beta-token change. Keeping the stale
// beta here would leave "UA=2.1.11x but fine-grained-tool-streaming present"
// as a one-rule scan-for-mimic flag.
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + clientExtraBetas

// MessageBetaHeaderNoTools /v1/messages 在无工具时的 beta header
//
// NOTE: Claude Code OAuth credentials are scoped to Claude Code. When we "mimic"
// Claude Code for non-Claude-Code clients, we must include the claude-code beta
// even if the request doesn't use tools, otherwise upstream may reject the
// request as a non-Claude-Code API request.
const MessageBetaHeaderNoTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + clientExtraBetas

// MessageBetaHeaderWithTools /v1/messages 在有工具时的 beta header
const MessageBetaHeaderWithTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + clientExtraBetas

// CountTokensBetaHeader count_tokens 请求使用的 anthropic-beta header
const CountTokensBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenCounting + clientExtraBetas

// HaikuBetaHeader Haiku 模型使用的 anthropic-beta header（不需要 claude-code beta）
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// APIKeyBetaHeader API-key 账号使用的 anthropic-beta header.
//
// Exactly matches the real Claude Code 2.1.111 capture against x-api-key auth
// (2.1.112 is a patch bump with no observed beta-token change):
//
//	claude-code-20250219,
//	interleaved-thinking-2025-05-14,
//	context-management-2025-06-27,
//	prompt-caching-scope-2026-01-05,
//	advisor-tool-2026-03-01
//
// Differs from DefaultBetaHeader only by the absence of oauth-2025-04-20
// (beta is auth-type conditional; non-OAuth requests never carry it).
const APIKeyBetaHeader = BetaClaudeCode + "," + BetaInterleavedThinking + clientExtraBetas

// APIKeyHaikuBetaHeader Haiku 模型在 API-key 账号下使用的 anthropic-beta header（不包含 oauth / claude-code）
const APIKeyHaikuBetaHeader = BetaInterleavedThinking

// DefaultHeaders 是 Claude Code 客户端默认请求头。
//
// Values re-verified 2026-04-17 from a live capture of Claude Code 2.1.111 on
// Node.js 24.14.1 / macOS arm64 (backend/tools/capture_fingerprint). UA bumped
// to 2.1.112 (latest on npm as of 2026-04-17) — patch-level Claude Code releases
// do not change Stainless package/runtime fields or the timeout. Bundled
// @anthropic-ai/sdk is still 0.81.0 — unchanged since the 2.1.109 capture.
// Keep these in sync with recent Claude CLI traffic to reduce the chance that
// Claude Code-scoped OAuth credentials are rejected as "non-CLI" usage.
var DefaultHeaders = map[string]string{
	"User-Agent":                  "claude-cli/2.1.112 (external, sdk-cli)",
	"X-Stainless-Lang":            "js",
	"X-Stainless-Package-Version": "0.81.0",
	"X-Stainless-OS":              "MacOS",
	"X-Stainless-Arch":            "arm64",
	"X-Stainless-Runtime":         "node",
	"X-Stainless-Runtime-Version": "v24.14.1",
	// 2.1.111 capture shows timeout=300; 2.1.109 was 600. Matches the
	// ~5-minute default @anthropic-ai/sdk recently moved to.
	"X-Stainless-Timeout": "300",
	"X-App":               "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
}

// SampleStainlessRetryCount returns a probabilistically sampled retry count
// for the X-Stainless-Retry-Count header. Real Claude CLI emits "0" on the
// vast majority of requests but occasionally "1" (after a transient failure)
// and rarely "2" (after two). A constant "0" across every request is a weak
// mimic tell — this introduces natural variance.
func SampleStainlessRetryCount() string {
	r := rand.Float64()
	switch {
	case r < 0.005:
		return "2"
	case r < 0.03:
		return "1"
	default:
		return "0"
	}
}

// Model 表示一个 Claude 模型
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels Claude Code 客户端支持的默认模型列表
var DefaultModels = []Model{
	{
		ID:          "claude-opus-4-5-20251101",
		Type:        "model",
		DisplayName: "Claude Opus 4.5",
		CreatedAt:   "2025-11-01T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-6",
		Type:        "model",
		DisplayName: "Claude Opus 4.6",
		CreatedAt:   "2026-02-06T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-7",
		Type:        "model",
		DisplayName: "Claude Opus 4.7",
		CreatedAt:   "2026-04-17T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-6",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.6",
		CreatedAt:   "2026-02-18T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-5-20250929",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.5",
		CreatedAt:   "2025-09-29T00:00:00Z",
	},
	{
		ID:          "claude-haiku-4-5-20251001",
		Type:        "model",
		DisplayName: "Claude Haiku 4.5",
		CreatedAt:   "2025-10-01T00:00:00Z",
	},
}

// DefaultModelIDs 返回默认模型的 ID 列表
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel 测试时使用的默认模型
const DefaultTestModel = "claude-sonnet-4-5-20250929"

// ModelIDOverrides Claude OAuth 请求需要的模型 ID 映射
var ModelIDOverrides = map[string]string{
	"claude-sonnet-4-5": "claude-sonnet-4-5-20250929",
	"claude-opus-4-5":   "claude-opus-4-5-20251101",
	"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
}

// ModelIDReverseOverrides 用于将上游模型 ID 还原为短名
var ModelIDReverseOverrides = map[string]string{
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	"claude-opus-4-5-20251101":   "claude-opus-4-5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
}

// NormalizeModelID 根据 Claude OAuth 规则映射模型
func NormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDOverrides[id]; ok {
		return mapped
	}
	return id
}

// DenormalizeModelID 将上游模型 ID 转换为短名
func DenormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		return mapped
	}
	return id
}

// HaikuModelShort is the short alias Claude Code's startup probe sends.
const HaikuModelShort = "claude-haiku-4-5"
