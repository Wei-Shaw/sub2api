package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProxyPoolHandler struct {
	service *service.ProxyPoolService
}

func NewProxyPoolHandler(poolService *service.ProxyPoolService) *ProxyPoolHandler {
	return &ProxyPoolHandler{service: poolService}
}

type createProxyPoolRequest struct {
	Name                  string  `json:"name" binding:"required,max=100"`
	Description           *string `json:"description"`
	Status                string  `json:"status" binding:"omitempty,oneof=active disabled"`
	HealthIntervalSeconds int     `json:"health_interval_seconds" binding:"omitempty,min=30,max=86400"`
	FailureThreshold      int     `json:"failure_threshold" binding:"omitempty,min=1,max=10"`
	AutoRebind            *bool   `json:"auto_rebind"`
}

type updateProxyPoolRequest struct {
	Name                  *string `json:"name" binding:"omitempty,max=100"`
	Description           *string `json:"description"`
	Status                *string `json:"status" binding:"omitempty,oneof=active disabled"`
	HealthIntervalSeconds *int    `json:"health_interval_seconds" binding:"omitempty,min=30,max=86400"`
	FailureThreshold      *int    `json:"failure_threshold" binding:"omitempty,min=1,max=10"`
	AutoRebind            *bool   `json:"auto_rebind"`
}

type proxyPoolIDsRequest struct {
	ProxyIDs []int64 `json:"proxy_ids" binding:"required,min=1,max=5000,dive,gt=0"`
}

type proxyPoolAccountIDsRequest struct {
	AccountIDs []int64 `json:"account_ids" binding:"required,min=1,max=10000,dive,gt=0"`
}

func parseProxyPoolID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid proxy pool ID")
		return 0, false
	}
	return id, true
}

func (h *ProxyPoolHandler) List(c *gin.Context) {
	pools, err := h.service.ListPools(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pools)
}

func (h *ProxyPoolHandler) GetByID(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	pool, err := h.service.GetPool(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pool)
}

func (h *ProxyPoolHandler) Create(c *gin.Context) {
	var req createProxyPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	autoRebind := true
	if req.AutoRebind != nil {
		autoRebind = *req.AutoRebind
	}
	pool, err := h.service.CreatePool(c.Request.Context(), &service.CreateProxyPoolInput{
		Name:                  req.Name,
		Description:           req.Description,
		Status:                req.Status,
		HealthIntervalSeconds: req.HealthIntervalSeconds,
		FailureThreshold:      req.FailureThreshold,
		AutoRebind:            autoRebind,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, pool)
}

func (h *ProxyPoolHandler) Update(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	var req updateProxyPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	pool, err := h.service.UpdatePool(c.Request.Context(), id, &service.UpdateProxyPoolInput{
		Name:                  req.Name,
		Description:           req.Description,
		Status:                req.Status,
		HealthIntervalSeconds: req.HealthIntervalSeconds,
		FailureThreshold:      req.FailureThreshold,
		AutoRebind:            req.AutoRebind,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pool)
}

func (h *ProxyPoolHandler) Delete(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	if err := h.service.DeletePool(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *ProxyPoolHandler) GetProxies(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	proxies, err := h.service.GetPoolProxies(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.ProxyPoolProxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *dto.ProxyPoolProxyFromService(&proxies[i]))
	}
	response.Success(c, out)
}

func (h *ProxyPoolHandler) AssignProxies(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	var req proxyPoolIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	affected, err := h.service.AssignProxies(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"assigned": affected})
}

func (h *ProxyPoolHandler) RemoveProxies(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	var req proxyPoolIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	affected, err := h.service.RemoveProxies(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"removed": affected})
}

func (h *ProxyPoolHandler) BindAccounts(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	var req proxyPoolAccountIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.service.BindAccounts(c.Request.Context(), id, req.AccountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProxyPoolHandler) UnbindAccounts(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	var req proxyPoolAccountIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	affected, err := h.service.UnbindAccounts(c.Request.Context(), id, req.AccountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"unbound": affected})
}

func (h *ProxyPoolHandler) Rebind(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	rebound, err := h.service.RunPoolNow(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"rebound_accounts": rebound})
}

func (h *ProxyPoolHandler) RebindLogs(c *gin.Context) {
	id, ok := parseProxyPoolID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	logs, err := h.service.RebindLogs(c.Request.Context(), id, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, logs)
}
