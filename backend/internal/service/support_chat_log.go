// Package service 内的 support_chat_log 文件定义客服浮窗对话审计
// （add-support-chat-transcript-log）的领域类型与 Repository 接口。
// Repository 实现位于 internal/repository/support_chat_log_repo.go。
//
// 设计要点（design.md）：
//   - 方案 B：会话头（以客户端 session_id 为业务键）+ 逐轮消息（1:N）。
//   - 一轮问答落两行：user 行（无 status）+ assistant 回包行（带完整状态分类）。
//   - status 完整分类：success | upstream_auth | upstream_error | interrupted
//     | rate_limited | config_error。全部落库。
//   - content 落库前截断到 SupportChatLogContentMaxLen（与工单 chat_context 上限对齐）。
//   - 落库是旁路审计：写失败只 log，绝不影响给用户的响应（由 handler 层保证）。
package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ErrSupportChatConversationNotFound：会话不存在。handler 层翻成 404。
var ErrSupportChatConversationNotFound = infraerrors.NotFound(
	"SUPPORT_CHAT_CONVERSATION_NOT_FOUND",
	"support chat conversation not found",
)

// 客服对话回包状态完整分类。仅 assistant 行有值；user 行的 status 为空。
const (
	// ChatLogStatusSuccess：流正常收尾（见到 [DONE]）。
	ChatLogStatusSuccess = "success"
	// ChatLogStatusUpstreamAuth：上游返回 401（api_key 配错）。
	ChatLogStatusUpstreamAuth = "upstream_auth"
	// ChatLogStatusUpstreamError：上游非 200，或流中途读取出错。
	ChatLogStatusUpstreamError = "upstream_error"
	// ChatLogStatusInterrupted：[DONE] 前客户端断开（记录已累积的部分文本）。
	ChatLogStatusInterrupted = "interrupted"
	// ChatLogStatusRateLimited：命中限流，未打到上游。
	ChatLogStatusRateLimited = "rate_limited"
	// ChatLogStatusConfigError：LLM 凭据缺失，未打到上游。
	ChatLogStatusConfigError = "config_error"
)

// 消息角色。
const (
	ChatLogRoleUser      = "user"
	ChatLogRoleAssistant = "assistant"
)

// SupportChatLogContentMaxLen 是单条消息 content 落库上限（字符数，与工单
// SupportTicketChatContextMaxLen 对齐）。DB 列不加约束，截断在 service 层做。
const SupportChatLogContentMaxLen = 50000

// SupportChatConversation 是会话头领域模型。
type SupportChatConversation struct {
	ID         int64
	SessionID  string
	UserID     *int64 // 匿名对话为 nil
	ClientIP   string
	TurnCount  int
	LastStatus string
	FirstAt    *time.Time
	LastAt     *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SupportChatMessage 是逐轮消息领域模型。
type SupportChatMessage struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
	Status         string // 仅 assistant 行有值
	ErrorMessage   string
	Model          string
	LatencyMS      *int
	CreatedAt      time.Time
}

// SupportChatTurn 是"落一轮问答"的入参：一条 user 消息 + 一条 assistant 回包，
// 归并到 SessionID 对应的会话头。Repository 负责 upsert 会话 + append 两行消息，
// 全程在一个事务里。
type SupportChatTurn struct {
	SessionID string
	UserID    *int64
	ClientIP  string

	// UserContent 是本轮用户最新一条消息正文（截断前，由 repo 截断）。
	UserContent string

	// AssistantContent 是本轮回包（成功时为累积文本；失败时可能为空或部分文本）。
	AssistantContent string
	// Status 是本轮 assistant 状态（ChatLogStatus* 之一）。
	Status string
	// ErrorMessage 是失败细节（Status != success 时填）。
	ErrorMessage string
	// Model 是本轮上游模型。
	Model string
	// LatencyMS 是本轮耗时毫秒（可空）。
	LatencyMS *int
	// At 是本轮时间戳（用于会话头 first_at / last_at 与消息 created_at 对齐）。
	At time.Time
}

// SupportChatConversationDetail 是会话详情：会话头 + 完整消息时间线。
type SupportChatConversationDetail struct {
	Conversation SupportChatConversation
	Messages     []SupportChatMessage
}

// SupportChatLogListFilters 是 admin 列表过滤条件。所有非空字段作为 AND 加入 WHERE。
type SupportChatLogListFilters struct {
	Status   string     // 命中会话级 last_status
	UserID   *int64     // 精确匹配
	ClientIP string     // 精确匹配
	Search   string     // 命中 message content ILIKE
	From     *time.Time // last_at >= From
	To       *time.Time // last_at <= To
}

// SupportChatLogRepository 定义客服对话审计的持久化契约。
type SupportChatLogRepository interface {
	// UpsertConversationAndAppend 在单个事务内：
	//   1. 以 turn.SessionID 幂等 upsert 会话头（存在则 turn_count+1、刷新 last_status/last_at；
	//      user_id 用 COALESCE(旧, 新) 保留首次登录身份）；
	//   2. append user 行（若 UserContent 非空）+ assistant 行；
	// content 由实现层截断到 SupportChatLogContentMaxLen。
	UpsertConversationAndAppend(ctx context.Context, turn SupportChatTurn) error

	// ListConversations 返回 admin 视角分页列表（不含消息正文），按 last_at DESC 排序。
	ListConversations(
		ctx context.Context,
		filters SupportChatLogListFilters,
		params pagination.PaginationParams,
	) ([]SupportChatConversation, *pagination.PaginationResult, error)

	// GetConversationWithMessages 返回会话头 + 按 created_at ASC 的全部消息。
	// 未找到返回 ErrSupportChatConversationNotFound。
	GetConversationWithMessages(ctx context.Context, id int64) (*SupportChatConversationDetail, error)
}

// IsValidChatLogStatus 判断给定字符串是否为合法回包状态。
func IsValidChatLogStatus(s string) bool {
	switch s {
	case ChatLogStatusSuccess,
		ChatLogStatusUpstreamAuth,
		ChatLogStatusUpstreamError,
		ChatLogStatusInterrupted,
		ChatLogStatusRateLimited,
		ChatLogStatusConfigError:
		return true
	default:
		return false
	}
}
