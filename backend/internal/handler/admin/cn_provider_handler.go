package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CNProviderHandler 暴露国产供应商的额度与余额查询端点。
//
//   - GET /admin/cn-providers/accounts/:id/quota   Kimi/Zhipu Coding Plan 用量
//   - GET /admin/cn-providers/accounts/:id/balance  payg 账号余额（kimi/deepseek）
//
// 智谱与 Qwen 无公开 payg 余额端点。Qwen Token Plan 无公开用量端点：
// QueryQuota 必须返回 CN_QUOTA_NOT_SUPPORTED，不得向阿里云发送任何 usage 请求。
// 服务端按账号 platform + account_mode 校验并返回明确错误（见
// CNProvider*Service 的 load*Account）。
type CNProviderHandler struct {
	quotaService   *service.CNProviderQuotaService
	balanceService *service.CNProviderBalanceService
}

func NewCNProviderHandler(
	quotaService *service.CNProviderQuotaService,
	balanceService *service.CNProviderBalanceService,
) *CNProviderHandler {
	return &CNProviderHandler{
		quotaService:   quotaService,
		balanceService: balanceService,
	}
}

// QueryQuota 查询 Kimi/Zhipu Coding Plan 滚动窗口用量。
// Qwen Token Plan 没有可查询的用量接口；服务层在发起任何上游请求前返回
// CN_QUOTA_NOT_SUPPORTED。
func (h *CNProviderHandler) QueryQuota(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h == nil || h.quotaService == nil {
		response.BadRequest(c, "cn provider quota service is not enabled")
		return
	}
	result, err := h.quotaService.QueryUsage(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// QueryBalance 查询 payg 账号余额。
func (h *CNProviderHandler) QueryBalance(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h == nil || h.balanceService == nil {
		response.BadRequest(c, "cn provider balance service is not enabled")
		return
	}
	result, err := h.balanceService.QueryBalance(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
