package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

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
		ScopeLevel:  c.Query("scope_level"),
		LimiterType: c.Query("limiter_type"),
	}
	if raw := c.Query("enabled"); raw != "" {
		v := raw == "true" || raw == "1"
		filter.Enabled = &v
	}
	rules, err := h.svc.ListRules(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"items": rules, "total": len(rules)})
}

func (h *ServiceQuotaHandler) Create(c *gin.Context) {
	var req service.ServiceQuotaRuleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	rule, err := h.svc.CreateRule(c.Request.Context(), req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, rule)
}

func (h *ServiceQuotaHandler) Update(c *gin.Context) {
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
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, rule)
}

func (h *ServiceQuotaHandler) Delete(c *gin.Context) {
	id, ok := parseServiceQuotaID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func parseServiceQuotaID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}
