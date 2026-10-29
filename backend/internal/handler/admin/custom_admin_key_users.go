package admin

import (
	"slices"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// adminUserResponse 对受限密钥使用字段白名单，避免完整管理员 DTO 新增字段时隐式扩大授权。
// 订阅及用量由各自的独立接口提供，用户资料接口不嵌入这些关联数据。
func adminUserResponse(c *gin.Context, user *service.User) any {
	principal := adminaccess.FromContext(c.Request.Context())
	if principal == nil {
		return dto.UserFromServiceAdmin(user)
	}
	if user == nil {
		return nil
	}
	out := gin.H{"id": user.ID}
	permissions := principal.Policy.GlobalPermissions
	if slices.Contains(permissions, "users.read") {
		out["email"] = user.Email
		out["username"] = user.Username
		out["role"] = user.Role
		out["status"] = user.Status
		out["concurrency"] = user.Concurrency
		out["rpm_limit"] = user.RPMLimit
		out["last_active_at"] = user.LastActiveAt
		out["created_at"] = user.CreatedAt
		out["updated_at"] = user.UpdatedAt
		out["deleted_at"] = user.DeletedAt
	}
	if slices.Contains(permissions, "users.balance.read") {
		out["balance"] = user.Balance
		out["frozen_balance"] = user.FrozenBalance
		out["total_recharged"] = user.TotalRecharged
	}
	if slices.Contains(permissions, "groups.read") {
		out["allowed_groups"] = user.AllowedGroups
		out["restrict_public_groups"] = user.RestrictPublicGroups
		out["group_rates"] = user.GroupRates
	}
	return out
}
