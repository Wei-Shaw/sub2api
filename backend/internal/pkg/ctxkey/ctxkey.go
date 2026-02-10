// Package ctxkey 定义用于 context.Value 的类型安全 key
package ctxkey

// Key 定义 context key 的类型，避免使用内置 string 类型（staticcheck SA1029）
type Key string

const (
	// ForcePlatform 强制平台（用于 /antigravity 路由），由 middleware.ForcePlatform 设置
	ForcePlatform Key = "ctx_force_platform"

	// ClientRequestID 客户端请求的唯一标识，用于追踪请求全生命周期（用于 Ops 监控与排障）。
	ClientRequestID Key = "ctx_client_request_id"

	// RetryCount 表示当前请求在网关层的重试次数（用于 Ops 记录与排障）。
	RetryCount Key = "ctx_retry_count"

	// AccountSwitchCount 表示请求过程中发生的账号切换次数
	AccountSwitchCount Key = "ctx_account_switch_count"

	// IsClaudeCodeClient 标识当前请求是否来自 Claude Code 客户端
	IsClaudeCodeClient Key = "ctx_is_claude_code_client"

	// ThinkingEnabled 标识当前请求是否开启 thinking（用于 Antigravity 最终模型名推导与模型维度限流）
	ThinkingEnabled Key = "ctx_thinking_enabled"
	// Group 认证后的分组信息，由 API Key 认证中间件设置
	Group Key = "ctx_group"

	// IsMaxTokensOneHaikuRequest 标识当前请求是否为 max_tokens=1 + haiku 模型的探测请求
	// 用于 ClaudeCodeOnly 验证绕过（绕过 system prompt 检查，但仍需验证 User-Agent）
	IsMaxTokensOneHaikuRequest Key = "ctx_is_max_tokens_one_haiku"

	// SingleAccountRetry 标识当前请求处于单账号 503 退避重试模式。
	// 在此模式下，Service 层的模型限流预检查将等待限流过期而非直接切换账号。
	SingleAccountRetry Key = "ctx_single_account_retry"
)
