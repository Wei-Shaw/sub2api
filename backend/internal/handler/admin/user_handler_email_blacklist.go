package admin

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// FindBlacklistedEmailUsersRequest 待核对的黑名单邮箱
type FindBlacklistedEmailUsersRequest struct {
	Emails []string `json:"emails"`
}

// BlacklistedEmailUser 收件箱身份命中黑名单、仍处于启用状态的普通用户
type BlacklistedEmailUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// BatchDisableUsersRequest 批量禁用用户
type BatchDisableUsersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

// FindBlacklistedEmailUsers 查找邮箱命中给定黑名单项的启用中普通用户
// POST /api/v1/admin/users/blacklist-matches
func (h *UserHandler) FindBlacklistedEmailUsers(c *gin.Context) {
	var req FindBlacklistedEmailUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.Emails) > service.MaxRegistrationEmailBlacklistSize {
		response.BadRequest(c, fmt.Sprintf("emails cannot exceed %d", service.MaxRegistrationEmailBlacklistSize))
		return
	}

	users, err := h.adminService.FindActiveUsersByBlacklistedEmails(c.Request.Context(), req.Emails)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]BlacklistedEmailUser, 0, len(users))
	for _, user := range users {
		items = append(items, BlacklistedEmailUser{ID: user.ID, Email: user.Email, Username: user.Username})
	}
	response.Success(c, gin.H{"items": items, "total": len(items)})
}

// BatchDisable 批量禁用用户（管理员与已禁用用户跳过）
// POST /api/v1/admin/users/batch-disable
func (h *UserHandler) BatchDisable(c *gin.Context) {
	var req BatchDisableUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.UserIDs) == 0 {
		response.BadRequest(c, "user_ids is required")
		return
	}
	if len(req.UserIDs) > service.MaxBatchDisableUsers {
		response.BadRequest(c, fmt.Sprintf("user_ids cannot exceed %d", service.MaxBatchDisableUsers))
		return
	}

	affected, err := h.adminService.BatchDisableUsers(c.Request.Context(), req.UserIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"affected": affected})
}
