package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ChatMessage struct {
	ID               int64           `json:"id"`
	SessionID        int64           `json:"session_id"`
	Seq              int             `json:"seq"`
	Role             string          `json:"role"`
	Direction        string          `json:"direction"`
	ContentText      string          `json:"content_text"`
	ContentJSON      json.RawMessage `json:"content_json,omitempty"`
	HasContentJSON   bool            `json:"has_content_json,omitempty"`
	ContentJSONBytes int64           `json:"content_json_bytes,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type ChatSession struct {
	ID               int64       `json:"id"`
	SessionKey       *string     `json:"session_key,omitempty"`
	RequestID        *string     `json:"request_id,omitempty"`
	UserID           int64       `json:"user_id"`
	APIKeyID         int64       `json:"api_key_id"`
	AccountID        *int64      `json:"account_id,omitempty"`
	GroupID          *int64      `json:"group_id,omitempty"`
	Platform         string      `json:"platform"`
	Model            string      `json:"model"`
	RequestedModel   *string     `json:"requested_model,omitempty"`
	UpstreamModel    *string     `json:"upstream_model,omitempty"`
	InboundEndpoint  *string     `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint *string     `json:"upstream_endpoint,omitempty"`
	RequestType      RequestType `json:"request_type"`
	Stream           bool        `json:"stream"`
	Status           string      `json:"status"`
	HTTPStatusCode   int         `json:"http_status_code"`
	UserPreview      *string     `json:"user_preview,omitempty"`
	AssistantPreview *string     `json:"assistant_preview,omitempty"`
	MessageCount     int         `json:"message_count"`
	CreatedAt        time.Time   `json:"created_at"`
}

type ChatSessionDetail struct {
	ChatSession
	Messages     []ChatMessage       `json:"messages"`
	MessagesPage ChatMessagePageData `json:"messages_page"`
}

type ChatMessagePageData struct {
	Items    []ChatMessage `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Pages    int           `json:"pages"`
}

type ChatMessageEvent struct {
	ID          int64           `json:"id"`
	SessionID   int64           `json:"session_id"`
	Seq         int             `json:"seq"`
	Kind        string          `json:"kind"`
	Role        string          `json:"role"`
	Direction   string          `json:"direction"`
	ContentText string          `json:"content_text"`
	ContentJSON json.RawMessage `json:"content_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type ChatSessionRecordInput struct {
	SessionKey       string
	RequestID        string
	UserID           int64
	APIKeyID         int64
	AccountID        *int64
	GroupID          *int64
	Platform         string
	Model            string
	RequestedModel   *string
	UpstreamModel    *string
	InboundEndpoint  *string
	UpstreamEndpoint *string
	RequestType      RequestType
	Stream           bool
	Status           string
	HTTPStatusCode   int
	Messages         []ChatMessageRecordInput
	Events           []ChatMessageEventRecordInput
	CreatedAt        time.Time
}

type ChatMessageRecordInput struct {
	Role        string
	Direction   string
	ContentText string
	ContentJSON json.RawMessage
}

type ChatMessageEventRecordInput struct {
	Kind        string
	Role        string
	Direction   string
	ContentText string
	ContentJSON json.RawMessage
}

type ChatSessionRepository interface {
	CreateSessionWithMessages(ctx context.Context, input *ChatSessionRecordInput) error
	ListSessionsByAPIKey(ctx context.Context, userID, apiKeyID int64, params pagination.PaginationParams) ([]*ChatSession, int64, error)
	GetSessionDetail(ctx context.Context, userID, apiKeyID, sessionID int64, params pagination.PaginationParams) (*ChatSessionDetail, error)
	GetChatMessageDetail(ctx context.Context, userID, apiKeyID, sessionID, messageID int64) (*ChatMessage, error)
	ListRecentMessagesByAPIKey(ctx context.Context, userID, apiKeyID int64, limit int) ([]ChatMessage, error)
}

type ChatSessionService struct {
	repo ChatSessionRepository
}

func NewChatSessionService(repo ChatSessionRepository) *ChatSessionService {
	return &ChatSessionService{repo: repo}
}

func (s *ChatSessionService) RecordSession(ctx context.Context, input *ChatSessionRecordInput) error {
	if s == nil || s.repo == nil || input == nil {
		return nil
	}
	if input.UserID <= 0 || input.APIKeyID <= 0 {
		return nil
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now()
	}
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "completed"
	}

	filtered := make([]ChatMessageRecordInput, 0, len(input.Messages))
	for _, msg := range input.Messages {
		if strings.TrimSpace(msg.ContentText) == "" && len(msg.ContentJSON) == 0 {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "assistant"
		}
		direction := strings.TrimSpace(msg.Direction)
		if direction == "" {
			direction = "outbound"
		}
		filtered = append(filtered, ChatMessageRecordInput{
			Role:        role,
			Direction:   direction,
			ContentText: strings.TrimSpace(msg.ContentText),
			ContentJSON: msg.ContentJSON,
		})
	}
	filteredEvents := make([]ChatMessageEventRecordInput, 0, len(input.Events))
	for _, ev := range input.Events {
		if strings.TrimSpace(ev.ContentText) == "" && len(ev.ContentJSON) == 0 {
			continue
		}
		kind := strings.TrimSpace(ev.Kind)
		if kind == "" {
			kind = "aux"
		}
		role := strings.TrimSpace(ev.Role)
		if role == "" {
			role = "system"
		}
		direction := strings.TrimSpace(ev.Direction)
		if direction == "" {
			direction = "inbound"
		}
		filteredEvents = append(filteredEvents, ChatMessageEventRecordInput{
			Kind:        kind,
			Role:        role,
			Direction:   direction,
			ContentText: strings.TrimSpace(ev.ContentText),
			ContentJSON: ev.ContentJSON,
		})
	}
	if len(filtered) == 0 && len(filteredEvents) == 0 {
		return nil
	}
	input.Messages = filtered
	input.Events = filteredEvents
	return s.repo.CreateSessionWithMessages(ctx, input)
}

func (s *ChatSessionService) ListSessionsByAPIKey(ctx context.Context, userID, apiKeyID int64, params pagination.PaginationParams) ([]*ChatSession, int64, error) {
	if s == nil || s.repo == nil {
		return []*ChatSession{}, 0, nil
	}
	if userID <= 0 || apiKeyID <= 0 {
		return nil, 0, infraerrors.BadRequest("INVALID_CHAT_SESSION_SCOPE", "invalid chat session scope")
	}
	return s.repo.ListSessionsByAPIKey(ctx, userID, apiKeyID, params)
}

func (s *ChatSessionService) GetSessionDetail(ctx context.Context, userID, apiKeyID, sessionID int64, params pagination.PaginationParams) (*ChatSessionDetail, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.NotFound("CHAT_SESSION_NOT_FOUND", "chat session not found")
	}
	if userID <= 0 || apiKeyID <= 0 || sessionID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CHAT_SESSION_SCOPE", "invalid chat session scope")
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	return s.repo.GetSessionDetail(ctx, userID, apiKeyID, sessionID, params)
}

func (s *ChatSessionService) GetChatMessageDetail(ctx context.Context, userID, apiKeyID, sessionID, messageID int64) (*ChatMessage, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.NotFound("CHAT_MESSAGE_NOT_FOUND", "chat message not found")
	}
	if userID <= 0 || apiKeyID <= 0 || sessionID <= 0 || messageID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CHAT_MESSAGE_SCOPE", "invalid chat message scope")
	}
	return s.repo.GetChatMessageDetail(ctx, userID, apiKeyID, sessionID, messageID)
}

func (s *ChatSessionService) ListRecentMessagesByAPIKey(ctx context.Context, userID, apiKeyID int64, limit int) ([]ChatMessage, error) {
	if s == nil || s.repo == nil {
		return []ChatMessage{}, nil
	}
	if userID <= 0 || apiKeyID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CHAT_MESSAGE_SCOPE", "invalid chat message scope")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.repo.ListRecentMessagesByAPIKey(ctx, userID, apiKeyID, limit)
}
