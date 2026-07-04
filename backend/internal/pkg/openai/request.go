package openai

import (
	"regexp"
	"strings"
)

// CodexCLIUserAgentPrefixes matches Codex CLI User-Agent patterns
// Examples: "codex_vscode/1.0.0", "codex_cli_rs/0.1.2"
var CodexCLIUserAgentPrefixes = []string{
	"codex_vscode/",
	"codex_cli_rs/",
}

// codexOfficialClientUAPrefixes：Codex 官方客户端家族 User-Agent 前缀（均含下划线/连字符，
// 每项都是确定字面量；不含会被 TrimSpace 退化成裸 "codex" 的空格前缀）。
// 用途：OpenAI OAuth `codex_cli_only` 访问限制判定 + passthrough 的「非官方 UA 安全兜底」
// （IsCodexOfficialClientRequest 命中即视为官方真实 UA，逐字透传、不改写）。
//
// Cursor/VSCode 扩展两种 UA：默认 `codex_vscode/`、GitHub Copilot 集成模式 `codex_vscode_copilot/`
// （取证 extension.js `IS="codex_vscode_copilot"` 经 env 注入）；交互式 TUI 自报 `codex-tui/`
// （连字符，2026-06-23 审计抽样约占真实流量 35%，必须显式列出）。`Codex Desktop/` 等 `Codex `
// 前缀家族由 codexOfficialClientFamilyPrefix 单独处理（保留空格，避免退化为裸 codex 的宽松兜底）。
var codexOfficialClientUAPrefixes = []string{
	"codex_cli_rs/",
	"codex-tui/",
	"codex_vscode/",
	"codex_vscode_copilot/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
}

// codexOfficialClientFamilyPrefix 覆盖 `Codex ` 前缀家族（Codex Desktop 等），对应 codex-rs
// is_first_party_originator 的 starts_with("Codex ")。**保留尾随空格**，并以 HasPrefix 直接比对
// 已归一化（小写 + 去首尾空格）的值——绝不能再经 normalizeCodexClientHeader 处理本前缀，否则
// 空格被 TrimSpace 去掉、退化成裸 "codex" 而把任何含 codex 的串都放行。
const codexOfficialClientFamilyPrefix = "codex "

// codexOfficialClientOriginators：Codex 官方客户端家族 originator 精确集合。
// app-server `initialize` 把 originator 设为 clientInfo.name 逐字值（codex-rs default_client.rs），
// 故官方集合是这些确定字面量；镜像 is_first_party_originator / is_first_party_chat_originator
// 并叠加 sub2api 已取证变体。用精确匹配而非「含 codex_/codex」的宽松兜底，避免 evil-codex_ 之类
// 伪造绕过（gate 仍需 UA 双因子佐证）。新官方/合作客户端经 allowed_client.go 命名预设放行，
// 或在 bump context/codex 时同步补入本集合。
var codexOfficialClientOriginators = map[string]bool{
	"codex_cli_rs":          true, // CLI 默认 DEFAULT_ORIGINATOR
	"codex-tui":             true, // 交互式 TUI（连字符，真实流量占比最高）
	"codex_vscode":          true, // VSCode/Cursor 扩展
	"codex_vscode_copilot":  true, // 扩展 GitHub Copilot 集成模式
	"codex_app":             true, // 历史保留
	"codex_chatgpt_desktop": true, // is_first_party_chat_originator
	"codex_atlas":           true, // is_first_party_chat_originator
	"codex_exec":            true, // codex exec 非交互
	"codex_sdk_ts":          true, // TypeScript SDK
}

// IsBrowserUserAgent 判断 User-Agent 是否来自浏览器（Chrome/Firefox/Safari/Edge/Opera 等）。
// 所有现代浏览器的 UA 均以 "Mozilla/" 作为前缀，CLI 工具（codex/claude/curl/postman/python-requests 等）不会。
// 该判定用于避免 Cloudflare 对浏览器型 UA 在 OpenAI 上游接口上触发 JS 质询。
func IsBrowserUserAgent(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(ua), "mozilla/")
}

// IsCodexCLIRequest checks if the User-Agent indicates a Codex CLI request
func IsCodexCLIRequest(userAgent string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(ua, CodexCLIUserAgentPrefixes)
}

// IsCodexOfficialClientRequest checks if the User-Agent indicates a Codex 官方客户端请求。
// 与 IsCodexCLIRequest 解耦，避免影响历史兼容逻辑。宽松版：官方 UA 前缀集允许 Contains 子串兜底，
// 供 passthrough（IsCodexOfficialClientByHeaders）等历史路径使用，行为不变。
func IsCodexOfficialClientRequest(userAgent string) bool {
	return isCodexOfficialClientRequest(userAgent, false)
}

// IsCodexOfficialClientRequestStrict 同 IsCodexOfficialClientRequest，但官方 UA 前缀集只做前缀
// 匹配（HasPrefix），不退化为 Contains 子串兜底——专供 codex_cli_only 访问门，收窄「浏览器前缀 +
// 中段 codex token」之类的伪造面。`Codex ` 家族前缀与 UA 尾部兜底保持一致；passthrough 仍用宽松版。
func IsCodexOfficialClientRequestStrict(userAgent string) bool {
	return isCodexOfficialClientRequest(userAgent, true)
}

// isCodexOfficialClientRequest 匹配层级（优先级由高到低）：
//  1. UA 前缀集 codexOfficialClientUAPrefixes（strict=仅 HasPrefix；否则含 Contains 子串兜底）
//  2. `Codex ` 家族前缀（保留空格，避免退化为裸 codex）
//  3. UA 尾部兜底：codex-rs 把 clientInfo.name 写入 UA 末尾括号组 `(name; version)`。
//     CODEX_INTERNAL_ORIGINATOR_OVERRIDE 只改前缀，不改尾部——可借此恢复被 override 的真实 client。
//     生产审计（10GB / 23 天）显示，originator=cccc 的真实 codex-tui 占全 openai 流量 5.3%，
//     若无此兜底则全部误拒。非官方尾部（如 evil/bash）仍被精确集拒绝。
func isCodexOfficialClientRequest(userAgent string, strict bool) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	if strict {
		if matchCodexClientHeaderStrictPrefixes(ua, codexOfficialClientUAPrefixes) {
			return true
		}
	} else if matchCodexClientHeaderPrefixes(ua, codexOfficialClientUAPrefixes) {
		return true
	}
	if strings.HasPrefix(ua, codexOfficialClientFamilyPrefix) {
		return true
	}
	// UA 尾部兜底：提取最后一个括号组里的 name 段，用官方 originator 检测器判定。
	if name := codexUATrailerName(ua); name != "" {
		return IsCodexOfficialClientOriginator(name)
	}
	return false
}

// codexUATrailerName extracts the clientInfo.name from the last parenthesized group
// of a codex-rs formatted User-Agent: `{orig}/{ver} ({os}; {arch}) {term} ({name}; {ver})`.
//
// CODEX_INTERNAL_ORIGINATOR_OVERRIDE 修改 UA 前缀（originator 段），但不修改尾部的
// `(name; version)` 括号组——该组由 codex-rs engine 写入，保留真实 clientInfo.name。
// 故从尾部提取 name 可以恢复被 override 的真实客户端标识（例如 cccc → codex-tui）。
//
// input 应为已归一化（小写 + 去首尾空格）的 UA。
// 若无法解析则返回空字符串。
func codexUATrailerName(ua string) string {
	last := strings.LastIndex(ua, "(")
	if last < 0 {
		return ""
	}
	rest := ua[last+1:]
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return ""
	}
	inner := strings.TrimSpace(rest[:closeIdx])
	if semi := strings.Index(inner, ";"); semi >= 0 {
		inner = strings.TrimSpace(inner[:semi])
	}
	return inner
}

// IsCodexOfficialClientOriginator checks if originator indicates a Codex 官方客户端请求。
// 精确集合匹配 + `Codex ` 家族前缀；不再用「含 codex」宽松兜底（避免伪造绕过）。
func IsCodexOfficialClientOriginator(originator string) bool {
	v := normalizeCodexClientHeader(originator)
	if v == "" {
		return false
	}
	if codexOfficialClientOriginators[v] {
		return true
	}
	return strings.HasPrefix(v, codexOfficialClientFamilyPrefix)
}

// IsCodexOfficialClientByHeaders checks whether the request headers indicate an
// official Codex client family request.
func IsCodexOfficialClientByHeaders(userAgent, originator string) bool {
	return IsCodexOfficialClientRequest(userAgent) || IsCodexOfficialClientOriginator(originator)
}

func normalizeCodexClientHeader(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchCodexClientHeaderPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		normalizedPrefix := normalizeCodexClientHeader(prefix)
		if normalizedPrefix == "" {
			continue
		}
		// 优先前缀匹配；若 UA/Originator 被网关拼接为复合字符串时，退化为包含匹配。
		if strings.HasPrefix(value, normalizedPrefix) || strings.Contains(value, normalizedPrefix) {
			return true
		}
	}
	return false
}

// matchCodexClientHeaderStrictPrefixes 仅前缀匹配（HasPrefix），不含 matchCodexClientHeaderPrefixes
// 的 Contains 子串兜底。用于 codex_cli_only 官方门收窄伪造面；passthrough 历史路径仍用宽松版。
// value 应为已归一化（小写 + 去首尾空格）的值。
func matchCodexClientHeaderStrictPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if p := normalizeCodexClientHeader(prefix); p != "" && strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

// codexEngineVersionPattern 提取版本段开头的三段数字 X.Y.Z（忽略 -alpha 等后缀）。
var codexEngineVersionPattern = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)

// ParseCodexEngineVersion 从 codex-rs 形态 UA 取引擎版本：
// `{originator}/{X.Y.Z} (...)`，第一个 '/' 后、首个空格或 '(' 前的三段版本。
// 该版本是 codex-rs CARGO_PKG_VERSION（引擎版本，CLI/app-server 一致）。
func ParseCodexEngineVersion(ua string) (string, bool) {
	ua = strings.TrimSpace(ua)
	slash := strings.IndexByte(ua, '/')
	if slash < 0 {
		return "", false
	}
	rest := ua[slash+1:]
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' || rest[i] == '(' {
			end = i
			break
		}
	}
	m := codexEngineVersionPattern.FindString(strings.TrimSpace(rest[:end]))
	if m == "" {
		return "", false
	}
	return m, true
}
