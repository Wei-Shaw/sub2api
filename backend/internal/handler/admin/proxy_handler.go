package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ProxyHandler handles admin proxy management
type ProxyHandler struct {
	adminService service.AdminService
	healthSvc    *service.ProxyHealthService
}

// NewProxyHandler creates a new admin proxy handler
func NewProxyHandler(adminService service.AdminService, healthSvc *service.ProxyHealthService) *ProxyHandler {
	return &ProxyHandler{
		adminService: adminService,
		healthSvc:    healthSvc,
	}
}

// HealthScan triggers one full proxy health probe round (admin).
// POST /api/v1/admin/proxies/health-scan
func (h *ProxyHandler) HealthScan(c *gin.Context) {
	if h.healthSvc == nil {
		response.ErrorFrom(c, fmt.Errorf("proxy health service not configured"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	result, err := h.healthSvc.RunScan(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := gin.H{
		"probed":    result.Probed,
		"isolated":  result.Isolated,
		"recovered": result.Recovered,
		"skipped":   result.Skipped,
		"errors":    result.Errors,
	}
	if m := h.healthSvc.Metrics(); m != nil {
		out["metrics"] = m.Snapshot()
	}
	response.Success(c, out)
}

// GetHealth returns Redis + DB health detail for one proxy.
// GET /api/v1/admin/proxies/:id/health
func (h *ProxyHandler) GetHealth(c *gin.Context) {
	if h.healthSvc == nil {
		response.ErrorFrom(c, fmt.Errorf("proxy health service not configured"))
		return
	}
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}
	detail, err := h.healthSvc.GetHealth(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, detail)
}

// GetHealthConfig GET /api/v1/admin/proxies/health/config
func (h *ProxyHandler) GetHealthConfig(c *gin.Context) {
	if h.healthSvc == nil {
		response.ErrorFrom(c, fmt.Errorf("proxy health service not configured"))
		return
	}
	cfg, err := h.healthSvc.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateHealthConfig PUT /api/v1/admin/proxies/health/config
func (h *ProxyHandler) UpdateHealthConfig(c *gin.Context) {
	if h.healthSvc == nil {
		response.ErrorFrom(c, fmt.Errorf("proxy health service not configured"))
		return
	}
	var req service.ProxyHealthSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	out, err := h.healthSvc.UpdateConfig(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, out)
}

// GetHealthRuntime GET /api/v1/admin/proxies/health/runtime
func (h *ProxyHandler) GetHealthRuntime(c *gin.Context) {
	if h.healthSvc == nil {
		response.ErrorFrom(c, fmt.Errorf("proxy health service not configured"))
		return
	}
	response.Success(c, h.healthSvc.RuntimeSnapshot(c.Request.Context()))
}

// CreateProxyRequest represents create proxy request
type CreateProxyRequest struct {
	Name           string `json:"name" binding:"required"`
	Protocol       string `json:"protocol" binding:"required,oneof=http https socks5 socks5h"`
	Host           string `json:"host" binding:"required"`
	Port           int    `json:"port" binding:"required,min=1,max=65535"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	ExpiresAt      *int64 `json:"expires_at"`
	FallbackMode   string `json:"fallback_mode" binding:"omitempty,oneof=none proxy direct"`
	BackupProxyID  *int64 `json:"backup_proxy_id"`
	ExpiryWarnDays int    `json:"expiry_warn_days" binding:"omitempty,min=0"`
}

// UpdateProxyRequest represents update proxy request
type UpdateProxyRequest struct {
	Name           string `json:"name"`
	Protocol       string `json:"protocol" binding:"omitempty,oneof=http https socks5 socks5h"`
	Host           string `json:"host"`
	Port           int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Status         string `json:"status" binding:"omitempty,oneof=active inactive"`
	ExpiresAt      *int64 `json:"expires_at"`
	FallbackMode   string `json:"fallback_mode" binding:"omitempty,oneof=none proxy direct"`
	BackupProxyID  *int64 `json:"backup_proxy_id"`
	ExpiryWarnDays int    `json:"expiry_warn_days" binding:"omitempty,min=0"`
}

// List handles listing all proxies with pagination
// GET /api/v1/admin/proxies
func (h *ProxyHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	protocol := c.Query("protocol")
	status := c.Query("status")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "id")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	// 标准化和验证 search 参数
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
	}

	proxies, total, err := h.adminService.ListProxiesWithAccountCount(c.Request.Context(), page, pageSize, protocol, status, search, sortBy, sortOrder)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminProxyWithAccountCount, 0, len(proxies))
	for i := range proxies {
		out = append(out, *dto.ProxyWithAccountCountFromServiceAdmin(&proxies[i]))
	}
	response.Paginated(c, out, total, page, pageSize)
}

// GetAll handles getting all active proxies without pagination
// GET /api/v1/admin/proxies/all
// Optional query param: with_count=true to include account count per proxy
func (h *ProxyHandler) GetAll(c *gin.Context) {
	withCount := c.Query("with_count") == "true"

	if withCount {
		proxies, err := h.adminService.GetAllProxiesWithAccountCount(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		out := make([]dto.AdminProxyWithAccountCount, 0, len(proxies))
		for i := range proxies {
			out = append(out, *dto.ProxyWithAccountCountFromServiceAdmin(&proxies[i]))
		}
		response.Success(c, out)
		return
	}

	proxies, err := h.adminService.GetAllProxies(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.AdminProxy, 0, len(proxies))
	for i := range proxies {
		out = append(out, *dto.ProxyFromServiceAdmin(&proxies[i]))
	}
	response.Success(c, out)
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.ProxyFromServiceAdmin(proxy))
}

// Create handles creating a new proxy
// POST /api/v1/admin/proxies
func (h *ProxyHandler) Create(c *gin.Context) {
	var req CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.proxies.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		var expiresAt *time.Time
		if req.ExpiresAt != nil && *req.ExpiresAt > 0 {
			t := time.Unix(*req.ExpiresAt, 0).UTC()
			expiresAt = &t
		}
		proxy, err := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
			Name:           strings.TrimSpace(req.Name),
			Protocol:       strings.TrimSpace(req.Protocol),
			Host:           strings.TrimSpace(req.Host),
			Port:           req.Port,
			Username:       strings.TrimSpace(req.Username),
			Password:       strings.TrimSpace(req.Password),
			ExpiresAt:      expiresAt,
			FallbackMode:   strings.TrimSpace(req.FallbackMode),
			BackupProxyID:  req.BackupProxyID,
			ExpiryWarnDays: req.ExpiryWarnDays,
		})
		if err != nil {
			return nil, err
		}
		return dto.ProxyFromServiceAdmin(proxy), nil
	})
}

// Update handles updating a proxy
// PUT /api/v1/admin/proxies/:id
// Partial updates: omitted JSON keys leave existing values unchanged. Explicit
// null on expires_at / backup_proxy_id clears those fields. Empty password is
// only applied when the key is present (so clients can clear credentials).
func (h *ProxyHandler) Update(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	// Re-validate known shape (oneof / port range) via typed bind.
	var req UpdateProxyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := &service.UpdateProxyInput{
		Name:     strings.TrimSpace(req.Name),
		Protocol: strings.TrimSpace(req.Protocol),
		Host:     strings.TrimSpace(req.Host),
		Port:     req.Port,
		Status:   strings.TrimSpace(req.Status),
	}
	if _, ok := raw["username"]; ok {
		u := strings.TrimSpace(req.Username)
		input.Username = &u
	}
	if _, ok := raw["password"]; ok {
		p := strings.TrimSpace(req.Password)
		input.Password = &p
	}
	if rawExp, ok := raw["expires_at"]; ok {
		input.ExpiresAtProvided = true
		if string(rawExp) != "null" && req.ExpiresAt != nil && *req.ExpiresAt > 0 {
			t := time.Unix(*req.ExpiresAt, 0).UTC()
			input.ExpiresAt = &t
		}
	}
	if _, ok := raw["fallback_mode"]; ok {
		mode := strings.TrimSpace(req.FallbackMode)
		input.FallbackMode = &mode
	}
	if rawBackup, ok := raw["backup_proxy_id"]; ok {
		input.BackupProxyIDProvided = true
		if string(rawBackup) != "null" {
			input.BackupProxyID = req.BackupProxyID
		}
	}
	if _, ok := raw["expiry_warn_days"]; ok {
		d := req.ExpiryWarnDays
		input.ExpiryWarnDays = &d
	}

	proxy, err := h.adminService.UpdateProxy(c.Request.Context(), proxyID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.ProxyFromServiceAdmin(proxy))
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Proxy deleted successfully"})
}

// BatchDelete handles batch deleting proxies
// POST /api/v1/admin/proxies/batch-delete
func (h *ProxyHandler) BatchDelete(c *gin.Context) {
	type BatchDeleteRequest struct {
		IDs []int64 `json:"ids" binding:"required,min=1"`
	}

	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.adminService.BatchDeleteProxies(c.Request.Context(), req.IDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
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
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}

// CheckQuality handles checking proxy quality across common AI targets.
// POST /api/v1/admin/proxies/:id/quality-check
func (h *ProxyHandler) CheckQuality(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	result, err := h.adminService.CheckProxyQuality(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}

// GetStats returns real proxy usage stats (account count + request summary).
// GET /api/v1/admin/proxies/:id/stats
func (h *ProxyHandler) GetStats(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	stats, err := h.adminService.GetProxyStats(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// GetProxyAccounts handles getting accounts using a proxy
// GET /api/v1/admin/proxies/:id/accounts
func (h *ProxyHandler) GetProxyAccounts(c *gin.Context) {
	proxyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid proxy ID")
		return
	}

	accounts, err := h.adminService.GetProxyAccounts(c.Request.Context(), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.ProxyAccountSummary, 0, len(accounts))
	for i := range accounts {
		out = append(out, *dto.ProxyAccountSummaryFromService(&accounts[i]))
	}
	response.Success(c, out)
}

// BatchCreateProxyItem represents a single proxy in batch create request
type BatchCreateProxyItem struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol" binding:"required,oneof=http https socks5 socks5h"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required,min=1,max=65535"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// BatchCreateRequest represents batch create proxies request
type BatchCreateRequest struct {
	Proxies []BatchCreateProxyItem `json:"proxies" binding:"required,min=1"`
}

// BatchCreateError is one failed/skipped item in a batch create response.
type BatchCreateError struct {
	Index  int    `json:"index"`
	Host   string `json:"host,omitempty"`
	Port   int    `json:"port,omitempty"`
	Reason string `json:"reason"`
}

// defaultBatchProxyName builds a stable display name from endpoint fields.
func defaultBatchProxyName(protocol, host string, port int) string {
	protocol = strings.TrimSpace(protocol)
	host = strings.TrimSpace(host)
	if protocol == "" {
		protocol = "proxy"
	}
	if host == "" {
		host = "unknown"
	}
	name := fmt.Sprintf("%s://%s:%d", protocol, host, port)
	// Proxy.name MaxLen(100) in schema.
	if len(name) > 100 {
		name = name[:100]
	}
	return name
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
	failed := 0
	errors := make([]BatchCreateError, 0)

	for i, item := range req.Proxies {
		// Trim all string fields
		host := strings.TrimSpace(item.Host)
		protocol := strings.TrimSpace(item.Protocol)
		username := strings.TrimSpace(item.Username)
		password := strings.TrimSpace(item.Password)
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = defaultBatchProxyName(protocol, host, item.Port)
		} else if len(name) > 100 {
			name = name[:100]
		}

		// Check for duplicates (same host, port, username, password)
		exists, err := h.adminService.CheckProxyExists(c.Request.Context(), host, item.Port, username, password)
		if err != nil {
			failed++
			errors = append(errors, BatchCreateError{
				Index: i, Host: host, Port: item.Port, Reason: err.Error(),
			})
			continue
		}

		if exists {
			skipped++
			errors = append(errors, BatchCreateError{
				Index: i, Host: host, Port: item.Port, Reason: "duplicate host/port/auth",
			})
			continue
		}

		_, err = h.adminService.CreateProxy(c.Request.Context(), &service.CreateProxyInput{
			Name:     name,
			Protocol: protocol,
			Host:     host,
			Port:     item.Port,
			Username: username,
			Password: password,
		})
		if err != nil {
			failed++
			errors = append(errors, BatchCreateError{
				Index: i, Host: host, Port: item.Port, Reason: err.Error(),
			})
			continue
		}

		created++
	}

	response.Success(c, gin.H{
		"created": created,
		"skipped": skipped,
		"failed":  failed,
		"errors":  errors,
	})
}
