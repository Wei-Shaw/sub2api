package admin

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UserIsolationHandler struct {
	service *service.UserIsolationLookupService
}

type userIsolationLookupRequest struct {
	AccountID   int64  `json:"account_id" binding:"required,gt=0"`
	IsolationID string `json:"isolation_id" binding:"required"`
}

func NewUserIsolationHandler(svc *service.UserIsolationLookupService) *UserIsolationHandler {
	return &UserIsolationHandler{service: svc}
}

func (h *UserIsolationHandler) Lookup(c *gin.Context) {
	middleware.SetAuditAction(c, "admin.user_isolation.lookup")
	var req userIsolationLookupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.SetAuditExtra(c, map[string]any{"result": "invalid_request"})
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.service.Lookup(c.Request.Context(), req.AccountID, req.IsolationID)
	if err != nil {
		errorCode := infraerrors.Reason(err)
		auditResult := "failed"
		if errorCode == "USER_ISOLATION_USER_NOT_FOUND" {
			auditResult = "not_found"
		}
		middleware.SetAuditExtra(c, map[string]any{"result": auditResult, "error_code": errorCode})
		response.ErrorFrom(c, err)
		return
	}
	middleware.SetAuditExtra(c, map[string]any{"result": "matched", "matched_count": 1})
	response.Success(c, result)
}
