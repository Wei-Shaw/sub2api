package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// DynamicProxyPoolHandler handles admin dynamic proxy pool management.
type DynamicProxyPoolHandler struct {
	svc *service.DynamicProxyPoolService
}

// NewDynamicProxyPoolHandler constructs the handler.
func NewDynamicProxyPoolHandler(svc *service.DynamicProxyPoolService) *DynamicProxyPoolHandler {
	return &DynamicProxyPoolHandler{svc: svc}
}

// List returns paginated dynamic proxy pools.
func (h *DynamicProxyPoolHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")

	var enabled *bool
	if v := c.Query("enabled"); v != "" {
		b := v == "true" || v == "1"
		enabled = &b
	}

	pools, total, err := h.svc.List(c.Request.Context(), service.DynamicProxyPoolListParams{
		Page:     page,
		PageSize: pageSize,
		Search:   search,
		Enabled:  enabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"items":     pools,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Create creates a new dynamic proxy pool.
func (h *DynamicProxyPoolHandler) Create(c *gin.Context) {
	var req struct {
		Name               string `json:"name" binding:"required"`
		SourceType         string `json:"source_type"`
		SubscriptionID     *int64 `json:"subscription_id"`
		ExtractURL         string `json:"extract_url"`
		Protocol           string `json:"protocol"`
		AuthMode           string `json:"auth_mode"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		ResponseFormat     string `json:"response_format"`
		LineSeparator      string `json:"line_separator"`
		IPFieldPath        string `json:"ip_field_path"`
		PortFieldPath      string `json:"port_field_path"`
		RefreshIntervalSec int    `json:"refresh_interval_sec"`
		IPDurationSec      int    `json:"ip_duration_sec"`
		ExtractCount       int    `json:"extract_count"`
		MinAlive           int    `json:"min_alive"`
		HealthCheckIntervalSec int `json:"health_check_interval_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pool, err := h.svc.Create(c.Request.Context(), service.DynamicProxyPoolCreateParams{
		Name:               req.Name,
		SourceType:         req.SourceType,
		SubscriptionID:     req.SubscriptionID,
		ExtractURL:         req.ExtractURL,
		Protocol:           req.Protocol,
		AuthMode:           req.AuthMode,
		Username:           req.Username,
		Password:           req.Password,
		ResponseFormat:     req.ResponseFormat,
		LineSeparator:      req.LineSeparator,
		IPFieldPath:        req.IPFieldPath,
		PortFieldPath:      req.PortFieldPath,
		RefreshIntervalSec: req.RefreshIntervalSec,
		IPDurationSec:      req.IPDurationSec,
		ExtractCount:       req.ExtractCount,
		MinAlive:           req.MinAlive,
		HealthCheckIntervalSec: req.HealthCheckIntervalSec,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pool)
}

// Get returns a single dynamic proxy pool.
func (h *DynamicProxyPoolHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	pool, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pool)
}

// Update modifies an existing dynamic proxy pool.
func (h *DynamicProxyPoolHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	var req struct {
		Name               *string `json:"name"`
		Enabled            *bool   `json:"enabled"`
		SourceType         *string `json:"source_type"`
		SubscriptionID     *int64  `json:"subscription_id"`
		ExtractURL         *string `json:"extract_url"`
		Protocol           *string `json:"protocol"`
		AuthMode           *string `json:"auth_mode"`
		Username           *string `json:"username"`
		Password           *string `json:"password"`
		ResponseFormat     *string `json:"response_format"`
		LineSeparator      *string `json:"line_separator"`
		IPFieldPath        *string `json:"ip_field_path"`
		PortFieldPath      *string `json:"port_field_path"`
		RefreshIntervalSec *int    `json:"refresh_interval_sec"`
		IPDurationSec      *int    `json:"ip_duration_sec"`
		ExtractCount       *int    `json:"extract_count"`
		MinAlive           *int    `json:"min_alive"`
		HealthCheckIntervalSec *int `json:"health_check_interval_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	pool, err := h.svc.Update(c.Request.Context(), id, service.DynamicProxyPoolUpdateParams{
		Name:               req.Name,
		Enabled:            req.Enabled,
		SourceType:         req.SourceType,
		SubscriptionID:     req.SubscriptionID,
		ExtractURL:         req.ExtractURL,
		Protocol:           req.Protocol,
		AuthMode:           req.AuthMode,
		Username:           req.Username,
		Password:           req.Password,
		ResponseFormat:     req.ResponseFormat,
		LineSeparator:      req.LineSeparator,
		IPFieldPath:        req.IPFieldPath,
		PortFieldPath:      req.PortFieldPath,
		RefreshIntervalSec: req.RefreshIntervalSec,
		IPDurationSec:      req.IPDurationSec,
		ExtractCount:       req.ExtractCount,
		MinAlive:           req.MinAlive,
		HealthCheckIntervalSec: req.HealthCheckIntervalSec,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pool)
}

// Delete removes a dynamic proxy pool.
func (h *DynamicProxyPoolHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// Extract triggers an immediate IP extraction.
func (h *DynamicProxyPoolHandler) Extract(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	result, err := h.svc.Extract(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// StartEntryProxy starts the entry proxy server for a pool.
func (h *DynamicProxyPoolHandler) StartEntryProxy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	var req struct {
		BindAddr string `json:"bind_addr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.BindAddr == "" {
		req.BindAddr = "127.0.0.1:9900"
	}
	if err := h.svc.EnsureEntryProxy(c.Request.Context(), id, req.BindAddr); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"bind_addr": req.BindAddr, "status": "running"})
}

// StopEntryProxy stops the entry proxy server for a pool.
func (h *DynamicProxyPoolHandler) StopEntryProxy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	h.svc.StopEntryProxy(id)
	response.Success(c, gin.H{"status": "stopped"})
}

// ListPoolProxies returns proxies owned by the pool (matching name_prefix).
func (h *DynamicProxyPoolHandler) ListPoolProxies(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	pool, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	proxies, err := h.svc.ListPoolProxies(c.Request.Context(), pool.NamePrefix)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": proxies, "total": len(proxies)})
}

// AssociateProxies adds existing proxies to the pool.
func (h *DynamicProxyPoolHandler) AssociateProxies(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	var req struct {
		ProxyIDs []int64 `json:"proxy_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.svc.AssociateProxies(c.Request.Context(), id, req.ProxyIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// DisassociateProxies removes pool-owned proxies.
func (h *DynamicProxyPoolHandler) DisassociateProxies(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	var req struct {
		ProxyIDs []int64 `json:"proxy_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result := h.svc.DisassociateProxies(c.Request.Context(), id, req.ProxyIDs)
	response.Success(c, result)
}

// PreviewSubscriptionNodes previews nodes from the linked subscription.
func (h *DynamicProxyPoolHandler) PreviewSubscriptionNodes(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	pool, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if pool.SourceType != "subscription" || pool.SubscriptionID == nil {
		response.BadRequest(c, "Pool is not subscription type")
		return
	}
	sub, err := h.svc.LookupSubscription(c.Request.Context(), *pool.SubscriptionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	nodes, err := h.svc.PreviewSubscriptionNodes(c.Request.Context(), sub)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"nodes": nodes, "total": len(nodes)})
}

// AddSubscriptionNodes adds selected subscription nodes to proxy list.
func (h *DynamicProxyPoolHandler) AddSubscriptionNodes(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid pool ID")
		return
	}
	var req struct {
		Identities []string `json:"identities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	createdIDs, err := h.svc.AddSubscriptionNodesToProxies(c.Request.Context(), id, req.Identities)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"created": len(createdIDs), "ids": createdIDs})
}

// TestPoolProxy tests a pool-owned proxy.
func (h *DynamicProxyPoolHandler) TestPoolProxy(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("proxyId"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	result, err := h.svc.TestPoolProxy(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ListPoolEntryProxiesForGroup returns pool entry proxies for group binding.
func (h *DynamicProxyPoolHandler) ListPoolEntryProxiesForGroup(c *gin.Context) {
	opts, err := h.svc.ListPoolEntryProxiesForGroup(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, opts)
}
