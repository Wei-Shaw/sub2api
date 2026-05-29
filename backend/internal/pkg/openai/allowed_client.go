package openai

import "strings"

// 命名预设 ID。账号侧 codex_cli_only_allowed_clients 只能引用这些预设键，
// 具体匹配规则固化在下方 registry 中，配置只能「选择启用哪些预设」、不能自定义规则，
// 以防该白名单退化为可任意放宽的后门。
const (
	// AllowedClientClaudeCode 对应 Claude Code CLI 的 codex 插件。
	AllowedClientClaudeCode = "claude_code"
)

// AllowedClientEntry 描述一个被额外放行的非官方 Codex 客户端签名。
// Originator 必须精确等值匹配（归一化后）。
// UAContains 为必填字段：列表为空，或列表中存在任何空白 marker，均视为非法配置，
// 整体安全失败（return false）；每一项都必须出现在 User-Agent 中。
// 这确保双因子匹配不会因缺失 UA 声明而退化为仅凭可伪造的 originator 单因子放行。
type AllowedClientEntry struct {
	Originator string
	UAContains []string
}

// allowedClientRegistry 固化各命名预设的签名规则。
//
// Claude Code codex 插件签名来源：插件以 clientInfo.name="Claude Code" 完成 app-server
// initialize 握手，codex 据此把 originator 设为 "Claude Code"，User-Agent 前缀同样为
// "Claude Code/"（两者同源）。若上游 Claude Code 插件更改 clientInfo.name，此处需同步更新。
var allowedClientRegistry = map[string]AllowedClientEntry{
	AllowedClientClaudeCode: {
		Originator: "Claude Code",
		UAContains: []string{"Claude Code/"},
	},
}

// IsAllowedClientMatch 判断请求头是否命中给定的额外客户端签名。
// originator 必须精确等值（归一化后）；UAContains 中每一项都必须出现在 UA 中。
// UAContains 为必填：列表为空或含任何空白 marker 均视为非法配置，整体安全失败。
func IsAllowedClientMatch(userAgent, originator string, entry AllowedClientEntry) bool {
	wantOriginator := normalizeCodexClientHeader(entry.Originator)
	if wantOriginator == "" {
		return false
	}
	if normalizeCodexClientHeader(originator) != wantOriginator {
		return false
	}
	// 预设必须声明 UA 特征：否则将退化为仅凭可伪造的 originator 单因子匹配。
	if len(entry.UAContains) == 0 {
		return false
	}
	ua := normalizeCodexClientHeader(userAgent)
	for _, marker := range entry.UAContains {
		normalizedMarker := normalizeCodexClientHeader(marker)
		if normalizedMarker == "" {
			// 空白 marker 让该项失去校验能力，会让双因子退化为仅 originator
			// 单因子；视为非法配置，安全失败。
			return false
		}
		if !strings.Contains(ua, normalizedMarker) {
			return false
		}
	}
	return true
}

// MatchAllowedClients 判断请求头是否命中 clientIDs 引用的任一预设签名。
// 未知预设 ID 会被忽略；空列表恒不放行（默认拒绝）。
func MatchAllowedClients(userAgent, originator string, clientIDs []string) bool {
	for _, id := range clientIDs {
		entry, ok := allowedClientRegistry[normalizeCodexClientHeader(id)]
		if !ok {
			continue
		}
		if IsAllowedClientMatch(userAgent, originator, entry) {
			return true
		}
	}
	return false
}
