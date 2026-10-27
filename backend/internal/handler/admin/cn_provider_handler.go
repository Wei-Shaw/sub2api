package admin

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CNProviderHandler 暴露国产供应商（kimi/zhipu/deepseek）的额度与余额查询端点。
//
//   - GET /admin/cn-providers/accounts/:id/quota   Coding Plan 滚动窗口用量（kimi/zhipu）
//   - GET /admin/cn-providers/accounts/:id/balance  payg 账号余额（kimi/deepseek）
//   - GET/POST /admin/cn-providers/accounts/:id/reset-quota  智谱 Coding Plan 重置卡（查询 / 使用）
//
// 智谱（zhipu）无余额端点，故同一账号仅 quota 或 balance 其一可用：服务端按账号
// platform + account_mode 校验并返回明确错误（见 CNProvider*Service 的 load*Account）。
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

// QueryQuota 查询 Coding Plan 滚动窗口用量（5h + weekly）。
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

// ListResetCards 拉取智谱 Coding Plan 账号的重置卡列表并落快照。
func (h *CNProviderHandler) ListResetCards(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h == nil || h.quotaService == nil {
		response.BadRequest(c, "cn provider quota service is not enabled")
		return
	}
	cards, err := h.quotaService.ListZhipuResetCards(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cards)
}

// UseResetCard 消耗一张智谱重置卡。
// body 可选：{"reset_type":"WEEK"|"FIVE_HOUR","record_id":123}，缺省用最早到期的周卡。
// 用卡、强制重探与账号状态恢复都在服务层的后台流程里完成（客户端断开也会跑完），
// 这里只等待并返回整体结果；用卡失败（上游拒绝、没有可用卡等）返回 400 与错误文案。
func (h *CNProviderHandler) UseResetCard(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h == nil || h.quotaService == nil {
		response.BadRequest(c, "cn provider quota service is not enabled")
		return
	}
	var req service.ZhipuResetCardUseRequest
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}

	result, err := h.quotaService.UseZhipuResetCard(c.Request.Context(), accountID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !result.Success {
		reason := result.ErrorCode
		if reason == "" {
			reason = service.ZhipuResetErrRejected
		}
		message := result.Error
		if message == "" {
			message = "zhipu reset card use failed"
		}
		response.ErrorWithDetails(c, http.StatusBadRequest, message, reason, nil)
		return
	}
	response.Success(c, result)
}
