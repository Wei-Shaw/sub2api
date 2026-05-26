package handler

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type EntitlementHandler struct {
	entitlementService *service.EntitlementService
}

func NewEntitlementHandler(entitlementService *service.EntitlementService) *EntitlementHandler {
	return &EntitlementHandler{entitlementService: entitlementService}
}

type switchEntitlementRequest struct {
	GroupID  int64  `json:"group_id" binding:"required"`
	APIKeyID *int64 `json:"api_key_id,omitempty"`
}

func (h *EntitlementHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("AUTH_REQUIRED", "authentication required"))
		return
	}
	state, err := h.entitlementService.GetUserEntitlements(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *EntitlementHandler) Refresh(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("AUTH_REQUIRED", "authentication required"))
		return
	}
	state, err := h.entitlementService.RefreshUserEntitlements(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

func (h *EntitlementHandler) Switch(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("AUTH_REQUIRED", "authentication required"))
		return
	}
	var req switchEntitlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", err.Error()))
		return
	}
	result, err := h.entitlementService.SwitchEntitlement(c.Request.Context(), userID, service.SwitchEntitlementRequest{GroupID: req.GroupID, APIKeyID: req.APIKeyID})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *EntitlementHandler) AutoSwitch(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("AUTH_REQUIRED", "authentication required"))
		return
	}
	var req service.AutoSwitchEntitlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", err.Error()))
		return
	}
	result, err := h.entitlementService.AutoSwitchEntitlement(c.Request.Context(), userID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func currentUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, false
	}
	return subject.UserID, true
}
