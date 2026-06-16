package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ScheduledTestHandler handles admin scheduled-test-plan management.
type ScheduledTestHandler struct {
	scheduledTestSvc *service.ScheduledTestService
	adminService     service.AdminService
}

// NewScheduledTestHandler creates a new ScheduledTestHandler.
func NewScheduledTestHandler(scheduledTestSvc *service.ScheduledTestService, adminService service.AdminService) *ScheduledTestHandler {
	return &ScheduledTestHandler{scheduledTestSvc: scheduledTestSvc, adminService: adminService}
}

type createScheduledTestPlanRequest struct {
	AccountID      int64  `json:"account_id" binding:"required"`
	ModelID        string `json:"model_id"`
	CronExpression string `json:"cron_expression" binding:"required"`
	Enabled        *bool  `json:"enabled"`
	MaxResults     int    `json:"max_results"`
	AutoRecover    *bool  `json:"auto_recover"`
}

type updateScheduledTestPlanRequest struct {
	ModelID        string `json:"model_id"`
	CronExpression string `json:"cron_expression"`
	Enabled        *bool  `json:"enabled"`
	MaxResults     int    `json:"max_results"`
	AutoRecover    *bool  `json:"auto_recover"`
}

// ListByAccount GET /admin/accounts/:id/scheduled-test-plans
func (h *ScheduledTestHandler) ListByAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}

	plans, err := h.scheduledTestSvc.ListPlansByAccount(c.Request.Context(), accountID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, plans)
}

// Create POST /admin/scheduled-test-plans
func (h *ScheduledTestHandler) Create(c *gin.Context) {
	var req createScheduledTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	plan := &service.ScheduledTestPlan{
		AccountID:      req.AccountID,
		ModelID:        req.ModelID,
		CronExpression: req.CronExpression,
		Enabled:        true,
		MaxResults:     req.MaxResults,
	}
	if req.Enabled != nil {
		plan.Enabled = *req.Enabled
	}
	if req.AutoRecover != nil {
		plan.AutoRecover = *req.AutoRecover
	}

	created, err := h.scheduledTestSvc.CreatePlan(c.Request.Context(), plan)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, created)
}

// Update PUT /admin/scheduled-test-plans/:id
func (h *ScheduledTestHandler) Update(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}

	existing, err := h.scheduledTestSvc.GetPlan(c.Request.Context(), planID)
	if err != nil {
		response.NotFound(c, "plan not found")
		return
	}

	var req updateScheduledTestPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.ModelID != "" {
		existing.ModelID = req.ModelID
	}
	if req.CronExpression != "" {
		existing.CronExpression = req.CronExpression
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.MaxResults > 0 {
		existing.MaxResults = req.MaxResults
	}
	if req.AutoRecover != nil {
		existing.AutoRecover = *req.AutoRecover
	}

	updated, err := h.scheduledTestSvc.UpdatePlan(c.Request.Context(), existing)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, updated)
}

// Delete DELETE /admin/scheduled-test-plans/:id
func (h *ScheduledTestHandler) Delete(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}

	if err := h.scheduledTestSvc.DeletePlan(c.Request.Context(), planID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ListResults GET /admin/scheduled-test-plans/:id/results
func (h *ScheduledTestHandler) ListResults(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid plan id")
		return
	}

	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	results, err := h.scheduledTestSvc.ListResults(c.Request.Context(), planID, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, results)
}

// ListResultsByAccount GET /admin/accounts/:id/health-results
// 返回该账号最近 N 条检测结果(含手动 + 定时),供健康详情/历史展示(需求 §7.5)。
func (h *ScheduledTestHandler) ListResultsByAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}

	limit := 10
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	results, err := h.scheduledTestSvc.ListResultsByAccount(c.Request.Context(), accountID, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, results)
}

type batchCreatePlansRequest struct {
	AccountIDs     []int64 `json:"account_ids" binding:"required"`
	ModelID        string  `json:"model_id" binding:"required"`
	CronExpression string  `json:"cron_expression" binding:"required"`
	Enabled        bool    `json:"enabled"`
	MaxResults     int     `json:"max_results"`
	AutoRecover    bool    `json:"auto_recover"`
	// 冲突策略:overwrite(默认)/skip/add。计划 §4.4 命名为 conflict_strategy;
	// 兼容旧字段名 conflict。值 new 视同 add。
	ConflictStrategy string `json:"conflict_strategy"`
	Conflict         string `json:"conflict"`
}

// resolveConflictStrategy 解析批量冲突策略,兼容 conflict_strategy / conflict 两个字段,
// 并将计划文档中的 new 归一化为 add。
func (r batchCreatePlansRequest) resolveConflictStrategy() service.BatchPlanConflictStrategy {
	raw := strings.TrimSpace(r.ConflictStrategy)
	if raw == "" {
		raw = strings.TrimSpace(r.Conflict)
	}
	if raw == "new" {
		raw = string(service.BatchConflictAdd)
	}
	return service.BatchPlanConflictStrategy(raw)
}

// batchCreatePlansResponse 对外响应(计划 §4.4 / 需求 §13.1):{total, success, failed, results}。
type batchCreatePlansResponse struct {
	Total   int                           `json:"total"`
	Success int                           `json:"success"`
	Failed  int                           `json:"failed"`
	Skipped int                           `json:"skipped"`
	Results []service.BatchPlanItemResult `json:"results"`
}

// BatchCreatePlans POST /admin/scheduled-test-plans/batch
// 为多个账号统一创建定时健康检测计划(需求 §7.4)。已删除/不存在账号会被过滤(§7.4.5)。
func (h *ScheduledTestHandler) BatchCreatePlans(c *gin.Context) {
	var req batchCreatePlansRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(req.AccountIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return
	}

	// 过滤不存在的账号(§7.4.5),仅对实际存在的账号建计划。
	existing, err := h.adminService.GetAccountsByIDs(c.Request.Context(), req.AccountIDs)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	validIDs := make([]int64, 0, len(existing))
	for _, acc := range existing {
		validIDs = append(validIDs, acc.ID)
	}
	if len(validIDs) == 0 {
		response.BadRequest(c, "no valid accounts found")
		return
	}

	result, err := h.scheduledTestSvc.BatchCreatePlans(c.Request.Context(), service.BatchCreatePlansInput{
		AccountIDs:     validIDs,
		ModelID:        req.ModelID,
		CronExpression: req.CronExpression,
		Enabled:        req.Enabled,
		MaxResults:     req.MaxResults,
		AutoRecover:    req.AutoRecover,
		Conflict:       req.resolveConflictStrategy(),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, batchCreatePlansResponse{
		Total:   len(req.AccountIDs),
		Success: result.Success,
		Failed:  result.Failed,
		Skipped: result.Skipped,
		Results: result.Items,
	})
}
