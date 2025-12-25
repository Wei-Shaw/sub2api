package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// toResponsePagination converts pagination.PaginationResult to response.PaginationResult
func toResponsePagination(p *pagination.PaginationResult) *response.PaginationResult {
	if p == nil {
		return nil
	}
	return &response.PaginationResult{
		Total:    p.Total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Pages:    p.Pages,
	}
}

// SubscriptionHandler handles admin subscription management
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewSubscriptionHandler creates a new admin subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
	}
}

// AssignSubscriptionRequest represents assign subscription request
type AssignSubscriptionRequest struct {
	UserID       int64  `json:"user_id" binding:"required"`
	GroupID      int64  `json:"group_id" binding:"required"`
	ValidityDays int    `json:"validity_days"`
	Notes        string `json:"notes"`
}

// BulkAssignSubscriptionRequest represents bulk assign subscription request
type BulkAssignSubscriptionRequest struct {
	UserIDs      []int64 `json:"user_ids" binding:"required,min=1"`
	GroupID      int64   `json:"group_id" binding:"required"`
	ValidityDays int     `json:"validity_days"`
	Notes        string  `json:"notes"`
}

// ExtendSubscriptionRequest represents extend subscription request
type ExtendSubscriptionRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

// List handles listing all subscriptions with pagination and filters
// GET /api/v1/admin/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	// Parse optional filters
	var userID, groupID *int64
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if id, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			userID = &id
		}
	}
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil {
			groupID = &id
		}
	}
	status := c.Query("status")

	subscriptions, pagination, err := h.subscriptionService.List(c.Request.Context(), page, pageSize, userID, groupID, status)
	if err != nil {
		response.InternalError(c, "Failed to list subscriptions: "+err.Error())
		return
	}

	response.PaginatedWithResult(c, subscriptions, toResponsePagination(pagination))
}

// GetByID handles getting a subscription by ID
// GET /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) GetByID(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	subscription, err := h.subscriptionService.GetByID(c.Request.Context(), subscriptionID)
	if err != nil {
		response.NotFound(c, "Subscription not found")
		return
	}

	response.Success(c, subscription)
}

// GetProgress handles getting subscription usage progress
// GET /api/v1/admin/subscriptions/:id/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), subscriptionID)
	if err != nil {
		response.NotFound(c, "Subscription not found")
		return
	}

	response.Success(c, progress)
}

// Assign handles assigning a subscription to a user
// POST /api/v1/admin/subscriptions/assign
func (h *SubscriptionHandler) Assign(c *gin.Context) {
	var req AssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	subscription, err := h.subscriptionService.AssignSubscription(c.Request.Context(), &service.AssignSubscriptionInput{
		UserID:       req.UserID,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.BadRequest(c, "Failed to assign subscription: "+err.Error())
		return
	}

	response.Success(c, subscription)
}

// BulkAssign handles bulk assigning subscriptions to multiple users
// POST /api/v1/admin/subscriptions/bulk-assign
func (h *SubscriptionHandler) BulkAssign(c *gin.Context) {
	var req BulkAssignSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Get admin user ID from context
	adminID := getAdminIDFromContext(c)

	result, err := h.subscriptionService.BulkAssignSubscription(c.Request.Context(), &service.BulkAssignSubscriptionInput{
		UserIDs:      req.UserIDs,
		GroupID:      req.GroupID,
		ValidityDays: req.ValidityDays,
		AssignedBy:   adminID,
		Notes:        req.Notes,
	})
	if err != nil {
		response.InternalError(c, "Failed to bulk assign subscriptions: "+err.Error())
		return
	}

	response.Success(c, result)
}

// Extend handles extending a subscription
// POST /api/v1/admin/subscriptions/:id/extend
func (h *SubscriptionHandler) Extend(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	var req ExtendSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subscription, err := h.subscriptionService.ExtendSubscription(c.Request.Context(), subscriptionID, req.Days)
	if err != nil {
		response.InternalError(c, "Failed to extend subscription: "+err.Error())
		return
	}

	response.Success(c, subscription)
}

// Revoke handles revoking a subscription
// DELETE /api/v1/admin/subscriptions/:id
func (h *SubscriptionHandler) Revoke(c *gin.Context) {
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}

	err = h.subscriptionService.RevokeSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		response.InternalError(c, "Failed to revoke subscription: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Subscription revoked successfully"})
}

// ListByGroup handles listing subscriptions for a specific group
// GET /api/v1/admin/groups/:id/subscriptions
func (h *SubscriptionHandler) ListByGroup(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	subscriptions, pagination, err := h.subscriptionService.ListGroupSubscriptions(c.Request.Context(), groupID, page, pageSize)
	if err != nil {
		response.InternalError(c, "Failed to list group subscriptions: "+err.Error())
		return
	}

	response.PaginatedWithResult(c, subscriptions, toResponsePagination(pagination))
}

// ListByUser handles listing subscriptions for a specific user
// GET /api/v1/admin/users/:id/subscriptions
func (h *SubscriptionHandler) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "Failed to list user subscriptions: "+err.Error())
		return
	}

	response.Success(c, subscriptions)
}

// Helper function to get admin ID from context
func getAdminIDFromContext(c *gin.Context) int64 {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*model.User); ok && u != nil {
			return u.ID
		}
	}
	return 0
}
