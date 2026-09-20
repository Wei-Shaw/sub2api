package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) QualityOverview(c *gin.Context) {
	data, err := h.accountTestService.QualityChecks().Overview(c.Request.Context())
	if err != nil {
		response.InternalError(c, "读取检查结果失败")
		return
	}
	response.Success(c, data)
}
func (h *AccountHandler) QualityConfigure(c *gin.Context) {
	var cfg service.QualityConfig
	if c.ShouldBindJSON(&cfg) != nil {
		response.BadRequest(c, "设置格式无效")
		return
	}
	if err := h.accountTestService.QualityChecks().Configure(c.Request.Context(), cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *AccountHandler) QualityRunNow(c *gin.Context) {
	var input struct {
		AccountID int64 `json:"account_id"`
	}
	if c.ShouldBindJSON(&input) != nil || input.AccountID < 0 {
		response.BadRequest(c, "账号编号无效")
		return
	}
	n, err := h.accountTestService.QualityChecks().Dispatch(c.Request.Context(), true, input.AccountID)
	if err != nil {
		response.InternalError(c, "启动检查失败")
		return
	}
	response.Success(c, gin.H{"started": n})
}
func (h *AccountHandler) QualityHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "账号编号无效")
		return
	}
	data, err := h.accountTestService.QualityChecks().History(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, "读取历史失败")
		return
	}
	response.Success(c, data)
}
func (h *AccountHandler) QualityRunDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "记录编号无效")
		return
	}
	data, err := h.accountTestService.QualityChecks().GetRun(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "检查记录不存在")
		return
	}
	response.Success(c, data)
}
