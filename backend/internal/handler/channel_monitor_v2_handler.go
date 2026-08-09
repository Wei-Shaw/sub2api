package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ChannelMonitorV2Handler struct {
	service *service.ChannelMonitorV2Service
}

func NewChannelMonitorV2Handler(svc *service.ChannelMonitorV2Service) *ChannelMonitorV2Handler {
	return &ChannelMonitorV2Handler{service: svc}
}

// channelMonitorV2IsAdmin is true when the request already passed admin auth
// (shared Dimensions/Errors handlers serve both user and admin route groups).
func channelMonitorV2IsAdmin(c *gin.Context) bool {
	role, ok := middleware.GetUserRoleFromContext(c)
	return ok && role == service.RoleAdmin
}

func (h *ChannelMonitorV2Handler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ChannelMonitorV2Handler) UpdateConfig(c *gin.Context) {
	var input service.ChannelMonitorV2Config
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "invalid channel monitor v2 config")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "user not found in context")
		return
	}
	updated, err := h.service.UpdateConfig(c.Request.Context(), input, input.Version, subject.UserID)
	if err != nil {
		if errors.Is(err, service.ErrChannelMonitorV2ConfigConflict) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, service.ErrChannelMonitorV2InvalidConfig) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *ChannelMonitorV2Handler) Dimensions(c *gin.Context) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	result, err := h.service.Dimensions(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// Admin and user share this handler; only non-admin responses strip volume.
	if !channelMonitorV2IsAdmin(c) {
		service.RedactChannelMonitorV2Dimensions(result)
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) Snapshot(c *gin.Context)      { h.snapshot(c, false) }
func (h *ChannelMonitorV2Handler) AdminSnapshot(c *gin.Context) { h.snapshot(c, true) }
func (h *ChannelMonitorV2Handler) Models(c *gin.Context)        { h.models(c, false) }
func (h *ChannelMonitorV2Handler) AdminModels(c *gin.Context)   { h.models(c, true) }
func (h *ChannelMonitorV2Handler) Matrix(c *gin.Context)        { h.matrix(c, false) }
func (h *ChannelMonitorV2Handler) AdminMatrix(c *gin.Context)   { h.matrix(c, true) }
func (h *ChannelMonitorV2Handler) Users(c *gin.Context)         { h.users(c, false) }
func (h *ChannelMonitorV2Handler) AdminUsers(c *gin.Context)    { h.users(c, true) }

func (h *ChannelMonitorV2Handler) snapshot(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	result, err := h.service.Snapshot(c.Request.Context(), filter, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) models(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	result, err := h.service.Models(c.Request.Context(), filter, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) matrix(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	groupBy, err := service.ParseChannelMonitorV2GroupBy(c.Query("group_by"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.service.Matrix(c.Request.Context(), filter, groupBy, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) Errors(c *gin.Context) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	result, err := h.service.ErrorsForViewer(c.Request.Context(), filter, channelMonitorV2IsAdmin(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) users(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
	}
	subject, exists := middleware.GetAuthSubjectFromContext(c)
	if !exists {
		response.Error(c, http.StatusUnauthorized, "user not found in context")
		return
	}
	result, err := h.service.Users(c.Request.Context(), filter, subject.UserID, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ChannelMonitorV2Handler) parseFilter(c *gin.Context) (service.ChannelMonitorV2Filter, bool) {
	groups, err := parseChannelMonitorV2GroupIDs(queryList(c, "group_id"))
	if err != nil {
		response.BadRequest(c, "invalid group_id")
		return service.ChannelMonitorV2Filter{}, false
	}
	filter, err := h.service.ParseFilter(c.Query("range"), queryList(c, "platform"), queryList(c, "model"), groups)
	if err != nil {
		response.BadRequest(c, err.Error())
		return service.ChannelMonitorV2Filter{}, false
	}
	return filter, true
}

func queryList(c *gin.Context, key string) []string {
	values := c.QueryArray(key)
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func parseChannelMonitorV2GroupIDs(values []string) ([]int64, error) {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("invalid group id")
		}
		result = append(result, id)
	}
	return result, nil
}
