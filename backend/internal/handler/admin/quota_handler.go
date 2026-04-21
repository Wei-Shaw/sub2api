package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// QuotaHandler 用户每日配额（feature issue #1750）管理端处理器
type QuotaHandler struct {
	quotaService service.QuotaService
}

// NewQuotaHandler 构造处理器
func NewQuotaHandler(quotaService service.QuotaService) *QuotaHandler {
	return &QuotaHandler{quotaService: quotaService}
}

// Get GET /api/v1/admin/users/:id/quota
// 返回用户配额总视图（override + resolved + today_usage）
func (h *QuotaHandler) Get(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	view, err := h.quotaService.GetUserQuota(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// Update PUT /api/v1/admin/users/:id/quota
// 支持双指针语义：undefined=不改；null=清空回默认；值=写入
func (h *QuotaHandler) Update(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	var req service.UpdateUserQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if err := h.quotaService.UpdateUserQuota(c.Request.Context(), userID, req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// ListRules GET /api/v1/admin/users/:id/quota/rules
func (h *QuotaHandler) ListRules(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	rules, err := h.quotaService.ListRules(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

// CreateRule POST /api/v1/admin/users/:id/quota/rules
func (h *QuotaHandler) CreateRule(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	var req service.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	rule, err := h.quotaService.CreateRule(c.Request.Context(), userID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

// UpdateRule PUT /api/v1/admin/users/:id/quota/rules/:ruleID
func (h *QuotaHandler) UpdateRule(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	ruleID, err := strconv.ParseInt(c.Param("ruleID"), 10, 64)
	if err != nil || ruleID <= 0 {
		response.BadRequest(c, "invalid rule id")
		return
	}
	var req service.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	rule, err := h.quotaService.UpdateRule(c.Request.Context(), userID, ruleID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

// DeleteRule DELETE /api/v1/admin/users/:id/quota/rules/:ruleID
func (h *QuotaHandler) DeleteRule(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	ruleID, err := strconv.ParseInt(c.Param("ruleID"), 10, 64)
	if err != nil || ruleID <= 0 {
		response.BadRequest(c, "invalid rule id")
		return
	}
	if err := h.quotaService.DeleteRule(c.Request.Context(), userID, ruleID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// ReplaceRules PUT /api/v1/admin/users/:id/quota/rules
// 全量替换：请求体 { "rules": [...] }，后端在单事务内 DELETE 旧规则 + 批量 INSERT 新规则。
// 任何规则校验失败或事务失败整体回滚；成功后失效配额配置缓存。
func (h *QuotaHandler) ReplaceRules(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "invalid user id")
		return
	}
	var req struct {
		Rules []service.ReplaceRuleInput `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.Rules == nil {
		req.Rules = []service.ReplaceRuleInput{}
	}
	rules, err := h.quotaService.ReplaceUserRules(c.Request.Context(), userID, req.Rules)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}
