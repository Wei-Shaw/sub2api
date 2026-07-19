package admin

import (
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var ingressRejectReasons = map[string]struct{}{
	"query_api_key_deprecated": {}, "api_key_required": {}, "invalid_api_key": {},
	"invalid_auth_rate_limited": {},
	"api_key_auth_overloaded":   {},
	"api_key_disabled":          {}, "ip_restricted": {}, "user_inactive": {}, "group_deleted": {},
	"group_disabled": {}, "group_not_allowed": {}, "group_unassigned": {}, "other": {},
}

var ingressRejectRouteFamilies = map[string]struct{}{
	"antigravity": {}, "gemini": {}, "codex": {}, "messages": {}, "responses": {},
	"chat_completions": {}, "images": {}, "videos": {}, "embeddings": {}, "models": {}, "other": {},
}

var ingressRejectProtocols = map[string]struct{}{
	"google": {}, "anthropic": {}, "openai": {}, "gateway": {}, "other": {},
}

// ListIngressRejects returns bounded security aggregates, never raw credentials or request bodies.
func (h *OpsHandler) ListIngressRejects(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}
	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	filter := &service.OpsIngressRejectFilter{Page: page, PageSize: pageSize}
	if !startTime.IsZero() {
		filter.StartTime = &startTime
	}
	if !endTime.IsZero() {
		filter.EndTime = &endTime
	}
	if filter.RejectReason, err = parseIngressRejectEnum(c, "reason", ingressRejectReasons); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if filter.RouteFamily, err = parseIngressRejectEnum(c, "route_family", ingressRejectRouteFamilies); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if filter.Protocol, err = parseIngressRejectEnum(c, "protocol", ingressRejectProtocols); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if raw := strings.TrimSpace(c.Query("client_ip")); raw != "" {
		addr, parseErr := netip.ParseAddr(raw)
		if parseErr != nil {
			response.BadRequest(c, "Invalid client_ip")
			return
		}
		addr = addr.Unmap()
		if addr.Is6() {
			addr = netip.PrefixFrom(addr, 64).Masked().Addr()
		}
		filter.ClientIP = addr.String()
	}
	if filter.UserID, err = parseOptionalPositiveID(c, "user_id"); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if filter.APIKeyID, err = parseOptionalPositiveID(c, "api_key_id"); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.opsService.ListIngressRejects(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpsHandler) GetIngressRejectHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.opsService.GetIngressRejectHealth())
}

func parseIngressRejectEnum(c *gin.Context, name string, allowed map[string]struct{}) (string, error) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return "", nil
	}
	if _, ok := allowed[value]; !ok {
		return "", &ingressRejectQueryError{message: "Invalid " + name}
	}
	return value, nil
}

func parseOptionalPositiveID(c *gin.Context, name string) (*int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, &ingressRejectQueryError{message: "Invalid " + name}
	}
	return &value, nil
}

type ingressRejectQueryError struct{ message string }

func (e *ingressRejectQueryError) Error() string { return e.message }
