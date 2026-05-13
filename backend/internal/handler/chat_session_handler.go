package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatmessage"
	"github.com/Wei-Shaw/sub2api/ent/chatsession"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const chatSessionRetention = 30 * 24 * time.Hour

type ChatSessionHandler struct {
	client *ent.Client
}

func NewChatSessionHandler(client *ent.Client) *ChatSessionHandler {
	return &ChatSessionHandler{client: client}
}

type chatSessionDTO struct {
	ID        int64      `json:"id"`
	APIKeyID  int64      `json:"api_key_id"`
	Title     string     `json:"title"`
	Model     string     `json:"model"`
	Status    string     `json:"status"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type chatMessageDTO struct {
	ID           int64     `json:"id"`
	SessionID    int64     `json:"session_id"`
	Role         string    `json:"role"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	Model        *string   `json:"model,omitempty"`
	DurationMs   *int      `json:"duration_ms,omitempty"`
	UsageLogID   *int64    `json:"usage_log_id,omitempty"`
	ActualCost   *float64  `json:"actual_cost,omitempty"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type createChatSessionRequest struct {
	APIKeyID int64  `json:"api_key_id" binding:"required"`
	Title    string `json:"title"`
	Model    string `json:"model" binding:"required"`
}

type updateChatSessionRequest struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
	Model  *string `json:"model"`
}

type createChatMessageRequest struct {
	Role         string   `json:"role" binding:"required"`
	Content      string   `json:"content"`
	Status       string   `json:"status"`
	Model        *string  `json:"model"`
	DurationMs   *int     `json:"duration_ms"`
	UsageLogID   *int64   `json:"usage_log_id"`
	ActualCost   *float64 `json:"actual_cost"`
	ErrorMessage *string  `json:"error_message"`
}

type updateChatMessageRequest struct {
	Content      *string  `json:"content"`
	Status       *string  `json:"status"`
	Model        *string  `json:"model"`
	DurationMs   *int     `json:"duration_ms"`
	UsageLogID   *int64   `json:"usage_log_id"`
	ActualCost   *float64 `json:"actual_cost"`
	ErrorMessage *string  `json:"error_message"`
}

func (h *ChatSessionHandler) ListSessions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	sessions, err := h.client.ChatSession.Query().
		Where(
			chatsession.UserIDEQ(subject.UserID),
			chatsession.ExpiresAtGT(time.Now()),
			chatsession.DeletedAtIsNil(),
		).
		Order(ent.Desc(chatsession.FieldUpdatedAt)).
		Limit(100).
		All(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to list chat sessions")
		return
	}

	out := make([]chatSessionDTO, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, toChatSessionDTO(session))
	}
	response.Success(c, out)
}

func (h *ChatSessionHandler) CreateSession(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req createChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	title := normalizeChatTitle(req.Title)
	if title == "" {
		title = "New chat"
	}

	apiKey, err := h.client.APIKey.Get(c.Request.Context(), req.APIKeyID)
	if err != nil {
		response.NotFound(c, "API key not found")
		return
	}
	if apiKey.UserID != subject.UserID {
		response.Forbidden(c, "Not authorized to use this API key")
		return
	}

	session, err := h.client.ChatSession.Create().
		SetUserID(subject.UserID).
		SetAPIKeyID(req.APIKeyID).
		SetTitle(title).
		SetModel(strings.TrimSpace(req.Model)).
		SetExpiresAt(time.Now().Add(chatSessionRetention)).
		Save(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to create chat session")
		return
	}
	response.Success(c, toChatSessionDTO(session))
}

func (h *ChatSessionHandler) UpdateSession(c *gin.Context) {
	session, ok := h.requireOwnedSession(c)
	if !ok {
		return
	}

	var req updateChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	update := h.client.ChatSession.UpdateOneID(session.ID)
	if req.Title != nil {
		update.SetTitle(normalizeChatTitle(*req.Title))
	}
	if req.Status != nil {
		update.SetStatus(strings.TrimSpace(*req.Status))
	}
	if req.Model != nil {
		update.SetModel(strings.TrimSpace(*req.Model))
	}
	updated, err := update.Save(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to update chat session")
		return
	}
	response.Success(c, toChatSessionDTO(updated))
}

func (h *ChatSessionHandler) DeleteSession(c *gin.Context) {
	session, ok := h.requireOwnedSession(c)
	if !ok {
		return
	}

	now := time.Now()
	if _, err := h.client.ChatSession.UpdateOneID(session.ID).SetDeletedAt(now).Save(c.Request.Context()); err != nil {
		response.InternalError(c, "Failed to delete chat session")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *ChatSessionHandler) ListMessages(c *gin.Context) {
	session, ok := h.requireOwnedSession(c)
	if !ok {
		return
	}

	messages, err := h.client.ChatMessage.Query().
		Where(chatmessage.SessionIDEQ(session.ID), chatmessage.UserIDEQ(session.UserID)).
		Order(ent.Asc(chatmessage.FieldCreatedAt), ent.Asc(chatmessage.FieldID)).
		All(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to list chat messages")
		return
	}

	out := make([]chatMessageDTO, 0, len(messages))
	for _, message := range messages {
		out = append(out, toChatMessageDTO(message))
	}
	response.Success(c, out)
}

func (h *ChatSessionHandler) CreateMessage(c *gin.Context) {
	session, ok := h.requireOwnedSession(c)
	if !ok {
		return
	}

	var req createChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "completed"
	}
	create := h.client.ChatMessage.Create().
		SetSessionID(session.ID).
		SetUserID(session.UserID).
		SetRole(strings.TrimSpace(req.Role)).
		SetContent(req.Content).
		SetStatus(status)
	if req.Model != nil {
		create.SetModel(strings.TrimSpace(*req.Model))
	}
	if req.DurationMs != nil {
		create.SetDurationMs(*req.DurationMs)
	}
	if req.UsageLogID != nil {
		create.SetUsageLogID(*req.UsageLogID)
	}
	if req.ActualCost != nil {
		create.SetActualCost(*req.ActualCost)
	}
	if req.ErrorMessage != nil {
		create.SetErrorMessage(*req.ErrorMessage)
	}
	message, err := create.Save(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to create chat message")
		return
	}
	_, _ = h.client.ChatSession.UpdateOneID(session.ID).SetUpdatedAt(time.Now()).Save(c.Request.Context())
	response.Success(c, toChatMessageDTO(message))
}

func (h *ChatSessionHandler) UpdateMessage(c *gin.Context) {
	session, ok := h.requireOwnedSession(c)
	if !ok {
		return
	}
	messageID, ok := parseIDParam(c, "message_id")
	if !ok {
		return
	}

	var req updateChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	message, err := h.client.ChatMessage.Query().
		Where(chatmessage.IDEQ(messageID), chatmessage.SessionIDEQ(session.ID), chatmessage.UserIDEQ(session.UserID)).
		Only(c.Request.Context())
	if err != nil {
		response.NotFound(c, "Chat message not found")
		return
	}

	update := h.client.ChatMessage.UpdateOneID(message.ID)
	if req.Content != nil {
		update.SetContent(*req.Content)
	}
	if req.Status != nil {
		update.SetStatus(strings.TrimSpace(*req.Status))
	}
	if req.Model != nil {
		update.SetModel(strings.TrimSpace(*req.Model))
	}
	if req.DurationMs != nil {
		update.SetDurationMs(*req.DurationMs)
	}
	if req.UsageLogID != nil {
		update.SetUsageLogID(*req.UsageLogID)
	}
	if req.ActualCost != nil {
		update.SetActualCost(*req.ActualCost)
	}
	if req.ErrorMessage != nil {
		update.SetErrorMessage(*req.ErrorMessage)
	}
	updated, err := update.Save(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to update chat message")
		return
	}
	_, _ = h.client.ChatSession.UpdateOneID(session.ID).SetUpdatedAt(time.Now()).Save(c.Request.Context())
	response.Success(c, toChatMessageDTO(updated))
}

func (h *ChatSessionHandler) requireOwnedSession(c *gin.Context) (*ent.ChatSession, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return nil, false
	}
	sessionID, ok := parseIDParam(c, "id")
	if !ok {
		return nil, false
	}
	session, err := h.client.ChatSession.Get(c.Request.Context(), sessionID)
	if err != nil {
		response.NotFound(c, "Chat session not found")
		return nil, false
	}
	if session.UserID != subject.UserID {
		response.Forbidden(c, "Not authorized to access this chat session")
		return nil, false
	}
	if session.DeletedAt != nil || time.Now().After(session.ExpiresAt) {
		response.NotFound(c, "Chat session not found")
		return nil, false
	}
	return session, true
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+name)
		return 0, false
	}
	return id, true
}

func normalizeChatTitle(title string) string {
	title = strings.TrimSpace(title)
	if len([]rune(title)) > 160 {
		runes := []rune(title)
		title = string(runes[:160])
	}
	return title
}

func toChatSessionDTO(session *ent.ChatSession) chatSessionDTO {
	return chatSessionDTO{
		ID:        session.ID,
		APIKeyID:  session.APIKeyID,
		Title:     session.Title,
		Model:     session.Model,
		Status:    session.Status,
		ExpiresAt: session.ExpiresAt,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		DeletedAt: session.DeletedAt,
	}
}

func toChatMessageDTO(message *ent.ChatMessage) chatMessageDTO {
	return chatMessageDTO{
		ID:           message.ID,
		SessionID:    message.SessionID,
		Role:         message.Role,
		Content:      message.Content,
		Status:       message.Status,
		Model:        message.Model,
		DurationMs:   message.DurationMs,
		UsageLogID:   message.UsageLogID,
		ActualCost:   message.ActualCost,
		ErrorMessage: message.ErrorMessage,
		CreatedAt:    message.CreatedAt,
		UpdatedAt:    message.UpdatedAt,
	}
}
