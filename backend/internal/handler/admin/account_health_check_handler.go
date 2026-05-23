package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountHealthCheckHandler struct {
	healthSvc *service.AccountHealthCheckService
}

func NewAccountHealthCheckHandler(healthSvc *service.AccountHealthCheckService) *AccountHealthCheckHandler {
	return &AccountHealthCheckHandler{healthSvc: healthSvc}
}

type createAccountBatchTestRequest struct {
	AccountIDs  []int64 `json:"account_ids" binding:"required,min=1"`
	ModelID     string  `json:"model_id"`
	Concurrency int     `json:"concurrency"`
	AutoDisable bool    `json:"auto_disable"`
}

type updateAccountHealthSettingsRequest struct {
	HealthCheckEnabled  *bool `json:"health_check_enabled"`
	HealthCheckProtected *bool `json:"health_check_protected"`
}

type bulkUpdateAccountHealthSettingsRequest struct {
	AccountIDs          []int64 `json:"account_ids" binding:"required,min=1"`
	HealthCheckEnabled  *bool   `json:"health_check_enabled"`
	HealthCheckProtected *bool  `json:"health_check_protected"`
}

func (h *AccountHealthCheckHandler) CreateBatchTest(c *gin.Context) {
	var req createAccountBatchTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	task, err := h.healthSvc.CreateManualBatchTest(c.Request.Context(), req.AccountIDs, req.ModelID, req.Concurrency, req.AutoDisable)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, task)
}

func (h *AccountHealthCheckHandler) ListBatchTests(c *gin.Context) {
	limit, offset := parseLimitOffset(c)
	tasks, total, err := h.healthSvc.ListTasks(c.Request.Context(), limit, offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"items":  tasks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *AccountHealthCheckHandler) GetBatchTest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid task id")
		return
	}
	limit, offset := parseLimitOffset(c)
	task, results, total, err := h.healthSvc.GetTask(c.Request.Context(), id, limit, offset)
	if err != nil {
		response.NotFound(c, "batch test not found")
		return
	}
	response.Success(c, gin.H{
		"task":          task,
		"results":       results,
		"results_total": total,
		"limit":         limit,
		"offset":        offset,
	})
}

func (h *AccountHealthCheckHandler) UpdateSettings(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid account id")
		return
	}
	var req updateAccountHealthSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	account, err := h.healthSvc.UpdateSettings(c.Request.Context(), accountID, service.AccountHealthCheckSettings{
		Enabled:   req.HealthCheckEnabled,
		Protected: req.HealthCheckProtected,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *AccountHealthCheckHandler) BulkUpdateSettings(c *gin.Context) {
	var req bulkUpdateAccountHealthSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	updated, err := h.healthSvc.BulkUpdateSettings(c.Request.Context(), req.AccountIDs, service.AccountHealthCheckSettings{
		Enabled:   req.HealthCheckEnabled,
		Protected: req.HealthCheckProtected,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"updated": updated})
}

func parseLimitOffset(c *gin.Context) (int, int) {
	limit := 50
	offset := 0
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}
	return limit, offset
}
