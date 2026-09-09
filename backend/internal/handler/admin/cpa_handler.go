package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CpaHandler 提供 CPA（CLIProxyAPI）只读管理面板的 admin 接口。
//
// 所有数据接口都只是把 CPA /v0/management/* 的响应原样透传，Sub2API 不落库、
// 不缓存、也不改写 CPA 的任何状态；唯一允许的“写”是 management key 的本地保存。
type CpaHandler struct {
	cpaService     *service.CpaService
	settingService *service.SettingService
}

// NewCpaHandler 构造 CPA 面板处理器。
func NewCpaHandler(cpaService *service.CpaService, settingService *service.SettingService) *CpaHandler {
	return &CpaHandler{cpaService: cpaService, settingService: settingService}
}

// GetConfig GET /api/v1/admin/cpa/config
// 返回面板配置；management key 只返回是否已配置 + 掩码提示。
func (h *CpaHandler) GetConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	response.Success(c, h.settingService.GetCpaConfig(c.Request.Context()).View())
}

type updateCpaConfigRequest struct {
	Enabled       bool    `json:"enabled"`
	BaseURL       string  `json:"base_url"`
	ManagementKey *string `json:"management_key"`
}

// UpdateConfig PUT /api/v1/admin/cpa/config
// management_key 为 null/省略时保留原值，空字符串表示清除。
func (h *CpaHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	var req updateCpaConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	baseURL := service.NormalizeCpaManagementBaseURL(req.BaseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			response.BadRequest(c, "CPA base URL must be a valid http(s) URL")
			return
		}
	}
	if err := h.settingService.UpdateCpaConfig(c.Request.Context(), req.Enabled, req.BaseURL, req.ManagementKey); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.settingService.GetCpaConfig(c.Request.Context()).View())
}

// TestConnection POST /api/v1/admin/cpa/test
// 探测 CPA 可达性、版本与 management key 是否被接受。
func (h *CpaHandler) TestConnection(c *gin.Context) {
	if h == nil || h.cpaService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	info, err := h.cpaService.Version(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, info)
}

// Overview GET /api/v1/admin/cpa/overview
// 概览：配置视图 + CPA 版本/健康。
func (h *CpaHandler) Overview(c *gin.Context) {
	if h == nil || h.cpaService == nil || h.settingService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	ctx := c.Request.Context()
	cfg := h.settingService.GetCpaConfig(ctx)
	version, err := h.cpaService.Version(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"config":  cfg.View(),
		"version": version,
	})
}

// ListAuthFiles GET /api/v1/admin/cpa/auth-files
func (h *CpaHandler) ListAuthFiles(c *gin.Context) {
	h.passthrough(c, func() (any, error) {
		return h.cpaService.ListAuthFiles(c.Request.Context())
	})
}

// AuthFileQuota GET /api/v1/admin/cpa/auth-files/quota?auth_index=...
func (h *CpaHandler) AuthFileQuota(c *gin.Context) {
	authIndex := strings.TrimSpace(c.Query("auth_index"))
	h.passthrough(c, func() (any, error) {
		return h.cpaService.AuthFileQuota(c.Request.Context(), authIndex)
	})
}

// APIKeyUsage GET /api/v1/admin/cpa/api-key-usage
func (h *CpaHandler) APIKeyUsage(c *gin.Context) {
	h.passthrough(c, func() (any, error) {
		return h.cpaService.APIKeyUsage(c.Request.Context())
	})
}

// RequestLogs GET /api/v1/admin/cpa/logs
func (h *CpaHandler) RequestLogs(c *gin.Context) {
	query := url.Values{}
	if limit := strings.TrimSpace(c.Query("limit")); limit != "" {
		if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
			query.Set("limit", strconv.Itoa(parsed))
		}
	}
	if cursor := strings.TrimSpace(c.Query("cursor")); cursor != "" {
		query.Set("cursor", cursor)
	}
	if after := strings.TrimSpace(c.Query("after")); after != "" {
		query.Set("after", after)
	}
	h.passthrough(c, func() (any, error) {
		return h.cpaService.RequestLogs(c.Request.Context(), query)
	})
}

// RequestErrorLogs GET /api/v1/admin/cpa/error-logs
func (h *CpaHandler) RequestErrorLogs(c *gin.Context) {
	h.passthrough(c, func() (any, error) {
		return h.cpaService.RequestErrorLogs(c.Request.Context())
	})
}

// DownloadErrorLog GET /api/v1/admin/cpa/error-logs/:name
func (h *CpaHandler) DownloadErrorLog(c *gin.Context) {
	if h == nil || h.cpaService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	name := c.Param("name")
	content, err := h.cpaService.DownloadRequestErrorLog(c.Request.Context(), name)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"name":    name,
		"content": string(content),
	})
}

func (h *CpaHandler) passthrough(c *gin.Context, fetch func() (any, error)) {
	if h == nil || h.cpaService == nil {
		response.Error(c, http.StatusServiceUnavailable, "CPA panel is unavailable")
		return
	}
	data, err := fetch()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, data)
}
