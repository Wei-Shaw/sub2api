package admin

import (
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type UpstreamBalanceMonitorHandler struct {
	service *service.UpstreamBalanceMonitorService
}

func NewUpstreamBalanceMonitorHandler(s *service.UpstreamBalanceMonitorService) *UpstreamBalanceMonitorHandler {
	return &UpstreamBalanceMonitorHandler{service: s}
}

type upstreamBalanceMonitorRequest struct {
	Name                   string   `json:"name" binding:"required,max=100"`
	Type                   string   `json:"type" binding:"required,oneof=sub2api newapi"`
	BaseURL                string   `json:"base_url" binding:"required,max=500"`
	APIKey                 string   `json:"api_key" binding:"max=4000"`
	Cookie                 string   `json:"cookie" binding:"max=16000"`
	UserID                 string   `json:"user_id" binding:"max=100"`
	CredentialMode         string   `json:"credential_mode" binding:"omitempty,oneof=password token"`
	Username               string   `json:"username" binding:"max=255"`
	Password               string   `json:"password" binding:"max=4000"`
	Enabled                *bool    `json:"enabled"`
	DisplayOrder           int      `json:"display_order"`
	ProbeIntervalMinutes   int      `json:"probe_interval_minutes" binding:"required,min=5,max=1440"`
	LowBalanceThresholdUSD *float64 `json:"low_balance_threshold_usd" binding:"omitempty,min=0"`
}

func (h *UpstreamBalanceMonitorHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *UpstreamBalanceMonitorHandler) Create(c *gin.Context) {
	var req upstreamBalanceMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	item, err := h.service.Create(c.Request.Context(), requestToUpstreamBalanceInput(req))
	if err != nil && item == nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *UpstreamBalanceMonitorHandler) Update(c *gin.Context) {
	id, ok := upstreamBalanceID(c)
	if !ok {
		return
	}
	var req upstreamBalanceMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, requestToUpstreamBalanceInput(req))
	if errors.Is(err, service.ErrUpstreamBalanceMonitorNotFound) {
		response.NotFound(c, err.Error())
		return
	}
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *UpstreamBalanceMonitorHandler) Delete(c *gin.Context) {
	id, ok := upstreamBalanceID(c)
	if !ok {
		return
	}
	err := h.service.Delete(c.Request.Context(), id)
	if errors.Is(err, service.ErrUpstreamBalanceMonitorNotFound) {
		response.NotFound(c, err.Error())
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *UpstreamBalanceMonitorHandler) Probe(c *gin.Context) {
	id, ok := upstreamBalanceID(c)
	if !ok {
		return
	}
	item, err := h.service.Probe(c.Request.Context(), id)
	if errors.Is(err, service.ErrUpstreamBalanceMonitorNotFound) {
		response.NotFound(c, err.Error())
		return
	}
	if item != nil {
		response.Success(c, item)
		return
	}
	response.ErrorFrom(c, err)
}

func (h *UpstreamBalanceMonitorHandler) ProbeAll(c *gin.Context) {
	items, err := h.service.ProbeAll(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func upstreamBalanceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid monitor id")
		return 0, false
	}
	return id, true
}

func requestToUpstreamBalanceInput(req upstreamBalanceMonitorRequest) service.UpstreamBalanceMonitorInput {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	threshold := 10.0
	if req.LowBalanceThresholdUSD != nil {
		threshold = *req.LowBalanceThresholdUSD
	}
	return service.UpstreamBalanceMonitorInput{Name: req.Name, Type: req.Type, BaseURL: req.BaseURL, APIKey: req.APIKey, Cookie: req.Cookie, UserID: req.UserID,
		CredentialMode: req.CredentialMode, Username: req.Username, Password: req.Password,
		Enabled: enabled, DisplayOrder: req.DisplayOrder, ProbeIntervalMinutes: req.ProbeIntervalMinutes,
		LowBalanceThresholdUSD: threshold}
}
