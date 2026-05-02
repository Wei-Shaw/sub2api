package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ServiceQuotaHandler 服务限额规则的 admin HTTP 入口。
//
// 错误处理统一走 response.ErrorFrom：service 层用 ApplicationError 表达
// 业务语义（404 NotFound / 409 Conflict / 400 BadRequest / 429 TooManyRequests），
// handler 不再吞错误码兜底为 400。
type ServiceQuotaHandler struct{ svc service.ServiceQuotaService }

func NewServiceQuotaHandler(svc service.ServiceQuotaService) *ServiceQuotaHandler {
	return &ServiceQuotaHandler{svc: svc}
}

func (h *ServiceQuotaHandler) List(c *gin.Context) {
	if h.svc == nil {
		response.Error(c, http.StatusNotFound, "service quota unavailable")
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
	if h.svc == nil {
		response.Error(c, http.StatusNotFound, "service quota unavailable")
		return
	}
	var req service.ServiceQuotaRuleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
	if h.svc == nil {
		response.Error(c, http.StatusNotFound, "service quota unavailable")
		return
	}
	id, ok := parseServiceQuotaID(c)
	if !ok {
		return
	}
	var req service.ServiceQuotaRuleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
	if h.svc == nil {
		response.Error(c, http.StatusNotFound, "service quota unavailable")
		return
	}
	id, ok := parseServiceQuotaID(c)
	if !ok {
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
	if h.svc == nil {
		response.Error(c, http.StatusNotFound, "service quota unavailable")
		return
	}
	var req ResetCounterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.ResetLimiterCounter(c.Request.Context(), req.RuleID, req.PathID, req.LimiterType, req.ScopeUserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"reset": true})
}

func parseServiceQuotaID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}
