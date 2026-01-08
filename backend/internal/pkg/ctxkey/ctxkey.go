// Package ctxkey 定义用于 context.Value 的类型安全 key
package ctxkey

// Key 定义 context key 的类型，避免使用内置 string 类型（staticcheck SA1029）
type Key string

const (
	// ForcePlatform 强制平台（用于 /antigravity 路由），由 middleware.ForcePlatform 设置
	ForcePlatform Key = "ctx_force_platform"
	// IsClaudeCodeClient 是否为 Claude Code 客户端，由中间件设置
	IsClaudeCodeClient Key = "ctx_is_claude_code_client"
)
