package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAISessionsService interface {
	ListActiveSessions(context.Context, int64) (*service.OpenAIActiveSessions, error)
	RevokeActiveSession(context.Context, int64, string) error
}

func (h *OpenAIOAuthHandler) sessionsAccountID(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "账号 ID 无效")
		return 0, false
	}
	if h.sessionsService == nil {
		response.BadRequest(c, "ChatGPT 活动会话服务未启用")
		return 0, false
	}
	return id, true
}

// ListActiveSessions returns sanitized ChatGPT login sessions, not scheduler sessions.
func (h *OpenAIOAuthHandler) ListActiveSessions(c *gin.Context) {
	id, ok := h.sessionsAccountID(c)
	if !ok {
		return
	}
	result, err := h.sessionsService.ListActiveSessions(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenAIOAuthHandler) RevokeActiveSession(c *gin.Context) {
	id, ok := h.sessionsAccountID(c)
	if !ok {
		return
	}
	var input struct {
		SessionID string `json:"session_id" binding:"required,max=256"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "请选择有效的 ChatGPT 会话")
		return
	}
	if err := h.sessionsService.RevokeActiveSession(c.Request.Context(), id, input.SessionID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revoked": true})
}
