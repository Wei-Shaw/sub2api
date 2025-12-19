package claude

// Claude Code 客户端相关常量

// Beta header 常量
const (
	BetaOAuth                  = "oauth-2025-04-20"
	BetaClaudeCode             = "claude-code-20250219"
	BetaInterleavedThinking    = "REDACTED"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
)

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// HaikuBetaHeader Haiku 模型使用的 anthropic-beta header（不需要 claude-code beta）
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// Claude Code 客户端默认请求头
var DefaultHeaders = map[string]string{
	"User-Agent":                                "claude-cli/2.0.62 (external, cli)",
	"X-Stainless-Lang":                          "js",
	"X-Stainless-Package-Version":               "0.52.0",
	"X-Stainless-OS":                            "Linux",
	"X-Stainless-Arch":                          "x64",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v22.14.0",
	"X-Stainless-Retry-Count":                   "0",
	"X-Stainless-Timeout":                       "60",
	"X-App":                                     "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
REDACTED
