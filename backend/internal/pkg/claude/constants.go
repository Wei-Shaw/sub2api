// Package claude provides constants and helpers for Claude API integration.
package claude

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// init 在启动时校验 CLIDefaultVersion 与 DefaultHeaders["User-Agent"] 中携带的版本号
// 必须严格一致。Anthropic 上游会比对 UA 与 billing block 中的 cc_version；若错位
// 就会判定为第三方调用（"Third-party apps now draw from your extra usage"）。
//
// 用 panic 而不是日志 warn 是故意的：一个 cc_version 错位的二进制上线，不会被监控
// 立刻发现，但每个 OAuth 账号都会持续被打 third-party 标。让进程 fail-fast 比让
// 共享池静默劣化更安全。
//
// 注意（P1-2 后）：这只校验编译期"默认版本"对齐，运行时的 cliCurrentVersion 由
// CLIVersionTrackerService 周期性更新，写入时通过 SetCLICurrentVersion 同步刷新
// DefaultHeaders["User-Agent"]，保持运行时不变量。
func init() {
	ua, ok := DefaultHeaders["User-Agent"]
	if !ok {
		panic("claude.DefaultHeaders missing User-Agent")
	}
	const prefix = "claude-cli/"
	idx := strings.Index(ua, prefix)
	if idx < 0 {
		panic(fmt.Sprintf("claude.DefaultHeaders[\"User-Agent\"]=%q is not a claude-cli UA", ua))
	}
	rest := ua[idx+len(prefix):]
	end := strings.IndexAny(rest, " (")
	if end < 0 {
		end = len(rest)
	}
	uaVersion := strings.TrimSpace(rest[:end])
	if uaVersion != CLIDefaultVersion {
		panic(fmt.Sprintf(
			"claude version mismatch: CLIDefaultVersion=%q vs DefaultHeaders[\"User-Agent\"]=%q (extracted=%q). "+
				"Both must be bumped together; see CLIDefaultVersion docstring.",
			CLIDefaultVersion, ua, uaVersion))
	}
	// 初始化运行时变量
	cliCurrentVersion = CLIDefaultVersion
}

// Claude Code 客户端相关常量

// Beta header 常量
//
// 这里的常量对齐真实 Claude Code CLI 的最新流量（截至 2026-04）。
// 选型参考：与 Parrot (src/transform/cc_mimicry.py) 的 BETAS 保持一致，
// 原因：Anthropic 上游会基于 anthropic-beta 的完整集合判定请求来源；
// 缺少任何"官方 Claude Code 请求才会带"的 beta，都会被降级到第三方额度，
// 对应报错：`Third-party apps now draw from your extra usage, not your plan limits.`
const (
	BetaOAuth                    = "oauth-2025-04-20"
	BetaClaudeCode               = "claude-code-20250219"
	BetaInterleavedThinking      = "interleaved-thinking-2025-05-14"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
	BetaTokenCounting            = "token-counting-2024-11-01"
	BetaContext1M                = "context-1m-2025-08-07"
	BetaContextManagement        = "context-management-2025-06-27"
	BetaFastMode                 = "fast-mode-2026-02-01"

	// 新增（对齐官方 CLI 2.1.9x 以来的流量）
	BetaPromptCachingScope = "prompt-caching-scope-2026-01-05"
	BetaEffort             = "effort-2025-11-24"
	BetaRedactThinking     = "redact-thinking-2026-02-12"
	BetaExtendedCacheTTL   = "extended-cache-ttl-2025-04-11"
)

// DroppedBetas 是转发时需要从 anthropic-beta header 中移除的 beta token 列表。
// 这些 token 是客户端特有的，不应透传给上游 API。
var DroppedBetas = []string{}

// NonOfficialUpstreamSafeBetaPrefixes 是对非官方上游（聚合商/自定义 base URL 账号）
// 转发时允许保留的 beta token 前缀白名单。不在此列表中的 beta token 会被剥离，
// 防止 Bedrock 等后端返回 "invalid beta flag" 错误。
//
// 使用前缀匹配：新版本号的同族 beta（如 interleaved-thinking-2025-XX-XX）
// 无需手动更新此列表即可自动放行。
//
// 维护原则：只有被主流 Bedrock/Vertex 后端证实支持的 beta 才应加入。
var NonOfficialUpstreamSafeBetaPrefixes = []string{
	"claude-code-",
	"fine-grained-tool-streaming-",
	"interleaved-thinking-",
	"token-counting-",
	"prompt-caching-scope-",
	"effort-",
	"context-1m-", // anyrouter.top requires; injected by ForceContext1M
}

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// MessageBetaHeaderNoTools /v1/messages 在无工具时的 beta header
//
// NOTE: Claude Code OAuth credentials are scoped to Claude Code. When we "mimic"
// Claude Code for non-Claude-Code clients, we must include the claude-code beta
// even if the request doesn't use tools, otherwise upstream may reject the
// request as a non-Claude-Code API request.
const MessageBetaHeaderNoTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// MessageBetaHeaderWithTools /v1/messages 在有工具时的 beta header
const MessageBetaHeaderWithTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// CountTokensBetaHeader count_tokens 请求使用的 anthropic-beta header
const CountTokensBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenCounting

// HaikuBetaHeader Haiku 模型使用的 anthropic-beta header（不需要 claude-code beta）
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// APIKeyBetaHeader API-key 账号建议使用的 anthropic-beta header（不包含 oauth）
const APIKeyBetaHeader = BetaClaudeCode + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// APIKeyHaikuBetaHeader Haiku 模型在 API-key 账号下使用的 anthropic-beta header（不包含 oauth / claude-code）
const APIKeyHaikuBetaHeader = BetaInterleavedThinking

// DefaultCacheControlTTL 是网关代理为自己生成的 cache_control 块默认使用的 ttl。
// 真实 Claude Code CLI 当前使用 "1h"，但本仓策略是"客户端透传 ttl 优先；
// 客户端缺省时统一使用 5m"，这样既不浪费 1h 缓存额度，也保留客户端自定义能力。
const DefaultCacheControlTTL = "5m"

// CLIDefaultVersion 是源码层硬编码的"出厂默认"Claude Code CLI 版本号，
// 在 init() 中用于校验源码层 DefaultHeaders["User-Agent"] 与之一致（fail-fast）。
//
// 真正运行时使用的版本号请走 GetCLICurrentVersion() / SetCLICurrentVersion() —
// CLIVersionTrackerService 启动时会从 system_settings.cli_current_version 回填，
// 每隔可配置周期从 npm 拉取最新版本并更新。
//
// **源码不变量**：CLIDefaultVersion 必须与 DefaultHeaders["User-Agent"] 中的版本号严格
// 一致；不一致 init() 直接 panic。升级流程：在同一个 PR 里修改下面两处常量，并跑一次
// `go test ./internal/pkg/claude/...`。
const CLIDefaultVersion = "2.1.161"

// CLICurrentVersion Deprecated：保留以兼容老调用方；返回当前运行时版本。
// 新代码请使用 GetCLICurrentVersion()。
//
// Deprecated: use GetCLICurrentVersion() instead.
func CLICurrentVersion() string { //nolint:revive // legacy name kept for compatibility
	return GetCLICurrentVersion()
}

var (
	// cliCurrentVersion 是运行时版本号，受 cliVersionMu 保护。
	cliCurrentVersion string
	cliVersionMu      sync.RWMutex

	// uaVersionRewriteRe 用于在 DefaultHeaders["User-Agent"] 中替换版本号片段。
	uaVersionRewriteRe = regexp.MustCompile(`claude-cli/\d+\.\d+\.\d+`)
)

// GetCLICurrentVersion 返回当前运行时的 CLI 版本号（线程安全）。
func GetCLICurrentVersion() string {
	cliVersionMu.RLock()
	defer cliVersionMu.RUnlock()
	if cliCurrentVersion == "" {
		return CLIDefaultVersion
	}
	return cliCurrentVersion
}

// SetCLICurrentVersion 更新运行时 CLI 版本号；同步刷新 DefaultHeaders["User-Agent"]。
// 传入空字符串视为重置为 CLIDefaultVersion。
//
// 仅在严格 semver `X.Y.Z` 格式时接受，否则返回 false 并保持原值（防止 npm 偶发返回
// pre-release / 错误格式时污染 UA）。
func SetCLICurrentVersion(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		v = CLIDefaultVersion
	}
	if !semverRe.MatchString(v) {
		return false
	}
	cliVersionMu.Lock()
	defer cliVersionMu.Unlock()
	cliCurrentVersion = v
	if ua, ok := DefaultHeaders["User-Agent"]; ok {
		DefaultHeaders["User-Agent"] = uaVersionRewriteRe.ReplaceAllString(ua, "claude-cli/"+v)
	}
	return true
}

// semverRe 严格 X.Y.Z 三段 semver。
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// FullClaudeCodeMimicryBetas 返回最"像"真实 Claude Code CLI 的完整 beta 列表，
// 用于 OAuth 账号伪装成 Claude Code 时使用。
// 顺序与真实 CLI 抓包一致。
//
// 使用建议：
//   - OAuth 账号 + 非 haiku：追加这整份列表，再按需保留 client 带来的 beta。
//   - OAuth 账号 + haiku：Anthropic 对 haiku 不做 third-party 判定，使用 HaikuBetaHeader 即可。
//   - API-key 账号：不要使用本函数，参见 APIKeyBetaHeader。
//   - 不默认加入 redact-thinking，避免上游抹除 thinking 内容；客户端显式传入时由合并逻辑保留。
func FullClaudeCodeMimicryBetas() []string {
	return []string{
		BetaClaudeCode,
		BetaOAuth,
		BetaInterleavedThinking,
		BetaPromptCachingScope,
		BetaEffort,
		BetaContextManagement,
		BetaExtendedCacheTTL,
	}
}

// DefaultHeaders 是 Claude Code 客户端默认请求头。
//
// Sync against real Claude CLI traffic so Anthropic does not reject OAuth
// requests as "non-CLI" third-party usage.
//
// Reference capture: Claude Code CLI 2.1.161 (external) — the x-stainless
// values below were verified against the installed Bun-compiled binary
// (package-version 0.94.0, runtime-version v24.3.0), and independently
// corroborated by public CLI captures (e.g. claude-cli/2.1.83 also reports
// X-Stainless-Runtime-Version: v24.3.0).
//
// Rules of thumb:
//   - Runtime-Version must be the real Node version the bundled CLI ships.
//     Recent CLI builds run Node v24 (LTS-track, even-numbered); only the
//     odd-numbered "current" releases (v23/v25) would mark the request as
//     "clearly not the bundled CLI".
//   - Package-Version should track @anthropic-ai/sdk's actual npm releases
//     for the impersonated CLI version (0.94.0 for 2.1.161).
//   - DO send anthropic-dangerous-direct-browser-access: the real CLI
//     constructs its SDK client with dangerouslyAllowBrowser: true, so the
//     SDK emits this header on every request. Omitting it is a third-party
//     tell. (Wire casing is all-lowercase; see service.resolveWireCasing.)
var DefaultHeaders = map[string]string{
	"User-Agent":                  "claude-cli/2.1.161 (external, cli)",
	"X-Stainless-Lang":            "js",
	"X-Stainless-Package-Version": "0.94.0",
	"X-Stainless-OS":              "Linux",
	"X-Stainless-Arch":            "arm64",
	"X-Stainless-Runtime":         "node",
	"X-Stainless-Runtime-Version": "v24.3.0",
	"X-Stainless-Retry-Count":     "0",
	"X-Stainless-Timeout":         "600",
	"X-App":                       "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
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
		ID:          "claude-fable-5",
		Type:        "model",
		DisplayName: "Claude Fable 5",
		CreatedAt:   "2026-06-09T00:00:00Z",
	},
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
		ID:          "claude-opus-4-8",
		Type:        "model",
		DisplayName: "Claude Opus 4.8",
		CreatedAt:   "2026-05-29T00:00:00Z",
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
