package openai

// CodexCLIUserAgentPrefixes matches Codex CLI User-Agent patterns
// Examples: "codex_vscode/1.0.0", "codex_cli_rs/0.1.2"
var CodexCLIUserAgentPrefixes = []string{
	"codex_vscode/",
	"codex_cli_rs/",
}

// IsCodexCLIRequest checks if the User-Agent indicates a Codex CLI request
func IsCodexCLIRequest(userAgent string) bool {
	for _, prefix := range CodexCLIUserAgentPrefixes {
		if len(userAgent) >= len(prefix) && userAgent[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
