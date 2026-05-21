package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type PlaygroundHandler struct {
	playgroundService *service.PlaygroundService
}

func NewPlaygroundHandler(playgroundService *service.PlaygroundService) *PlaygroundHandler {
	return &PlaygroundHandler{playgroundService: playgroundService}
}

type playgroundCreateChatSessionRequest struct {
	Title        string         `json:"title"`
	Model        string         `json:"model"`
	APIKeyID     *int64         `json:"api_key_id"`
	SystemPrompt string         `json:"system_prompt"`
	UseContext   *bool          `json:"use_context"`
	Metadata     map[string]any `json:"metadata"`
}

type playgroundUpdateChatSessionRequest struct {
	Title        *string        `json:"title"`
	Model        *string        `json:"model"`
	APIKeyID     *int64         `json:"api_key_id"`
	SystemPrompt *string        `json:"system_prompt"`
	UseContext   *bool          `json:"use_context"`
	Metadata     map[string]any `json:"metadata"`
}

type playgroundCreateChatMessageRequest struct {
	APIKeyID    *int64           `json:"api_key_id"`
	Role        string           `json:"role" binding:"required"`
	Model       string           `json:"model"`
	Content     string           `json:"content"`
	ContentJSON map[string]any   `json:"content_json"`
	Images      []map[string]any `json:"images"`
	Usage       map[string]any   `json:"usage"`
	Status      string           `json:"status"`
	Error       string           `json:"error"`
	DurationMS  *int             `json:"duration_ms"`
	Metadata    map[string]any   `json:"metadata"`
}

type playgroundCreateImageTaskRequest struct {
	APIKeyID        *int64           `json:"api_key_id"`
	Model           string           `json:"model"`
	Prompt          string           `json:"prompt"`
	Quality         string           `json:"quality"`
	Size            string           `json:"size"`
	N               int              `json:"n"`
	Endpoint        string           `json:"endpoint"`
	Status          string           `json:"status"`
	Request         map[string]any   `json:"request"`
	ReferenceImages []map[string]any `json:"reference_images"`
	ResultImages    []map[string]any `json:"result_images"`
	Response        map[string]any   `json:"response"`
	Error           string           `json:"error"`
	Usage           map[string]any   `json:"usage"`
	Cost            float64          `json:"cost"`
	DurationMS      *int             `json:"duration_ms"`
	Metadata        map[string]any   `json:"metadata"`
}

type playgroundUpdateImageTaskRequest struct {
	Status       *string          `json:"status"`
	ResultImages []map[string]any `json:"result_images"`
	Response     map[string]any   `json:"response"`
	Error        *string          `json:"error"`
	Usage        map[string]any   `json:"usage"`
	Cost         *float64         `json:"cost"`
	DurationMS   *int             `json:"duration_ms"`
	Metadata     map[string]any   `json:"metadata"`
}

func (h *PlaygroundHandler) ListChatSessions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.playgroundService.ListChatSessions(c.Request.Context(), subject.UserID, pagination.PaginationParams{Page: page, PageSize: pageSize}, service.PlaygroundChatSessionListFilters{})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.PlaygroundChatSessionsFromService(items), result.Total, result.Page, result.PageSize)
}

func (h *PlaygroundHandler) CreateChatSession(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req playgroundCreateChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	useContext := true
	if req.UseContext != nil {
		useContext = *req.UseContext
	}
	item, err := h.playgroundService.CreateChatSession(c.Request.Context(), subject.UserID, service.CreatePlaygroundChatSessionRequest{Title: req.Title, Model: req.Model, APIKeyID: req.APIKeyID, SystemPrompt: req.SystemPrompt, UseContext: useContext, Metadata: req.Metadata})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.PlaygroundChatSessionFromService(item))
}

func (h *PlaygroundHandler) UpdateChatSession(c *gin.Context) {
	subject, sessionID, ok := h.subjectAndID(c, "session_id")
	if !ok {
		return
	}
	var req playgroundUpdateChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.playgroundService.UpdateChatSession(c.Request.Context(), subject.UserID, sessionID, service.UpdatePlaygroundChatSessionRequest{Title: req.Title, Model: req.Model, APIKeyID: req.APIKeyID, SystemPrompt: req.SystemPrompt, UseContext: req.UseContext, Metadata: req.Metadata})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PlaygroundChatSessionFromService(item))
}

func (h *PlaygroundHandler) DeleteChatSession(c *gin.Context) {
	subject, sessionID, ok := h.subjectAndID(c, "session_id")
	if !ok {
		return
	}
	if err := h.playgroundService.DeleteChatSession(c.Request.Context(), subject.UserID, sessionID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *PlaygroundHandler) ListChatMessages(c *gin.Context) {
	subject, sessionID, ok := h.subjectAndID(c, "session_id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.playgroundService.ListChatMessages(c.Request.Context(), subject.UserID, sessionID, pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.PlaygroundChatMessagesFromService(items), result.Total, result.Page, result.PageSize)
}

func (h *PlaygroundHandler) CreateChatMessage(c *gin.Context) {
	subject, sessionID, ok := h.subjectAndID(c, "session_id")
	if !ok {
		return
	}
	var req playgroundCreateChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.playgroundService.CreateChatMessage(c.Request.Context(), subject.UserID, sessionID, service.CreatePlaygroundChatMessageRequest{APIKeyID: req.APIKeyID, Role: req.Role, Model: req.Model, Content: req.Content, ContentJSON: req.ContentJSON, Images: req.Images, Usage: req.Usage, Status: req.Status, Error: req.Error, DurationMS: req.DurationMS, Metadata: req.Metadata, TouchSession: true})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.PlaygroundChatMessageFromService(item))
}

func (h *PlaygroundHandler) ListImageTasks(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.playgroundService.ListImageTasks(c.Request.Context(), subject.UserID, pagination.PaginationParams{Page: page, PageSize: pageSize}, service.PlaygroundImageTaskListFilters{Status: c.Query("status")})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.PlaygroundImageTasksFromService(items), result.Total, result.Page, result.PageSize)
}

func (h *PlaygroundHandler) CreateImageTask(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req playgroundCreateImageTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.playgroundService.CreateImageTask(c.Request.Context(), subject.UserID, service.CreatePlaygroundImageTaskRequest{APIKeyID: req.APIKeyID, Model: req.Model, Prompt: req.Prompt, Quality: req.Quality, Size: req.Size, N: req.N, Endpoint: req.Endpoint, Status: req.Status, Request: req.Request, ReferenceImages: req.ReferenceImages, ResultImages: req.ResultImages, Response: req.Response, Error: req.Error, Usage: req.Usage, Cost: req.Cost, DurationMS: req.DurationMS, Metadata: req.Metadata})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.PlaygroundImageTaskFromService(item))
}

func (h *PlaygroundHandler) GetImageTask(c *gin.Context) {
	subject, taskID, ok := h.subjectAndID(c, "task_id")
	if !ok {
		return
	}
	item, err := h.playgroundService.GetImageTask(c.Request.Context(), subject.UserID, taskID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PlaygroundImageTaskFromService(item))
}

func (h *PlaygroundHandler) UpdateImageTask(c *gin.Context) {
	subject, taskID, ok := h.subjectAndID(c, "task_id")
	if !ok {
		return
	}
	var req playgroundUpdateImageTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.playgroundService.UpdateImageTask(c.Request.Context(), subject.UserID, taskID, service.UpdatePlaygroundImageTaskRequest{Status: req.Status, ResultImages: req.ResultImages, Response: req.Response, Error: req.Error, Usage: req.Usage, Cost: req.Cost, DurationMS: req.DurationMS, Metadata: req.Metadata})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.PlaygroundImageTaskFromService(item))
}

func (h *PlaygroundHandler) DeleteImageTask(c *gin.Context) {
	subject, taskID, ok := h.subjectAndID(c, "task_id")
	if !ok {
		return
	}
	if err := h.playgroundService.DeleteImageTask(c.Request.Context(), subject.UserID, taskID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *PlaygroundHandler) subjectAndID(c *gin.Context, param string) (middleware2.AuthSubject, int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return middleware2.AuthSubject{}, 0, false
	}
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ID")
		return middleware2.AuthSubject{}, 0, false
	}
	return subject, id, true
}
