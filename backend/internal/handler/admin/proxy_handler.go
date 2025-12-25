package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProxyHandler handles admin proxy management
type ProxyHandler struct {
	adminService service.AdminService
}

// NewProxyHandler creates a new admin proxy handler
func NewProxyHandler(adminService service.AdminService) *ProxyHandler {
	return &ProxyHandler{
		adminService: adminService,
	}
}

// CreateProxyRequest represents create proxy request
type CreateProxyRequest struct {
	Name     string `json:"name" binding:"required"`
	Protocol string `json:"protocol" binding:"required,oneof=http https socks5"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required,min=1,max=65535"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateProxyRequest represents update proxy request
type UpdateProxyRequest struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol" binding:"omitempty,oneof=http https socks5"`
	Host     string `json:"host"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// List handles listing all proxies with pagination
// GET /api/v1/admin/proxies
func (h *ProxyHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	protocol := c.Query("protocol")
	status := c.Query("status")
	search := c.Query("search")

	proxies, total, err := h.adminService.ListProxies(c.Request.Context(), page, pageSize, protocol, status, search)
	if err != nil {
		response.InternalError(c, "Failed to list proxies: "+err.Error())
		return
	}

	response.Paginated(c, proxies, total, page, pageSize)
}

// GetAll handles getting all active proxies without pagination
// GET /api/v1/admin/proxies/all
// Optional query param: with_count=true to include account count per proxy
func (h *ProxyHandler) GetAll(c *gin.Context) {
	withCount := c.Query("with_count") == "true"

	if withCount {
		proxies, err := h.adminService.GetAllProxiesWithAccountCount(c.Request.Context())
		if err != nil {
			response.InternalError(c, "Failed to get proxies: "+err.Error())
			return
		}
		response.Success(c, proxies)
		return
	}

	proxies, err := h.adminService.GetAllProxies(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to get proxies: "+err.Error())
		return
	}

	response.Success(c, proxies)
}

// GetByID handles getting a proxy by ID
// GET /api/v1/admin/proxies/:id
func (h *ProxyHandler) GetByID(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	proxy, err := h.adminService.GetProxy(c.Request.Context(), proxyID)
	if err != nil {
		response.NotFound(c, "Proxy not found")
		return
	}

	response.Success(c, proxy)
}

// Create handles creating a new proxy
// POST /api/v1/admin/proxies
func (h *ProxyHandler) Create(c *gin.Context) {
	var req CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	proxy, err := h.adminService.CreateProxy(c.Request.Context(), &service.CreateProxyInput{
		Name:     strings.TrimSpace(req.Name),
		Protocol: strings.TrimSpace(req.Protocol),
		Host:     strings.TrimSpace(req.Host),
		Port:     req.Port,
		Username: strings.TrimSpace(req.Username),
		Password: strings.TrimSpace(req.Password),
	})
	if err != nil {
		response.BadRequest(c, "Failed to create proxy: "+err.Error())
		return
	}

	response.Success(c, proxy)
}

// Update handles updating a proxy
// PUT /api/v1/admin/proxies/:id
func (h *ProxyHandler) Update(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	var req UpdateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	proxy, err := h.adminService.UpdateProxy(c.Request.Context(), proxyID, &service.UpdateProxyInput{
		Name:     strings.TrimSpace(req.Name),
		Protocol: strings.TrimSpace(req.Protocol),
		Host:     strings.TrimSpace(req.Host),
		Port:     req.Port,
		Username: strings.TrimSpace(req.Username),
		Password: strings.TrimSpace(req.Password),
		Status:   strings.TrimSpace(req.Status),
	})
	if err != nil {
		response.InternalError(c, "Failed to update proxy: "+err.Error())
		return
	}

	response.Success(c, proxy)
}

// Delete handles deleting a proxy
// DELETE /api/v1/admin/proxies/:id
func (h *ProxyHandler) Delete(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	err = h.adminService.DeleteProxy(c.Request.Context(), proxyID)
	if err != nil {
		response.InternalError(c, "Failed to delete proxy: "+err.Error())
		return
	}

	response.Success(c, gin.H{"message": "Proxy deleted successfully"})
}

// Test handles testing proxy connectivity
// POST /api/v1/admin/proxies/:id/test
func (h *ProxyHandler) Test(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	result, err := h.adminService.TestProxy(c.Request.Context(), proxyID)
	if err != nil {
		response.InternalError(c, "Failed to test proxy: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GetStats handles getting proxy statistics
// GET /api/v1/admin/proxies/:id/stats
func (h *ProxyHandler) GetStats(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	// Return mock data for now
	_ = proxyID
	response.Success(c, gin.H{
		"total_accounts":  0,
		"active_accounts": 0,
		"total_requests":  0,
		"success_rate":    100.0,
		"average_latency": 0,
	})
}

// GetProxyAccounts handles getting accounts using a proxy
// GET /api/v1/admin/proxies/:id/accounts
func (h *ProxyHandler) GetProxyAccounts(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	accounts, total, err := h.adminService.GetProxyAccounts(c.Request.Context(), proxyID, page, pageSize)
	if err != nil {
		response.InternalError(c, "Failed to get proxy accounts: "+err.Error())
		return
	}

	response.Paginated(c, accounts, total, page, pageSize)
}

// BatchCreateProxyItem represents a single proxy in batch create request
type BatchCreateProxyItem struct {
	Protocol string `json:"protocol" binding:"required,oneof=http https socks5"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required,min=1,max=65535"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// BatchCreateRequest represents batch create proxies request
type BatchCreateRequest struct {
	Proxies []BatchCreateProxyItem `json:"proxies" binding:"required,min=1"`
}

// BatchCreate handles batch creating proxies
// POST /api/v1/admin/proxies/batch
func (h *ProxyHandler) BatchCreate(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	created := 0
	skipped := 0

	for _, item := range req.Proxies {
		// Trim all string fields
		host := strings.TrimSpace(item.Host)
		protocol := strings.TrimSpace(item.Protocol)
		username := strings.TrimSpace(item.Username)
		password := strings.TrimSpace(item.Password)

		// Check for duplicates (same host, port, username, password)
		exists, err := h.adminService.CheckProxyExists(c.Request.Context(), host, item.Port, username, password)
		if err != nil {
			response.InternalError(c, "Failed to check proxy existence: "+err.Error())
			return
		}

		if exists {
			skipped++
			continue
		}

		// Create proxy with default name
		_, err = h.adminService.CreateProxy(c.Request.Context(), &service.CreateProxyInput{
			Name:     "default",
			Protocol: protocol,
			Host:     host,
			Port:     item.Port,
			Username: username,
			Password: password,
		})
		if err != nil {
			// If creation fails due to duplicate, count as skipped
			skipped++
			continue
		}

		created++
	}

	response.Success(c, gin.H{
		"created": created,
		"skipped": skipped,
	})
}
