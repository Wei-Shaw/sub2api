package admin

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SyncPrivateGroupExpiresRequest 批量同步私有订阅到期日请求体。
// confirm=true 为二次确认门闩（前端确认弹窗后提交）。
type SyncPrivateGroupExpiresRequest struct {
	Confirm bool `json:"confirm"`
}

// SyncPrivateGroupExpires 将全部私有专属订阅 expires_at 同步为设置中的绝对到期日。
// POST /api/v1/admin/settings/private-group-expires/sync
//
// 产品 B：与 settings 保存分离；未配置日期 → 400。
// S1：含 expired/suspended；target 未来→active，过去→expired。
// 强制同步审计（actor_admin_id / updated / target date）。
func (h *SettingHandler) SyncPrivateGroupExpires(c *gin.Context) {
	var req SyncPrivateGroupExpiresRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "confirm must be true to sync private subscription expiry")
		return
	}
	if h.privateGroups == nil {
		response.ErrorFrom(c, errors.New("private group provisioner not configured"))
		return
	}

	result, err := h.privateGroups.SyncPrivateSubscriptionExpiresAt(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	actorID := getAdminIDFromContext(c)
	expiresAtStr := result.ExpiresAt.Format(time.RFC3339)
	middleware.SetAuditAction(c, service.AuditActionPrivateGroupExpiresSync)
	middleware.SetAuditExtra(c, map[string]any{
		"updated":        result.Updated,
		"expires_at":     expiresAtStr,
		"status":         result.Status,
		"actor_admin_id": actorID,
		"confirm":        true,
		"result":         "ok",
	})

	// 强制同步落库；成功则跳过中间件异步写，避免双记。
	if h.auditLog != nil {
		entry := &service.AuditLog{
			Action:     service.AuditActionPrivateGroupExpiresSync,
			Method:     c.Request.Method,
			Path:       c.FullPath(),
			StatusCode: 200,
			ActorRole:  service.RoleAdmin,
			Extra: map[string]any{
				"updated":        result.Updated,
				"expires_at":     expiresAtStr,
				"status":         result.Status,
				"actor_admin_id": actorID,
				"confirm":        true,
			},
		}
		if actorID > 0 {
			entry.ActorUserID = &actorID
		}
		if ferr := h.auditLog.ForceRecord(c.Request.Context(), entry); ferr != nil {
			// DB 已更新；审计失败必须显式暴露（与 ClearAll 留痕失败语义一致）。
			slog.Error("private group expires sync audit force write failed",
				"updated", result.Updated, "expires_at", expiresAtStr, "err", ferr)
			response.ErrorFrom(c, ferr)
			return
		}
		middleware.SkipAudit(c)
	}

	response.Success(c, gin.H{
		"updated":    result.Updated,
		"expires_at": expiresAtStr,
		"status":     result.Status,
	})
}
