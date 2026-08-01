package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RefreshUpstreamQuotaSync 手动触发单账号的上游配额同步。
// POST /api/v1/admin/accounts/:id/upstream-quota-sync/refresh
// 忽略二级开关与节流，立即调用上游 /v1/usage 并刷新 extra 快照。
func (h *AccountHandler) RefreshUpstreamQuotaSync(c *gin.Context) {
	if h.upstreamQuotaSync == nil {
		response.ErrorFrom(c, service.ErrUpstreamQuotaSyncUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	snapshot, err := h.upstreamQuotaSync.SyncAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 同步成功后重新加载账号，把最新 extra（含 quota_daily/weekly/monthly）回传给前端。
	account, loadErr := h.adminService.GetAccount(c.Request.Context(), accountID)
	if loadErr != nil {
		// 同步已成功但加载失败：仍返回 snapshot，前端可凭 snapshot 判断。
		response.Success(c, gin.H{
			"account":  nil,
			"snapshot": snapshot,
		})
		return
	}
	response.Success(c, gin.H{
		"account":  h.buildAccountResponseWithRuntime(c.Request.Context(), account),
		"snapshot": snapshot,
	})
}
