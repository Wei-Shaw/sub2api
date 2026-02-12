package openai

import "strings"

// CodexCLIUserAgentPrefixes matches Codex CLI User-Agent patterns
// Examples: "codex_vscode/1.0.0", "codex_cli_rs/0.1.2"
var CodexCLIUserAgentPrefixes = []string{
	"codex_vscode/",
	"codex_cli_rs/",
REDACTED

// IsCodexCLIRequest checks if the User-Agent indicates a Codex CLI request
func IsCodexCLIRequest(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
REDACTED
	for _, prefix := range CodexCLIUserAgentPrefixes {
		normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
		if normalizedPrefix == "" {
			continue
	REDACTED
		// 优先前缀匹配；若 UA 被网关/代理拼接为复合字符串时，退化为包含匹配。
		if strings.HasPrefix(ua, normalizedPrefix) || strings.Contains(ua, normalizedPrefix) {
			return true
	REDACTED
REDACTED
	return false
REDACTED
