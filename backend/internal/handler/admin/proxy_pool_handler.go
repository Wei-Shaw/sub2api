package admin

import (
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProxyPoolHandler 代理池管理。
type ProxyPoolHandler struct {
	adminService service.AdminService
}

// NewProxyPoolHandler 创建代理池管理 handler。
func NewProxyPoolHandler(adminService service.AdminService) *ProxyPoolHandler {
	return &ProxyPoolHandler{adminService: adminService}
}

// CreateProxyPoolRequest 创建代理池请求。
type CreateProxyPoolRequest struct {
	Name                  string  `json:"name" binding:"required,max=100"`
	Description           *string `json:"description"`
	Status                string  `json:"status" binding:"omitempty,oneof=active disabled"`
	HealthIntervalSeconds int     `json:"health_interval_seconds" binding:"omitempty,min=30,max=86400"`
	FailureThreshold      int     `json:"failure_threshold" binding:"omitempty,min=1,max=100"`
	AutoRebind            *bool   `json:"auto_rebind"`
}

// UpdateProxyPoolRequest 更新代理池请求（nil 字段不修改）。
type UpdateProxyPoolRequest struct {
	Name                  *string `json:"name" binding:"omitempty,max=100"`
	Description           *string `json:"description"`
	Status                *string `json:"status" binding:"omitempty,oneof=active disabled"`
	HealthIntervalSeconds *int    `json:"health_interval_seconds" binding:"omitempty,min=30,max=86400"`
	FailureThreshold      *int    `json:"failure_threshold" binding:"omitempty,min=1,max=100"`
	AutoRebind            *bool   `json:"auto_rebind"`
}

// AssignProxiesRequest 池成员变更请求。
type AssignProxiesRequest struct {
	ProxyIDs []int64 `json:"proxy_ids" binding:"required"`
}

func parsePoolID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid pool id")
		return 0, false
	}
	return id, true
}

// List 代理池列表（含统计）。
// GET /api/v1/admin/proxy-pools
func (h *ProxyPoolHandler) List(c *gin.Context) {
	pools, err := h.adminService.ListProxyPools(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.AdminProxyPoolWithStats, 0, len(pools))
	for i := range pools {
		out = append(out, *dto.ProxyPoolWithStatsFromService(&pools[i]))
	}
	response.Success(c, out)
}

// GetByID 代理池详情。
// GET /api/v1/admin/proxy-pools/:id
func (h *ProxyPoolHandler) GetByID(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	pool, err := h.adminService.GetProxyPool(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrProxyPoolNotFound) {
			response.NotFound(c, "proxy pool not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyPoolFromService(pool))
}

// GetProxies 池内代理列表（含账号数与延迟）。
// GET /api/v1/admin/proxy-pools/:id/proxies
func (h *ProxyPoolHandler) GetProxies(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	proxies, err := h.adminService.GetProxyPoolProxies(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.AdminProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		out = append(out, *dto.ProxyWithAccountCountFromServiceAdmin(&proxies[i]))
	}
	response.Success(c, out)
}

// Create 创建代理池。
// POST /api/v1/admin/proxy-pools
func (h *ProxyPoolHandler) Create(c *gin.Context) {
	var req CreateProxyPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	input := &service.CreateProxyPoolInput{
		Name:                  req.Name,
		Description:           req.Description,
		Status:                req.Status,
		HealthIntervalSeconds: req.HealthIntervalSeconds,
		FailureThreshold:      req.FailureThreshold,
	}
	if req.AutoRebind != nil {
		input.AutoRebind = *req.AutoRebind
	}
	pool, err := h.adminService.CreateProxyPool(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.ProxyPoolFromService(pool))
}

// Update 更新代理池。
// PUT /api/v1/admin/proxy-pools/:id
func (h *ProxyPoolHandler) Update(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	var req UpdateProxyPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// description 需要区分「未传」与「清空」：用本地指针传递
	input := &service.UpdateProxyPoolInput{
		Name:                  req.Name,
		Description:           req.Description,
		Status:                req.Status,
		HealthIntervalSeconds: req.HealthIntervalSeconds,
		FailureThreshold:      req.FailureThreshold,
		AutoRebind:            req.AutoRebind,
	}
	pool, err := h.adminService.UpdateProxyPool(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProxyPoolFromService(pool))
}

// Delete 删除代理池。
// DELETE /api/v1/admin/proxy-pools/:id
func (h *ProxyPoolHandler) Delete(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	if err := h.adminService.DeleteProxyPool(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// AssignProxies 把代理加入池。
// POST /api/v1/admin/proxy-pools/:id/proxies
func (h *ProxyPoolHandler) AssignProxies(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	var req AssignProxiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	affected, err := h.adminService.AssignProxiesToPool(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"assigned": affected})
}

// RemoveProxies 把代理移出池。
// DELETE /api/v1/admin/proxy-pools/:id/proxies
func (h *ProxyPoolHandler) RemoveProxies(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	var req AssignProxiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	affected, err := h.adminService.RemoveProxiesFromPool(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"removed": affected})
}

// Rebind 手动触发一轮健康探测 + 自动重绑。
// POST /api/v1/admin/proxy-pools/:id/rebind
func (h *ProxyPoolHandler) Rebind(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	rebound, err := h.adminService.RunProxyPoolRebind(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"rebound_accounts": rebound})
}

// RebindLogs 池内最近的重绑日志。
// GET /api/v1/admin/proxy-pools/:id/rebind-logs?limit=50
func (h *ProxyPoolHandler) RebindLogs(c *gin.Context) {
	id, ok := parsePoolID(c)
	if !ok {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	logs, err := h.adminService.ListProxyPoolRebindLogs(c.Request.Context(), id, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.AdminProxyPoolRebindLog, 0, len(logs))
	for i := range logs {
		out = append(out, *dto.ProxyPoolRebindLogFromService(&logs[i]))
	}
	response.Success(c, out)
}
