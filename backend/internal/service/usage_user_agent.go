package service

import (
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

var usageUserAgentProduct = regexp.MustCompile("^([!#$%&'*+.^_`|~0-9A-Za-z-]+)/([^\\s/()]+)(?:\\s|$)")

// classifyUsageUserAgent 只解析 UA 自报的客户端，不将分类结果用于鉴权。
// 特殊分类使用固定标记供前端翻译；空版本表示未知，原始 UA 始终另行保留。
func classifyUsageUserAgent(raw string) (client, version string) {
	ua := strings.TrimSpace(raw)
	if ua == "" {
		return "__missing__", ""
	}
	if strings.HasPrefix(strings.ToLower(ua), "mozilla/") {
		return "__browser__", ""
	}
	if openai.IsCodexOfficialClientRequestStrict(ua) {
		return "Codex", openai.CodexUserAgentVersion(ua)
	}
	match := usageUserAgentProduct.FindStringSubmatch(ua)
	if match == nil {
		return "__unknown__", ""
	}
	if strings.EqualFold(match[1], "claude-cli") {
		return "Claude Code", match[2]
	}
	return strings.ToLower(match[1]), match[2]
}
