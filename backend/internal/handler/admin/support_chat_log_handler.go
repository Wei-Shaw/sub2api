// Package admin — support_chat_log_handler.go
//
// admin 端客服对话记录（只读）HTTP handler。覆盖：
//
//   - GET /api/v1/admin/support/chat/conversations      分页 + 过滤（status/user_id/ip/q/from/to）
//   - GET /api/v1/admin/support/chat/conversations/:id   会话详情（整段消息时间线）
//
// 纯只读审计：admin 只能查看，不能介入/回复（要人工介入走工单）。
// admin 路由不卡 feature flag（可查存量记录），鉴权由 admin 路由组中间件保证。
package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SupportChatLogHandler 处理 admin 视角的客服对话记录接口。
type SupportChatLogHandler struct {
	service *service.SupportChatLogService
}

// NewSupportChatLogHandler 构造 admin 端对话记录 handler。
func NewSupportChatLogHandler(svc *service.SupportChatLogService) *SupportChatLogHandler {
	return &SupportChatLogHandler{service: svc}
}

// chatConversationListItem 是列表项（不含消息正文）。
type chatConversationListItem struct {
	ID         int64   `json:"id"`
	SessionID  string  `json:"session_id"`
	UserID     *int64  `json:"user_id"`
	ClientIP   string  `json:"client_ip"`
	TurnCount  int     `json:"turn_count"`
	LastStatus string  `json:"last_status"`
	FirstAt    *string `json:"first_at"`
	LastAt     *string `json:"last_at"`
	CreatedAt  string  `json:"created_at"`
}

// chatMessageItem 是详情里的单条消息。
type chatMessageItem struct {
	ID           int64  `json:"id"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	Status       string `json:"status,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Model        string `json:"model,omitempty"`
	LatencyMS    *int   `json:"latency_ms,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// chatConversationDetail 是详情响应：会话头 + 消息时间线。
type chatConversationDetail struct {
	chatConversationListItem
	Messages []chatMessageItem `json:"messages"`
}

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func chatConversationToListItem(c *service.SupportChatConversation) chatConversationListItem {
	return chatConversationListItem{
		ID:         c.ID,
		SessionID:  c.SessionID,
		UserID:     c.UserID,
		ClientIP:   c.ClientIP,
		TurnCount:  c.TurnCount,
		LastStatus: c.LastStatus,
		FirstAt:    fmtTimePtr(c.FirstAt),
		LastAt:     fmtTimePtr(c.LastAt),
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
	}
}

// List 处理 GET /api/v1/admin/support/chat/conversations。
//
// 过滤参数（任意组合）：status / user_id / ip / q（消息正文 ILIKE） / from / to（RFC3339）。
func (h *SupportChatLogHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}

	filters := service.SupportChatLogListFilters{
		Status:   strings.TrimSpace(c.Query("status")),
		ClientIP: strings.TrimSpace(c.Query("ip")),
		Search:   strings.TrimSpace(c.Query("q")),
	}
	if rawUID := strings.TrimSpace(c.Query("user_id")); rawUID != "" {
		uid, err := strconv.ParseInt(rawUID, 10, 64)
		if err != nil || uid <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filters.UserID = &uid
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid from (want RFC3339)")
			return
		}
		filters.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid to (want RFC3339)")
			return
		}
		filters.To = &t
	}
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}

	items, paginationResult, err := h.service.ListConversations(c.Request.Context(), filters, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]chatConversationListItem, 0, len(items))
	for i := range items {
		out = append(out, chatConversationToListItem(&items[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// Get 处理 GET /api/v1/admin/support/chat/conversations/:id。
func (h *SupportChatLogHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	detail, err := h.service.GetConversation(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := chatConversationDetail{
		chatConversationListItem: chatConversationToListItem(&detail.Conversation),
		Messages:                 make([]chatMessageItem, 0, len(detail.Messages)),
	}
	for i := range detail.Messages {
		m := &detail.Messages[i]
		out.Messages = append(out.Messages, chatMessageItem{
			ID:           m.ID,
			Role:         m.Role,
			Content:      m.Content,
			Status:       m.Status,
			ErrorMessage: m.ErrorMessage,
			Model:        m.Model,
			LatencyMS:    m.LatencyMS,
			CreatedAt:    m.CreatedAt.Format(time.RFC3339),
		})
	}
	response.Success(c, out)
}
