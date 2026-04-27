package monitorhandler

import (
	"net/http"

	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"

	"github.com/gin-gonic/gin"
)

// UserHandler is the read-only channel-monitor handler exposed to logged-in
// users. The plugin mounts it under /api/v1/plugin/channel-management/
// monitors. As with the admin counterpart, only List is wired; the rest are
// stubs awaiting the repository and aggregator ports.
type UserHandler struct {
	monitorService *monitorservice.ChannelMonitorService
}

// NewUserHandler builds the user handler.
func NewUserHandler(svc *monitorservice.ChannelMonitorService) *UserHandler {
	return &UserHandler{monitorService: svc}
}

// List returns the user-facing card grid data. Stub returns an empty list
// until BatchMonitorStatusSummary can produce real data (which depends on
// the repository port).
func (h *UserHandler) List(c *gin.Context) {
	if h.monitorService == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "monitor service unavailable", "MONITOR_DISABLED", nil)
		return
	}
	response.Success(c, gin.H{"items": []any{}, "total": 0})
}

// GetStatus / Detail are 501 placeholders.
func (h *UserHandler) GetStatus(c *gin.Context) { notImplemented(c, "user_get_status") }
func (h *UserHandler) Detail(c *gin.Context)    { notImplemented(c, "user_detail") }
