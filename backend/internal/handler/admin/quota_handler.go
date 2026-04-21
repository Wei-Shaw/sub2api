package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ---- 错误码常量（前端 constants/quota.ts 同步） ----
//
// 历史上 quota_handler 用 "invalid request: <err.Error()>" 这类拼好的自然语言回给前端，
// 违反 CLAUDE.md §4：错误响应必须结构化（code + message + metadata），由前端做 i18n。
// 下列常量覆盖处理器层的三类 400：user id 解析失败、rule id 解析失败、JSON 反序列化失败。
//
// 具体解析/绑定逻辑已上抽为 admin 包公共 helper（common.go 的 ParseInt64Param /
// BindJSONOrError），本文件只保留业务错误码 + 路由组装，避免与其他 handler 重复
// "ParseInt + BadRequest + metadata" 这三行模板。
const (
	errReasonQuotaInvalidUserID  = "QUOTA_INVALID_USER_ID"
	errReasonQuotaInvalidRuleID  = "QUOTA_INVALID_RULE_ID"
	errReasonQuotaInvalidRequest = "QUOTA_INVALID_REQUEST"
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req service.UpdateUserQuotaRequest
	if err := BindJSONOrError(c, &req, errReasonQuotaInvalidRequest); err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req service.CreateRuleRequest
	if err := BindJSONOrError(c, &req, errReasonQuotaInvalidRequest); err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	ruleID, err := ParseInt64Param(c, "ruleID", errReasonQuotaInvalidRuleID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req service.UpdateRuleRequest
	if err := BindJSONOrError(c, &req, errReasonQuotaInvalidRequest); err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	ruleID, err := ParseInt64Param(c, "ruleID", errReasonQuotaInvalidRuleID)
	if err != nil {
		response.ErrorFrom(c, err)
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
	userID, err := ParseInt64Param(c, "id", errReasonQuotaInvalidUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var req struct {
		Rules []service.CreateRuleRequest `json:"rules"`
	}
	if err := BindJSONOrError(c, &req, errReasonQuotaInvalidRequest); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if req.Rules == nil {
		req.Rules = []service.CreateRuleRequest{}
	}
	rules, err := h.quotaService.ReplaceUserRules(c.Request.Context(), userID, req.Rules)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}
