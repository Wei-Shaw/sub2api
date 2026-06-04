package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService       service.AdminService
	chatSessionService *service.ChatSessionService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService, chatSessionService *service.ChatSessionService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService:       adminService,
		chatSessionService: chatSessionService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key.
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID             *int64 `json:"group_id"`               // nil=不修改, 0=解绑, >0=绑定到目标分组
	ResetRateLimitUsage *bool  `json:"reset_rate_limit_usage"` // true=重置 5h/1d/7d 限速用量
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}

// ListChatSessions handles admin listing chat sessions for an API key.
// GET /api/v1/admin/api-keys/:id/chat-sessions?user_id=...
func (h *AdminAPIKeyHandler) ListChatSessions(c *gin.Context) {
	if h.chatSessionService == nil {
		response.Success(c, response.PaginatedData{Items: []*service.ChatSession{}, Total: 0, Page: 1, PageSize: 20, Pages: 1})
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}
	userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	items, total, err := h.chatSessionService.ListSessionsByAPIKey(c.Request.Context(), userID, keyID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, params.Page, params.PageSize)
}

// GetChatSession handles admin loading a single chat session.
// GET /api/v1/admin/api-keys/:id/chat-sessions/:sessionId?user_id=...
func (h *AdminAPIKeyHandler) GetChatSession(c *gin.Context) {
	if h.chatSessionService == nil {
		response.NotFound(c, "Chat session feature is not enabled")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("sessionId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}
	userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), parsePositiveInt(c.Query("limit"), 20))
	if pageSize > 100 {
		pageSize = 100
	}

	item, err := h.chatSessionService.GetSessionDetail(c.Request.Context(), userID, keyID, sessionID, pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// GetChatMessage handles admin loading one full chat message payload.
// GET /api/v1/admin/api-keys/:id/chat-sessions/:sessionId/messages/:messageId?user_id=...
func (h *AdminAPIKeyHandler) GetChatMessage(c *gin.Context) {
	if h.chatSessionService == nil {
		response.NotFound(c, "Chat session feature is not enabled")
		return
	}

	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("sessionId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid session ID")
		return
	}
	messageID, err := strconv.ParseInt(c.Param("messageId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid message ID")
		return
	}
	userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	item, err := h.chatSessionService.GetChatMessageDetail(c.Request.Context(), userID, keyID, sessionID, messageID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func parsePositiveInt(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
