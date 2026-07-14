// Package service — support_chat_log_service.go
//
// SupportChatLogService 是客服对话审计的应用服务层：给 handler 提供"落一轮对话"
// 与"admin 查询"两组能力，屏蔽 Repository 细节。
//
// 落库语义（design.md D4）：
//   - PersistTurn 是旁路审计入口。它内部吞掉并记录 repo 错误吗？——不。是否记录日志
//     由 handler 决定（handler 持有 slog + ctx），service 层只负责"尽力落库并把错误
//     原样返回"，让 handler 统一 `if err != nil { slog.Warn(...) }` 且**绝不影响响应**。
package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SupportChatLogService 编排客服对话审计的落库与查询。
type SupportChatLogService struct {
	repo SupportChatLogRepository
}

// NewSupportChatLogService 构造对话审计服务。
func NewSupportChatLogService(repo SupportChatLogRepository) *SupportChatLogService {
	return &SupportChatLogService{repo: repo}
}

// PersistTurn 落一轮问答（upsert 会话 + append user/assistant 消息）。
//
// 前置：turn.SessionID 与 turn.Status 必须非空且合法，否则跳过落库（返回 nil）——
// 避免脏数据。调用方（handler）已保证在有有效 user 内容的分支才调用本方法。
func (s *SupportChatLogService) PersistTurn(ctx context.Context, turn SupportChatTurn) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if strings.TrimSpace(turn.SessionID) == "" {
		return nil
	}
	if !IsValidChatLogStatus(turn.Status) {
		return nil
	}
	return s.repo.UpsertConversationAndAppend(ctx, turn)
}

// ListConversations 返回 admin 分页列表（不含消息正文）。
func (s *SupportChatLogService) ListConversations(
	ctx context.Context,
	filters SupportChatLogListFilters,
	params pagination.PaginationParams,
) ([]SupportChatConversation, *pagination.PaginationResult, error) {
	return s.repo.ListConversations(ctx, filters, params)
}

// GetConversation 返回会话详情（含消息时间线）。
func (s *SupportChatLogService) GetConversation(ctx context.Context, id int64) (*SupportChatConversationDetail, error) {
	return s.repo.GetConversationWithMessages(ctx, id)
}
