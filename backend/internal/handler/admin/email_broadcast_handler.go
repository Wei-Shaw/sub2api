package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// EmailBroadcastHandler handles admin bulk announcement email management.
type EmailBroadcastHandler struct {
	broadcastService *service.EmailBroadcastService
	adminService     service.AdminService
}

// NewEmailBroadcastHandler wires a new EmailBroadcastHandler.
func NewEmailBroadcastHandler(
	broadcastService *service.EmailBroadcastService,
	adminService service.AdminService,
) *EmailBroadcastHandler {
	return &EmailBroadcastHandler{
		broadcastService: broadcastService,
		adminService:     adminService,
	}
}

// CreateEmailBroadcastRequest is the body for POST /api/v1/admin/email-broadcasts.
type CreateEmailBroadcastRequest struct {
	Subject          string  `json:"subject" binding:"required"`
	Body             string  `json:"body" binding:"required"`
	BodyFormat       string  `json:"body_format" binding:"omitempty,oneof=html text"`
	RecipientsMode   string  `json:"recipients_mode" binding:"required,oneof=all selected"`
	RecipientUserIDs []int64 `json:"recipient_user_ids"`
}

// PreviewEmailBroadcastRequest is the body for POST /api/v1/admin/email-broadcasts/preview.
type PreviewEmailBroadcastRequest struct {
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	BodyFormat string `json:"body_format" binding:"omitempty,oneof=html text"`
}

// EmailBroadcastRecipientCandidate is one searchable user shown in the recipient picker.
type EmailBroadcastRecipientCandidate struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

// Create handles POST /api/v1/admin/email-broadcasts.
// 立即返回 202 + broadcast 当前状态；实际发送在后台进行。
func (h *EmailBroadcastHandler) Create(c *gin.Context) {
	var req CreateEmailBroadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	var createdBy *int64
	if subject.UserID > 0 {
		uid := subject.UserID
		createdBy = &uid
	}

	broadcast, err := h.broadcastService.Send(c.Request.Context(), service.EmailBroadcastSendInput{
		Subject:          req.Subject,
		Body:             req.Body,
		BodyFormat:       req.BodyFormat,
		RecipientsMode:   req.RecipientsMode,
		RecipientUserIDs: req.RecipientUserIDs,
		CreatedBy:        createdBy,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Accepted(c, dto.EmailBroadcastFromService(broadcast))
}

// Get returns the full broadcast (including body) for inspecting status / re-reading content.
// GET /api/v1/admin/email-broadcasts/:id
func (h *EmailBroadcastHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid broadcast id")
		return
	}

	b, err := h.broadcastService.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.EmailBroadcastFromService(b))
}

// List returns broadcast history (paginated, newest first).
// GET /api/v1/admin/email-broadcasts
func (h *EmailBroadcastHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := strings.TrimSpace(c.Query("status"))

	result, err := h.broadcastService.List(c.Request.Context(), service.EmailBroadcastListParams{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]dto.EmailBroadcastSummary, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, dto.EmailBroadcastSummaryFromService(&result.Items[i]))
	}

	response.Success(c, gin.H{
		"items":     items,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// Delete hard-deletes a broadcast record by id. Returns 409 if the broadcast is
// still pending or sending so the user retries after it settles.
// DELETE /api/v1/admin/email-broadcasts/:id
func (h *EmailBroadcastHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid broadcast id")
		return
	}
	if err := h.broadcastService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

// Preview returns the fully composed HTML that would be delivered for the given
// subject + body + format, so the admin UI can render a faithful live preview.
// POST /api/v1/admin/email-broadcasts/preview
func (h *EmailBroadcastHandler) Preview(c *gin.Context) {
	var req PreviewEmailBroadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.BodyFormat == "" {
		req.BodyFormat = "html"
	}
	htmlBody := h.broadcastService.PreviewHTML(c.Request.Context(), req.Subject, req.Body, req.BodyFormat)
	response.Success(c, gin.H{"html": htmlBody})
}

// SearchRecipients backs the recipient multi-select picker.
// GET /api/v1/admin/email-broadcasts/recipients/search?q=...&limit=...
func (h *EmailBroadcastHandler) SearchRecipients(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 200 {
		q = q[:200]
	}
	limit := 20
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	filters := service.UserListFilters{Search: q}
	// 不需要订阅信息，节省查询成本
	includeSubs := false
	filters.IncludeSubscriptions = &includeSubs

	users, _, err := h.adminService.ListUsers(c.Request.Context(), 1, limit, filters, "id", "asc")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]EmailBroadcastRecipientCandidate, 0, len(users))
	for i := range users {
		u := users[i]
		if strings.TrimSpace(u.Email) == "" {
			continue
		}
		out = append(out, EmailBroadcastRecipientCandidate{
			ID:       u.ID,
			Email:    u.Email,
			Username: u.Username,
		})
	}

	response.Success(c, gin.H{"items": out})
}
