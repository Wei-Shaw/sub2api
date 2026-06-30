package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// TLSFingerprintProfileHandler 处理 TLS 指纹模板的 HTTP 请求
type TLSFingerprintProfileHandler struct {
	service   *service.TLSFingerprintProfileService
	collector *service.TLSFingerprintCollectorService
}

// NewTLSFingerprintProfileHandler 创建 TLS 指纹模板处理器。
// collector 用可变参数注入，使既有(仅 profileService)调用方仍可编译。
func NewTLSFingerprintProfileHandler(profileService *service.TLSFingerprintProfileService, collectors ...*service.TLSFingerprintCollectorService) *TLSFingerprintProfileHandler {
	h := &TLSFingerprintProfileHandler{service: profileService}
	if len(collectors) > 0 {
		h.collector = collectors[0]
	}
	return h
}

// CreateTLSFingerprintProfileRequest 创建模板请求
type CreateTLSFingerprintProfileRequest struct {
	Name                string   `json:"name" binding:"required"`
	Description         *string  `json:"description"`
	EnableGREASE        *bool    `json:"enable_grease"`
	CipherSuites        []uint16 `json:"cipher_suites"`
	Curves              []uint16 `json:"curves"`
	PointFormats        []uint16 `json:"point_formats"`
	SignatureAlgorithms []uint16 `json:"signature_algorithms"`
	ALPNProtocols       []string `json:"alpn_protocols"`
	SupportedVersions   []uint16 `json:"supported_versions"`
	KeyShareGroups      []uint16 `json:"key_share_groups"`
	PSKModes            []uint16 `json:"psk_modes"`
	Extensions          []uint16 `json:"extensions"`
}

// UpdateTLSFingerprintProfileRequest 更新模板请求（部分更新）
type UpdateTLSFingerprintProfileRequest struct {
	Name                *string  `json:"name"`
	Description         *string  `json:"description"`
	EnableGREASE        *bool    `json:"enable_grease"`
	CipherSuites        []uint16 `json:"cipher_suites"`
	Curves              []uint16 `json:"curves"`
	PointFormats        []uint16 `json:"point_formats"`
	SignatureAlgorithms []uint16 `json:"signature_algorithms"`
	ALPNProtocols       []string `json:"alpn_protocols"`
	SupportedVersions   []uint16 `json:"supported_versions"`
	KeyShareGroups      []uint16 `json:"key_share_groups"`
	PSKModes            []uint16 `json:"psk_modes"`
	Extensions          []uint16 `json:"extensions"`
}

// List 获取所有模板
// GET /api/v1/admin/tls-fingerprint-profiles
func (h *TLSFingerprintProfileHandler) List(c *gin.Context) {
	profiles, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profiles)
}

// GetByID 根据 ID 获取模板
// GET /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	profile, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if profile == nil {
		response.NotFound(c, "Profile not found")
		return
	}

	response.Success(c, profile)
}

// Create 创建模板
// POST /api/v1/admin/tls-fingerprint-profiles
func (h *TLSFingerprintProfileHandler) Create(c *gin.Context) {
	var req CreateTLSFingerprintProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	profile := &model.TLSFingerprintProfile{
		Name:                req.Name,
		Description:         req.Description,
		CipherSuites:        req.CipherSuites,
		Curves:              req.Curves,
		PointFormats:        req.PointFormats,
		SignatureAlgorithms: req.SignatureAlgorithms,
		ALPNProtocols:       req.ALPNProtocols,
		SupportedVersions:   req.SupportedVersions,
		KeyShareGroups:      req.KeyShareGroups,
		PSKModes:            req.PSKModes,
		Extensions:          req.Extensions,
	}

	if req.EnableGREASE != nil {
		profile.EnableGREASE = *req.EnableGREASE
	}

	created, err := h.service.Create(c.Request.Context(), profile)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, created)
}

// Update 更新模板（支持部分更新）
// PUT /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	var req UpdateTLSFingerprintProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if existing == nil {
		response.NotFound(c, "Profile not found")
		return
	}

	// 部分更新
	profile := &model.TLSFingerprintProfile{
		ID:                  id,
		Name:                existing.Name,
		Description:         existing.Description,
		EnableGREASE:        existing.EnableGREASE,
		CipherSuites:        existing.CipherSuites,
		Curves:              existing.Curves,
		PointFormats:        existing.PointFormats,
		SignatureAlgorithms: existing.SignatureAlgorithms,
		ALPNProtocols:       existing.ALPNProtocols,
		SupportedVersions:   existing.SupportedVersions,
		KeyShareGroups:      existing.KeyShareGroups,
		PSKModes:            existing.PSKModes,
		Extensions:          existing.Extensions,
	}

	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Description != nil {
		profile.Description = req.Description
	}
	if req.EnableGREASE != nil {
		profile.EnableGREASE = *req.EnableGREASE
	}
	if req.CipherSuites != nil {
		profile.CipherSuites = req.CipherSuites
	}
	if req.Curves != nil {
		profile.Curves = req.Curves
	}
	if req.PointFormats != nil {
		profile.PointFormats = req.PointFormats
	}
	if req.SignatureAlgorithms != nil {
		profile.SignatureAlgorithms = req.SignatureAlgorithms
	}
	if req.ALPNProtocols != nil {
		profile.ALPNProtocols = req.ALPNProtocols
	}
	if req.SupportedVersions != nil {
		profile.SupportedVersions = req.SupportedVersions
	}
	if req.KeyShareGroups != nil {
		profile.KeyShareGroups = req.KeyShareGroups
	}
	if req.PSKModes != nil {
		profile.PSKModes = req.PSKModes
	}
	if req.Extensions != nil {
		profile.Extensions = req.Extensions
	}

	updated, err := h.service.Update(c.Request.Context(), profile)
	if err != nil {
		if _, ok := err.(*model.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, updated)
}

// Delete 删除模板
// DELETE /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Profile deleted successfully"})
}

// CollectorStatus 获取 TLS 指纹收集器状态
// GET /api/v1/admin/tls-fingerprint-profiles/collector/status
func (h *TLSFingerprintProfileHandler) CollectorStatus(c *gin.Context) {
	if h.collector == nil {
		response.Success(c, service.TLSFingerprintCollectorStatus{})
		return
	}
	response.Success(c, h.collector.Status())
}

// StartCollector 从页面启动 TLS 指纹收集器
// POST /api/v1/admin/tls-fingerprint-profiles/collector/start
func (h *TLSFingerprintProfileHandler) StartCollector(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	status, err := h.collector.Start(c.Request.Context())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, status)
}

// StopCollector 从页面停止 TLS 指纹收集器
// POST /api/v1/admin/tls-fingerprint-profiles/collector/stop
func (h *TLSFingerprintProfileHandler) StopCollector(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	if err := h.collector.Stop(c.Request.Context()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, h.collector.Status())
}

// CreateCollectorSession 创建一次短期采集会话
// POST /api/v1/admin/tls-fingerprint-profiles/collector/sessions
func (h *TLSFingerprintProfileHandler) CreateCollectorSession(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	session, err := h.collector.CreateSession()
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, session)
}

// ListCollectorCaptures 获取采集会话结果
// GET /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token/captures
func (h *TLSFingerprintProfileHandler) ListCollectorCaptures(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	records, err := h.collector.ListCaptures(c.Param("token"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, records)
}

// DeleteCollectorSession 删除采集会话
// DELETE /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token
func (h *TLSFingerprintProfileHandler) DeleteCollectorSession(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	h.collector.DeleteSession(c.Param("token"))
	response.Success(c, gin.H{"deleted": true})
}
