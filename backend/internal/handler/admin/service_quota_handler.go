package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ServiceQuotaHandler 服务限额规则的 admin HTTP 入口。
//
// 错误处理统一走 response.ErrorFrom：service 层与 handler 层一致用 ApplicationError
// 表达业务语义（404 NotFound / 409 Conflict / 400 BadRequest / 429 TooManyRequests），
// 让前端能按 reason / metadata 做 i18n，而不是吃到 gin binding 英文 message。
type ServiceQuotaHandler struct{ svc service.ServiceQuotaService }

func NewServiceQuotaHandler(svc service.ServiceQuotaService) *ServiceQuotaHandler {
	return &ServiceQuotaHandler{svc: svc}
}

// 错误码常量：与前端 i18n key（admin.serviceQuota.errors.* / common.errors.*）对齐。
// INVALID_REQUEST_BODY / INVALID_ID 是通用错误码，集中在 admin/common.go 定义为
// errReasonInvalidRequestBody / errReasonInvalidID 共享常量；本文件仅保留模块特定的 reason。
//
// SERVICE_QUOTA_UNAVAILABLE 是 service 模块未启用时的标识，前端按此引导用户去开启 setting。
const (
	errReasonServiceQuotaUnavailable = "SERVICE_QUOTA_UNAVAILABLE"
)

// requireService 校验 svc 是否注册（service quota 模块未启用时返回结构化 NotFound）。
// 抽出来让 4 个 handler 入口的 nil-check 集中维护，避免散落的 response.Error 直接写英文文案。
func (h *ServiceQuotaHandler) requireService(c *gin.Context) bool {
	if h.svc == nil {
		response.ErrorFrom(c, errors.NotFound(
			errReasonServiceQuotaUnavailable,
			"service quota module is not enabled",
		))
		return false
	}
	return true
}

func (h *ServiceQuotaHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	filter := service.ServiceQuotaListFilter{
		LimiterType: c.Query("limiter_type"),
	}
	if raw := c.Query("enabled"); raw != "" {
		v := raw == "true" || raw == "1"
		filter.Enabled = &v
	}
	rules, err := h.svc.ListRules(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": rules, "total": len(rules)})
}

func (h *ServiceQuotaHandler) Create(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req service.ServiceQuotaRuleInput
	if err := BindJSONOrError(c, &req, errReasonInvalidRequestBody); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rule, err := h.svc.CreateRule(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

func (h *ServiceQuotaHandler) Update(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, err := ParseInt64Param(c, "id", errReasonInvalidID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if id <= 0 {
		response.ErrorFrom(c, errors.BadRequest(errReasonInvalidID, "invalid id").WithMetadata(map[string]string{
			"param": "id",
			"value": c.Param("id"),
		}))
		return
	}
	var req service.ServiceQuotaRuleInput
	if err := BindJSONOrError(c, &req, errReasonInvalidRequestBody); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rule, err := h.svc.UpdateRule(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rule)
}

func (h *ServiceQuotaHandler) Delete(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, err := ParseInt64Param(c, "id", errReasonInvalidID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if id <= 0 {
		response.ErrorFrom(c, errors.BadRequest(errReasonInvalidID, "invalid id").WithMetadata(map[string]string{
			"param": "id",
			"value": c.Param("id"),
		}))
		return
	}
	if err := h.svc.DeleteRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// ResetCounterRequest 是手动重置限额的请求体。
//
// scope_user_id 为 nil/0 表示 shared 计数（counter_mode=shared，
// 或 per_user 在管理员视图未限定用户的情形）；非 nil 对应特定用户的
// 独立计数（counter_mode=user 命中或 per_user 限定到某用户）。
type ResetCounterRequest struct {
	RuleID      int64  `json:"rule_id"`
	PathID      int64  `json:"path_id"`
	LimiterType string `json:"limiter_type"`
	ScopeUserID *int64 `json:"scope_user_id,omitempty"`
}

func (h *ServiceQuotaHandler) ResetCounter(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req ResetCounterRequest
	if err := BindJSONOrError(c, &req, errReasonInvalidRequestBody); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.svc.ResetLimiterCounter(c.Request.Context(), req.RuleID, req.PathID, req.LimiterType, req.ScopeUserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"reset": true})
}
