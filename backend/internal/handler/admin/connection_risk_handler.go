package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ConnectionRiskHandler admin REST for abnormal connection detection.
type ConnectionRiskHandler struct {
	svc *service.ConnectionRiskService
}

// NewConnectionRiskHandler constructs the handler.
func NewConnectionRiskHandler(svc *service.ConnectionRiskService) *ConnectionRiskHandler {
	return &ConnectionRiskHandler{svc: svc}
}

// GetConfig GET /api/v1/admin/connection-risk/config
func (h *ConnectionRiskHandler) GetConfig(c *gin.Context) {
	cfg, err := h.svc.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig PUT /api/v1/admin/connection-risk/config
func (h *ConnectionRiskHandler) UpdateConfig(c *gin.Context) {
	var req service.ConnectionRiskSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.svc.UpdateConfig(c.Request.Context(), &req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, &req)
}

// GetRuntime GET /api/v1/admin/connection-risk/runtime
func (h *ConnectionRiskHandler) GetRuntime(c *gin.Context) {
	response.Success(c, h.svc.RuntimeSnapshot(c.Request.Context()))
}

// ListEvents GET /api/v1/admin/connection-risk/events
func (h *ConnectionRiskHandler) ListEvents(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := &service.ConnectionRiskEventFilter{
		Page:     page,
		PageSize: pageSize,
		Status:   strings.TrimSpace(c.Query("status")),
		Severity: strings.TrimSpace(c.Query("severity")),
		Rule:     strings.TrimSpace(c.Query("rule")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid api_key_id")
			return
		}
		filter.APIKeyID = &id
	}
	result, err := h.svc.ListEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, result.Items, result.Total, result.Page, result.PageSize)
}

// GetEvent GET /api/v1/admin/connection-risk/events/:id
func (h *ConnectionRiskHandler) GetEvent(c *gin.Context) {
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	item, err := h.svc.GetEvent(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if item == nil {
		response.NotFound(c, "Event not found")
		return
	}
	response.Success(c, item)
}

// AckEvent POST /api/v1/admin/connection-risk/events/:id/ack
func (h *ConnectionRiskHandler) AckEvent(c *gin.Context) {
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.AckEvent(c.Request.Context(), id, jwtActorID(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "status": service.ConnectionRiskStatusAcknowledged})
}

// ResolveEvent POST /api/v1/admin/connection-risk/events/:id/resolve
func (h *ConnectionRiskHandler) ResolveEvent(c *gin.Context) {
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.ResolveEvent(c.Request.Context(), id, jwtActorID(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "status": service.ConnectionRiskStatusResolved})
}

// SuppressEvent POST /api/v1/admin/connection-risk/events/:id/suppress
func (h *ConnectionRiskHandler) SuppressEvent(c *gin.Context) {
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.SuppressEvent(c.Request.Context(), id, jwtActorID(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "status": service.ConnectionRiskStatusSuppressed})
}

// DeleteEvent DELETE /api/v1/admin/connection-risk/events/:id
func (h *ConnectionRiskHandler) DeleteEvent(c *gin.Context) {
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteEvent(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": id})
}

// Exempt POST /api/v1/admin/connection-risk/actions/exempt
func (h *ConnectionRiskHandler) Exempt(c *gin.Context) {
	var req struct {
		Scope  string `json:"scope" binding:"required"` // k | u
		ID     int64  `json:"id" binding:"required"`
		Reason string `json:"reason"`
		TTLSec int    `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	ttl := time.Duration(req.TTLSec) * time.Second
	if err := h.svc.ExemptSubject(c.Request.Context(), req.Scope, req.ID, req.Reason, ttl); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"scope": req.Scope, "id": req.ID})
}

// ClearExempt DELETE /api/v1/admin/connection-risk/actions/exempt/:scope/:id
func (h *ConnectionRiskHandler) ClearExempt(c *gin.Context) {
	scope := strings.TrimSpace(c.Param("scope"))
	id, ok := parseConnectionRiskID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.ClearExempt(c.Request.Context(), scope, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"cleared": true})
}

// WhitelistIP POST /api/v1/admin/connection-risk/actions/whitelist-ip
func (h *ConnectionRiskHandler) WhitelistIP(c *gin.Context) {
	var req struct {
		APIKeyID int64    `json:"api_key_id" binding:"required"`
		IPs      []string `json:"ips" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	key, err := h.svc.WhitelistIPs(c.Request.Context(), req.APIKeyID, req.IPs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"api_key_id":   req.APIKeyID,
		"ip_whitelist": key.IPWhitelist,
	})
}

// RunRetention POST /api/v1/admin/connection-risk/actions/run-retention
func (h *ConnectionRiskHandler) RunRetention(c *gin.Context) {
	n, err := h.svc.RunRetention(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": n})
}

func parseConnectionRiskID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+name)
		return 0, false
	}
	return id, true
}

func jwtActorID(c *gin.Context) *int64 {
	if sub, ok := middleware.GetAuthSubjectFromContext(c); ok && sub.UserID > 0 {
		uid := sub.UserID
		return &uid
	}
	return nil
}
