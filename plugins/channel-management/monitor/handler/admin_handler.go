// Package monitorhandler hosts the channel-monitor HTTP handlers exposed by
// the channel-management plugin. There are two surfaces:
//
//   - AdminHandler   — privileged CRUD + run-now + history endpoints
//     mounted under /api/v1/plugin/channel-management/admin/monitors
//   - UserHandler    — read-only list / detail endpoints mounted under
//     /api/v1/plugin/channel-management/monitors
//
// Both handlers are skeletal: only List is implemented (returning the empty
// page from the service so a smoke test confirms the wiring) and every
// other method is a 501 Not Implemented stub. The real handler bodies (~840
// lines combined) will land in follow-up commits as the repository, runner,
// and checker ports complete.
package monitorhandler

import (
	"net/http"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"

	"github.com/gin-gonic/gin"
)

// AdminHandler is the channel-monitor admin REST handler. The plugin wires
// it under /api/v1/plugin/channel-management/admin/monitors.
type AdminHandler struct {
	monitorService *monitorservice.ChannelMonitorService
}

// NewAdminHandler builds the admin handler. It is safe to pass a nil
// service when running in --no-http mode; routes will simply 503.
func NewAdminHandler(svc *monitorservice.ChannelMonitorService) *AdminHandler {
	return &AdminHandler{monitorService: svc}
}

// List returns a paginated list of monitors. The full filter set (provider /
// enabled / search) lands with the repository port; for now this returns the
// service's pass-through result so the wiring smoke-test succeeds.
func (h *AdminHandler) List(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
		return
	}
	items, total, err := h.monitorService.List(c.Request.Context(), monitorservice.ChannelMonitorListParams{Page: 1, PageSize: 20})
	if err != nil {
		response.ErrorWithDetails(c, http.StatusInternalServerError, err.Error(), "MONITOR_LIST_FAILED", nil)
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

// Create / GetByID / Update / Delete / RunNow / History are placeholders
// returning 501 until the matching repository methods are ported.
func (h *AdminHandler) Create(c *gin.Context)   { notImplemented(c, "create") }
func (h *AdminHandler) GetByID(c *gin.Context)  { notImplemented(c, "get_by_id") }
func (h *AdminHandler) Update(c *gin.Context)   { notImplemented(c, "update") }
func (h *AdminHandler) Delete(c *gin.Context)   { notImplemented(c, "delete") }
func (h *AdminHandler) RunNow(c *gin.Context)   { notImplemented(c, "run_now") }
func (h *AdminHandler) History(c *gin.Context)  { notImplemented(c, "history") }

func notImplemented(c *gin.Context, op string) {
	response.ErrorWithDetails(c, http.StatusNotImplemented,
		"operation "+op+" not yet ported (V5 W6 in progress)",
		"MONITOR_OP_NOT_PORTED",
		map[string]string{"operation": op},
	)
}
