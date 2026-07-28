package admin

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AIAgentHandler struct {
	service *service.AIAgentService
}

func NewAIAgentHandler(agentService *service.AIAgentService) *AIAgentHandler {
	return &AIAgentHandler{service: agentService}
}

func (h *AIAgentHandler) actor(c *gin.Context) (service.AIAgentActor, bool) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Authorization required")
		return service.AIAgentActor{}, false
	}
	return service.AIAgentActor{
		UserID:      subject.UserID,
		Concurrency: subject.Concurrency,
		Email:       c.GetString(servermiddleware.ContextKeyAuthEmail),
		SessionID:   c.GetString(servermiddleware.ContextKeySessionID),
	}, true
}

func (h *AIAgentHandler) RequireEnabled(c *gin.Context) {
	config, err := h.service.Config(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		c.Abort()
		return
	}
	if !config.Enabled {
		response.Error(c, http.StatusServiceUnavailable, service.ErrAIAgentDisabled.Error())
		c.Abort()
		return
	}
	c.Next()
}

func (h *AIAgentHandler) GetConfig(c *gin.Context) {
	config, err := h.service.Config(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *AIAgentHandler) UpdateConfig(c *gin.Context) {
	var input service.UpdateAIAgentConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid Agent configuration")
		return
	}
	config, err := h.service.UpdateConfig(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *AIAgentHandler) RollbackCapabilities(c *gin.Context) {
	response.Success(c, gin.H{"operations": h.service.RollbackCapabilities()})
}

func (h *AIAgentHandler) ListModels(c *gin.Context) {
	models, err := h.service.ListModels(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, gin.H{"models": models})
}

func (h *AIAgentHandler) ListConversations(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Conversations(c.Request.Context(), actor.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) CreateConversation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.CreateConversation(c.Request.Context(), actor.UserID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) DeleteConversation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.DeleteConversation(c.Request.Context(), actor.UserID, c.Param("conversationID")); err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *AIAgentHandler) GetSession(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Session(c.Request.Context(), actor.UserID, c.Query("conversation_id"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) ClearSession(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	conversationID := c.Query("conversation_id")
	if conversationID == "" {
		response.Error(c, http.StatusBadRequest, "conversation_id is required")
		return
	}
	if err := h.service.DeleteConversation(c.Request.Context(), actor.UserID, conversationID); err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
	}
	response.Success(c, gin.H{"cleared": true})
}

func (h *AIAgentHandler) Chat(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var input struct {
		Message        string `json:"message"`
		ConversationID string `json:"conversation_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Message) == "" {
		response.Error(c, http.StatusBadRequest, "Message is required")
		return
	}
	result, err := h.service.StartChat(c.Request.Context(), actor, input.ConversationID, input.Message)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) Stop(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var input struct {
		ConversationID string `json:"conversation_id"`
	}
	if c.ShouldBindJSON(&input) != nil || input.ConversationID == "" {
		response.Error(c, http.StatusBadRequest, "conversation_id is required")
		return
	}
	response.Success(c, gin.H{"stopping": h.service.Stop(actor.UserID, input.ConversationID)})
}

func (h *AIAgentHandler) Confirm(c *gin.Context) {
	h.confirm(c, false)
}

func (h *AIAgentHandler) ConfirmSensitive(c *gin.Context) {
	h.confirm(c, true)
}

func (h *AIAgentHandler) confirm(c *gin.Context, stepUpConfirmed bool) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Confirm(c.Request.Context(), actor, c.Query("conversation_id"), c.Param("id"), stepUpConfirmed)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) Cancel(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.Cancel(c.Request.Context(), actor.UserID, c.Query("conversation_id"), c.Param("id")); err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
	}
	response.Success(c, gin.H{"cancelled": true})
}

func (h *AIAgentHandler) PreviewRollback(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	preview, err := h.service.PreviewRollback(c.Request.Context(), actor, c.Query("conversation_id"), c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error())
		return
	}
	response.Success(c, preview)
}

func (h *AIAgentHandler) Rollback(c *gin.Context) {
	h.rollback(c, false)
}

func (h *AIAgentHandler) RollbackSensitive(c *gin.Context) {
	h.rollback(c, true)
}

func (h *AIAgentHandler) rollback(c *gin.Context, stepUpConfirmed bool) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Rollback(c.Request.Context(), actor, c.Query("conversation_id"), c.Param("id"), stepUpConfirmed)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *AIAgentHandler) AssistRollback(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var input struct {
		Instruction string `json:"instruction"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid rollback assistance request")
		return
	}
	snapshot, err := h.service.AssistRollback(c.Request.Context(), actor, c.Query("conversation_id"), c.Param("id"), input.Instruction)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, snapshot)
}
