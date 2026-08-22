package admin

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
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

const (
	proxySubscriptionMaxBytes        = 2 << 20
	proxySubscriptionRequestMaxBytes = proxySubscriptionMaxBytes + (64 << 10)
	proxySubscriptionMaxBase64Depth  = 2

	proxySubscriptionQualityPolicyNone            = "none"
	proxySubscriptionQualityPolicyDisableD        = "disable_d"
	proxySubscriptionQualityPolicyDisableCOrBelow = "disable_c_or_below"
	proxySubscriptionQualityPolicyDisableBOrBelow = "disable_b_or_below"
)

// ImportProxySubscriptionRequest imports supported HTTP/SOCKS nodes from a subscription.
type ImportProxySubscriptionRequest struct {
	URL           string `json:"url"`
	Content       string `json:"content"`
	NamePrefix    string `json:"name_prefix"`
	QualityPolicy string `json:"quality_policy"`
}

type importProxySubscriptionCandidate struct {
	Name     string
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
}

type importProxySubscriptionParseResult struct {
	Total       int
	Parsed      []importProxySubscriptionCandidate
	Unsupported int
	Invalid     int
}

type importProxySubscriptionResult struct {
	Total           int      `json:"total"`
	Parsed          int      `json:"parsed"`
	Created         int      `json:"created"`
	Skipped         int      `json:"skipped"`
	Unsupported     int      `json:"unsupported"`
	Invalid         int      `json:"invalid"`
	Failed          int      `json:"failed"`
	QualityChecked  int      `json:"quality_checked"`
	QualityDisabled int      `json:"quality_disabled"`
	QualityFailed   int      `json:"quality_failed"`
	Errors          []string `json:"errors,omitempty"`
}

type applyProxyQualityPolicyRequest struct {
	IDs           []int64 `json:"ids" binding:"required,min=1,max=1000,dive,min=1"`
	QualityPolicy string  `json:"quality_policy" binding:"required"`
}

type applyProxyQualityPolicyResult struct {
	Total           int      `json:"total"`
	QualityChecked  int      `json:"quality_checked"`
	QualityDisabled int      `json:"quality_disabled"`
	QualityFailed   int      `json:"quality_failed"`
	DisabledIDs     []int64  `json:"disabled_ids"`
	Errors          []string `json:"errors,omitempty"`
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

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt > 0 {
		t := time.Unix(*req.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	proxy, err := h.adminService.UpdateProxy(c.Request.Context(), proxyID, &service.UpdateProxyInput{
		Name:           strings.TrimSpace(req.Name),
		Protocol:       strings.TrimSpace(req.Protocol),
		Host:           strings.TrimSpace(req.Host),
		Port:           req.Port,
		Username:       strings.TrimSpace(req.Username),
		Password:       strings.TrimSpace(req.Password),
		Status:         strings.TrimSpace(req.Status),
		ExpiresAt:      expiresAt,
		FallbackMode:   strings.TrimSpace(req.FallbackMode),
		BackupProxyID:  req.BackupProxyID,
		ExpiryWarnDays: req.ExpiryWarnDays,
	})
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

// ApplyQualityPolicy checks selected proxies and disables those below the
// chosen quality threshold.
// POST /api/v1/admin/proxies/apply-quality-policy
func (h *ProxyHandler) ApplyQualityPolicy(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req applyProxyQualityPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	policy, err := normalizeSubscriptionQualityPolicy(req.QualityPolicy)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if policy == proxySubscriptionQualityPolicyNone {
		response.BadRequest(c, "quality_policy must disable at least one grade")
		return
	}

	ids := uniquePositiveProxyIDs(req.IDs)
	if len(ids) == 0 {
		response.BadRequest(c, "ids is required")
		return
	}
	req.IDs = ids
	req.QualityPolicy = policy

	executeAdminIdempotentJSON(c, "admin.proxies.apply_quality_policy", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		result := applyProxyQualityPolicyResult{Total: len(ids)}
		for _, id := range ids {
			quality, err := h.adminService.CheckProxyQuality(ctx, id)
			if err != nil {
				result.QualityFailed++
				result.Errors = append(result.Errors, "proxy "+strconv.FormatInt(id, 10)+" quality check: "+err.Error())
				continue
			}
			result.QualityChecked++
			if !shouldDisableProxyForSubscriptionQuality(policy, quality.Grade) {
				continue
			}
			if _, err := h.adminService.UpdateProxy(ctx, id, &service.UpdateProxyInput{Status: service.StatusDisabled}); err != nil {
				result.QualityFailed++
				result.Errors = append(result.Errors, "proxy "+strconv.FormatInt(id, 10)+" disable: "+err.Error())
				continue
			}
			result.QualityDisabled++
			result.DisabledIDs = append(result.DisabledIDs, id)
		}
		return result, nil
	})
}

func uniquePositiveProxyIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
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
			response.ErrorFrom(c, err)
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

// ImportSubscription handles importing supported proxies from a Clash/Mihomo
// subscription or a plain proxy URL list.
// POST /api/v1/admin/proxies/import-subscription
func (h *ProxyHandler) ImportSubscription(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, proxySubscriptionRequestMaxBytes)
	var req ImportProxySubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Content = strings.TrimSpace(req.Content)
	req.NamePrefix = strings.TrimSpace(req.NamePrefix)
	qualityPolicy, err := normalizeSubscriptionQualityPolicy(req.QualityPolicy)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	req.QualityPolicy = qualityPolicy
	if req.URL == "" && req.Content == "" {
		response.BadRequest(c, "url or content is required")
		return
	}
	if len(req.Content) > proxySubscriptionMaxBytes {
		response.BadRequest(c, "subscription content is too large")
		return
	}

	executeAdminIdempotentJSON(c, "admin.proxies.import_subscription", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		content := req.Content
		if content == "" {
			fetched, err := fetchProxySubscription(ctx, req.URL)
			if err != nil {
				return nil, err
			}
			content = fetched
		}

		parsed, err := parseProxySubscription(content, req.NamePrefix)
		if err != nil {
			return nil, err
		}

		result := importProxySubscriptionResult{
			Total:       parsed.Total,
			Parsed:      len(parsed.Parsed),
			Unsupported: parsed.Unsupported,
			Invalid:     parsed.Invalid,
		}

		for _, item := range parsed.Parsed {
			exists, err := h.adminService.CheckProxyExists(ctx, item.Host, item.Port, item.Username, item.Password)
			if err != nil {
				return nil, err
			}
			if exists {
				result.Skipped++
				continue
			}
			createdProxy, err := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
				Name:     item.Name,
				Protocol: item.Protocol,
				Host:     item.Host,
				Port:     item.Port,
				Username: item.Username,
				Password: item.Password,
			})
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, item.Name+": "+err.Error())
				continue
			}
			result.Created++

			if qualityPolicy == proxySubscriptionQualityPolicyNone {
				continue
			}
			quality, err := h.adminService.CheckProxyQuality(ctx, createdProxy.ID)
			if err != nil {
				result.QualityFailed++
				result.Errors = append(result.Errors, item.Name+" quality check: "+err.Error())
				continue
			}
			result.QualityChecked++
			if shouldDisableProxyForSubscriptionQuality(qualityPolicy, quality.Grade) {
				if _, err := h.adminService.UpdateProxy(ctx, createdProxy.ID, &service.UpdateProxyInput{Status: service.StatusDisabled}); err != nil {
					result.QualityFailed++
					result.Errors = append(result.Errors, item.Name+" disable: "+err.Error())
					continue
				}
				result.QualityDisabled++
			}
		}

		return result, nil
	})
}

func normalizeSubscriptionQualityPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default", proxySubscriptionQualityPolicyDisableD:
		return proxySubscriptionQualityPolicyDisableD, nil
	case proxySubscriptionQualityPolicyNone:
		return proxySubscriptionQualityPolicyNone, nil
	case proxySubscriptionQualityPolicyDisableCOrBelow:
		return proxySubscriptionQualityPolicyDisableCOrBelow, nil
	case proxySubscriptionQualityPolicyDisableBOrBelow:
		return proxySubscriptionQualityPolicyDisableBOrBelow, nil
	default:
		return "", errors.New("invalid quality_policy")
	}
}

func shouldDisableProxyForSubscriptionQuality(policy, grade string) bool {
	if policy == proxySubscriptionQualityPolicyNone {
		return false
	}
	rank, ok := proxySubscriptionQualityGradeRank(grade)
	if !ok {
		return true
	}
	switch policy {
	case proxySubscriptionQualityPolicyDisableBOrBelow:
		return rank <= proxySubscriptionQualityGradeRankValue("B")
	case proxySubscriptionQualityPolicyDisableCOrBelow:
		return rank <= proxySubscriptionQualityGradeRankValue("C")
	case proxySubscriptionQualityPolicyDisableD:
		return rank <= proxySubscriptionQualityGradeRankValue("D")
	default:
		return true
	}
}

func proxySubscriptionQualityGradeRank(grade string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A":
		return 4, true
	case "B":
		return 3, true
	case "C":
		return 2, true
	case "D":
		return 1, true
	case "F":
		return 0, true
	default:
		return -1, false
	}
}

func proxySubscriptionQualityGradeRankValue(grade string) int {
	rank, _ := proxySubscriptionQualityGradeRank(grade)
	return rank
}

func fetchProxySubscription(ctx context.Context, rawURL string) (string, error) {
	parsed, err := validateSubscriptionFetchURL(ctx, rawURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sub2api-proxy-subscription-import/1.0")

	client := newSubscriptionFetchHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("subscription fetch failed: " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, proxySubscriptionMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > proxySubscriptionMaxBytes {
		return "", errors.New("subscription is too large")
	}
	return string(data), nil
}

func validateSubscriptionFetchURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid subscription URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("subscription URL must use http or https")
	}
	if _, err := lookupSubscriptionHostIPs(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func newSubscriptionFetchHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}

			ips, err := lookupSubscriptionHostIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			ip := chooseSubscriptionDialIP(network, ips)
			if ip == nil {
				return nil, errors.New("subscription URL host has no address for requested network")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("subscription URL redirects too many times")
			}
			_, err := validateSubscriptionFetchURL(req.Context(), req.URL.String())
			return err
		},
	}
}

func lookupSubscriptionHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	host = strings.TrimSpace(host)
	if host == "" || strings.Contains(host, "%") {
		return nil, errors.New("subscription URL host is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if !isPublicSubscriptionIP(ip) {
			return nil, errors.New("subscription URL host resolves to a private address")
		}
		return []net.IP{ip}, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("subscription URL host has no addresses")
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if !isPublicSubscriptionIP(addr.IP) {
			return nil, errors.New("subscription URL host resolves to a private address")
		}
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func chooseSubscriptionDialIP(network string, ips []net.IP) net.IP {
	for _, ip := range ips {
		switch network {
		case "tcp4":
			if ip.To4() != nil {
				return ip
			}
		case "tcp6":
			if ip.To4() == nil {
				return ip
			}
		default:
			return ip
		}
	}
	return nil
}

func isPublicSubscriptionIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsUnspecified()
}

func parseProxySubscription(content, namePrefix string) (importProxySubscriptionParseResult, error) {
	content, err := decodeProxySubscriptionContent(content)
	if err != nil {
		return importProxySubscriptionParseResult{}, err
	}
	if content == "" {
		return importProxySubscriptionParseResult{}, errors.New("subscription content is empty")
	}

	if result, ok := parseMihomoProxySubscription(content, namePrefix); ok {
		return result, nil
	}

	result := importProxySubscriptionParseResult{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result.Total++
		item, ok := parseProxySubscriptionURL(line, namePrefix, result.Total)
		if !ok {
			result.Invalid++
			continue
		}
		result.Parsed = append(result.Parsed, item)
	}
	if result.Total == 0 {
		return result, errors.New("no proxy entries found")
	}
	return result, nil
}

func decodeProxySubscriptionContent(content string) (string, error) {
	normalized := strings.TrimSpace(content)
	if len(normalized) > proxySubscriptionMaxBytes {
		return "", errors.New("subscription content is too large")
	}
	for depth := 0; depth < proxySubscriptionMaxBase64Depth; depth++ {
		decoded, ok, err := decodeMaybeBase64SubscriptionOnce(normalized)
		if err != nil {
			return "", err
		}
		if !ok {
			return normalized, nil
		}
		normalized = strings.TrimSpace(decoded)
		if normalized == "" {
			return "", errors.New("subscription content is empty")
		}
		if len(normalized) > proxySubscriptionMaxBytes {
			return "", errors.New("subscription content is too large")
		}
	}
	if _, ok, err := decodeMaybeBase64SubscriptionOnce(normalized); err != nil {
		return "", err
	} else if ok {
		return "", errors.New("subscription base64 nesting is too deep")
	}
	return normalized, nil
}

func decodeMaybeBase64SubscriptionOnce(content string) (string, bool, error) {
	normalized := strings.TrimSpace(content)
	if normalized == "" || strings.ContainsAny(normalized, "\n\r{}[]:") {
		return "", false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(normalized)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(normalized)
	}
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(normalized)
	}
	if err != nil {
		return "", false, nil
	}
	if len(decoded) > proxySubscriptionMaxBytes {
		return "", false, errors.New("subscription content is too large")
	}
	out := strings.TrimSpace(string(decoded))
	return out, out != "", nil
}

func parseMihomoProxySubscription(content, namePrefix string) (importProxySubscriptionParseResult, bool) {
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil || len(doc.Proxies) == 0 {
		return importProxySubscriptionParseResult{}, false
	}

	result := importProxySubscriptionParseResult{Total: len(doc.Proxies)}
	for i, raw := range doc.Proxies {
		item, status := parseMihomoProxyNode(raw, namePrefix, i+1)
		switch status {
		case "parsed":
			result.Parsed = append(result.Parsed, item)
		case "unsupported":
			result.Unsupported++
		default:
			result.Invalid++
		}
	}
	return result, true
}

func parseMihomoProxyNode(raw map[string]any, namePrefix string, index int) (importProxySubscriptionCandidate, string) {
	protocol := normalizeSubscriptionProxyProtocol(stringFromAny(raw["type"]))
	if protocol == "" {
		if strings.TrimSpace(stringFromAny(raw["type"])) == "" {
			return importProxySubscriptionCandidate{}, "invalid"
		}
		return importProxySubscriptionCandidate{}, "unsupported"
	}

	host := strings.TrimSpace(stringFromAny(raw["server"]))
	port := intFromAny(raw["port"])
	if host == "" || port < 1 || port > 65535 {
		return importProxySubscriptionCandidate{}, "invalid"
	}

	name := strings.TrimSpace(stringFromAny(raw["name"]))
	if name == "" {
		name = "subscription proxy " + strconv.Itoa(index)
	}
	return importProxySubscriptionCandidate{
		Name:     buildSubscriptionProxyName(namePrefix, name),
		Protocol: protocol,
		Host:     host,
		Port:     port,
		Username: firstNonEmptyString(raw["username"], raw["user"]),
		Password: firstNonEmptyString(raw["password"], raw["pass"]),
	}, "parsed"
}

func parseProxySubscriptionURL(line, namePrefix string, index int) (importProxySubscriptionCandidate, bool) {
	parsed, err := url.Parse(line)
	if err != nil {
		return importProxySubscriptionCandidate{}, false
	}
	protocol := normalizeSubscriptionProxyProtocol(parsed.Scheme)
	if protocol == "" || parsed.Hostname() == "" {
		return importProxySubscriptionCandidate{}, false
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return importProxySubscriptionCandidate{}, false
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	name := strings.TrimSpace(parsed.Fragment)
	if name == "" {
		name = "subscription proxy " + strconv.Itoa(index)
	}
	return importProxySubscriptionCandidate{
		Name:     buildSubscriptionProxyName(namePrefix, name),
		Protocol: protocol,
		Host:     parsed.Hostname(),
		Port:     port,
		Username: username,
		Password: password,
	}, true
}

func normalizeSubscriptionProxyProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "http":
		return "http"
	case "https":
		return "https"
	case "socks", "socks5":
		return "socks5"
	case "socks5h":
		return "socks5h"
	default:
		return ""
	}
}

func buildSubscriptionProxyName(prefix, name string) string {
	prefix = strings.TrimSpace(prefix)
	name = strings.TrimSpace(name)
	if prefix == "" {
		return name
	}
	return prefix + " " + name
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if out := strings.TrimSpace(stringFromAny(value)); out != "" {
			return out
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return ""
	default:
		return ""
	}
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}
