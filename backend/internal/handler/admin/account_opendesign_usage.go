package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openDesignUsageBindingRequest struct {
	Token       string `json:"token"`        // 设备授权签发的 Control Key，仅写入。
	WorkspaceID string `json:"workspace_id"` // 仅旧版 Cookie 绑定使用。
	Session     string `json:"session"`      // 旧版 Cookie 请求头，仅写入。
}

type openDesignDevicePollRequest struct {
	AttemptID string `json:"attempt_id"` // 发起接口返回的本次授权标识。
}

func (h *AccountHandler) openDesignUsageAccountID(c *gin.Context) (int64, bool) {
	if h.openDesignUsage == nil {
		response.ErrorFrom(c, service.ErrOpenDesignUsageUnavailable)
		return 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

// BindOpenDesignUsage 验证并保存账号的上游用量授权，只返回脱敏快照。
func (h *AccountHandler) BindOpenDesignUsage(c *gin.Context) {
	id, ok := h.openDesignUsageAccountID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req openDesignUsageBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid OpenDesign usage binding")
		return
	}
	state, err := h.openDesignUsage.Bind(c.Request.Context(), id, req.WorkspaceID, req.Session, req.Token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

// StartOpenDesignDeviceAuthorization 返回官方授权地址，不接收账号密码，不返回设备密钥。
func (h *AccountHandler) StartOpenDesignDeviceAuthorization(c *gin.Context) {
	id, ok := h.openDesignUsageAccountID(c)
	if !ok {
		return
	}
	actor, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || actor.UserID <= 0 {
		response.Unauthorized(c, "Administrator session required")
		return
	}
	prompt, err := h.openDesignUsage.StartDeviceAuthorization(c.Request.Context(), id, actor.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, prompt)
}

// PollOpenDesignDeviceAuthorization 检查官方授权结果，仅在批准并验证额度后保存 Control Key。
func (h *AccountHandler) PollOpenDesignDeviceAuthorization(c *gin.Context) {
	id, ok := h.openDesignUsageAccountID(c)
	if !ok {
		return
	}
	actor, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || actor.UserID <= 0 {
		response.Unauthorized(c, "Administrator session required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req openDesignDevicePollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid OpenDesign authorization attempt")
		return
	}
	status, err := h.openDesignUsage.PollDeviceAuthorization(c.Request.Context(), id, actor.UserID, req.AttemptID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if status.Status == "pending" {
		middleware.SkipAudit(c)
	}
	response.Success(c, status)
}

// RefreshOpenDesignUsage 查询已授权的工作区并返回最新可用快照。
func (h *AccountHandler) RefreshOpenDesignUsage(c *gin.Context) {
	id, ok := h.openDesignUsageAccountID(c)
	if !ok {
		return
	}
	state, err := h.openDesignUsage.Refresh(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, state)
}

// UnbindOpenDesignUsage 删除用量授权和快照，不影响账号推理凭证。
func (h *AccountHandler) UnbindOpenDesignUsage(c *gin.Context) {
	id, ok := h.openDesignUsageAccountID(c)
	if !ok {
		return
	}
	if err := h.openDesignUsage.Unbind(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}
