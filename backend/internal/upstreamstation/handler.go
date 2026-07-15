package upstreamstation

import (
	"errors"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type stationRequest struct {
	Name               string            `json:"name" binding:"required,max=128"`
	SiteType           string            `json:"site_type" binding:"omitempty,oneof=auto newapi sub2api"`
	BaseURL            string            `json:"base_url" binding:"required,max=1024"`
	CredentialMode     string            `json:"credential_mode" binding:"required,oneof=password token api_key"`
	Credentials        *Credentials      `json:"credentials" binding:"required"`
	RechargeMultiplier float64           `json:"recharge_multiplier" binding:"omitempty,gt=0"`
	RechargeSource     string            `json:"recharge_source" binding:"omitempty,oneof=manual auto"`
	Enabled            *bool             `json:"enabled"`
	AutoSync           *bool             `json:"auto_sync"`
	FixedRoutes        []FixedRouteInput `json:"fixed_routes"`
}

type stationUpdateRequest struct {
	Name               *string      `json:"name" binding:"omitempty,max=128"`
	SiteType           *string      `json:"site_type" binding:"omitempty,oneof=auto newapi sub2api"`
	BaseURL            *string      `json:"base_url" binding:"omitempty,max=1024"`
	CredentialMode     *string      `json:"credential_mode" binding:"omitempty,oneof=password token api_key"`
	Credentials        *Credentials `json:"credentials"`
	RechargeMultiplier *float64     `json:"recharge_multiplier" binding:"omitempty,gt=0"`
	RechargeSource     *string      `json:"recharge_source" binding:"omitempty,oneof=manual auto"`
	Enabled            *bool        `json:"enabled"`
	AutoSync           *bool        `json:"auto_sync"`
}

type routeUpdateRequest struct {
	RemoteGroupName    *string   `json:"remote_group_name"`
	Models             *[]string `json:"models"`
	GroupRate          *float64  `json:"group_rate" binding:"omitempty,gte=0"`
	RechargeMultiplier *float64  `json:"recharge_multiplier" binding:"omitempty,gt=0"`
	Schedulable        *bool     `json:"schedulable"`
}

func (h *Handler) RegisterRoutes(admin *gin.RouterGroup) {
	stations := admin.Group("/upstream-stations")
	stations.GET("", h.List)
	stations.POST("", h.Create)
	stations.POST("/sync-all", h.SyncAll)
	stations.GET("/:id", h.Get)
	stations.PUT("/:id", h.Update)
	stations.DELETE("/:id", h.Delete)
	stations.POST("/:id/test", h.Test)
	stations.POST("/:id/sync", h.Sync)
	stations.GET("/:id/routes", h.ListRoutes)
	stations.POST("/:id/routes", h.CreateFixedRoute)
	stations.GET("/:id/logs", h.ListLogs)

	routes := admin.Group("/upstream-routes")
	routes.PUT("/:id", h.UpdateRoute)
	routes.POST("/:id/test", h.TestRoute)
	routes.POST("/:id/schedulable", h.SetRouteSchedulable)
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.service.ListStations(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	item, err := h.service.GetStation(c.Request.Context(), id)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) Create(c *gin.Context) {
	var req stationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_STATION", err.Error()))
		return
	}
	recharge := req.RechargeMultiplier
	if recharge <= 0 {
		recharge = 1
	}
	item, err := h.service.CreateStation(c.Request.Context(), StationInput{
		Name: req.Name, SiteType: req.SiteType, BaseURL: req.BaseURL, CredentialMode: req.CredentialMode,
		Credentials: req.Credentials, RechargeMultiplier: recharge, RechargeSource: req.RechargeSource,
		Enabled: boolDefault(req.Enabled, true), AutoSync: boolDefault(req.AutoSync, true), FixedRoutes: req.FixedRoutes,
	})
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	var req stationUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_STATION", err.Error()))
		return
	}
	item, err := h.service.UpdateStation(c.Request.Context(), id, StationUpdateInput{
		Name: req.Name, SiteType: req.SiteType, BaseURL: req.BaseURL, CredentialMode: req.CredentialMode,
		Credentials: req.Credentials, RechargeMultiplier: req.RechargeMultiplier, RechargeSource: req.RechargeSource,
		Enabled: req.Enabled, AutoSync: req.AutoSync,
	})
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteStation(c.Request.Context(), id); err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

func (h *Handler) Test(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	result, err := h.service.TestStation(c.Request.Context(), id)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) Sync(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	result, err := h.service.SyncStation(c.Request.Context(), id)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) SyncAll(c *gin.Context) {
	response.Success(c, h.service.SyncAll(c.Request.Context()))
}

func (h *Handler) ListRoutes(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	items, err := h.service.ListRoutes(c.Request.Context(), id)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *Handler) CreateFixedRoute(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	var req FixedRouteInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_ROUTE", err.Error()))
		return
	}
	item, err := h.service.CreateFixedRoute(c.Request.Context(), id, req)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *Handler) UpdateRoute(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	var req routeUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_ROUTE", err.Error()))
		return
	}
	item, err := h.service.UpdateRoute(c.Request.Context(), id, RouteUpdateInput{
		RemoteGroupName: req.RemoteGroupName, Models: req.Models, GroupRate: req.GroupRate,
		RechargeMultiplier: req.RechargeMultiplier, Schedulable: req.Schedulable,
	})
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *Handler) TestRoute(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	route, err := h.service.repository.GetRoute(c.Request.Context(), id)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	result, err := h.service.SyncStation(c.Request.Context(), route.StationID)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) SetRouteSchedulable(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	var req struct {
		Schedulable bool `json:"schedulable"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_ROUTE", err.Error()))
		return
	}
	if err := h.service.SetRouteSchedulable(c.Request.Context(), id, req.Schedulable); err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, gin.H{"schedulable": req.Schedulable})
}

func (h *Handler) ListLogs(c *gin.Context) {
	id, ok := parseUpstreamID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.service.ListSyncLogs(c.Request.Context(), id, limit)
	if err != nil {
		writeUpstreamError(c, err)
		return
	}
	response.Success(c, items)
}

func parseUpstreamID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_UPSTREAM_ID", "invalid upstream id"))
		return 0, false
	}
	return id, true
}

func writeUpstreamError(c *gin.Context, err error) {
	if errorsIsNotFound(err) {
		response.ErrorFrom(c, infraerrors.NotFound("UPSTREAM_NOT_FOUND", err.Error()))
		return
	}
	response.ErrorFrom(c, infraerrors.BadRequest("UPSTREAM_OPERATION_FAILED", err.Error()))
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
