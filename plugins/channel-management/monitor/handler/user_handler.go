package monitorhandler

import (
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/dto"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/gin-gonic/gin"
)

// UserHandler is the read-only channel-monitor handler exposed to logged-in
// users. The plugin mounts it under /api/v1/plugin/channel-management/monitors.
type UserHandler struct {
	monitorService *monitorservice.ChannelMonitorService
	settings       pluginsdk.SettingsClient
}

// NewUserHandler builds the user handler. settings is optional — passing
// nil disables the runtime feature flag check (the user list is always
// returned). Production wires the SDK SettingsClient so the admin can
// flip the master switch without redeploying.
func NewUserHandler(svc *monitorservice.ChannelMonitorService, settings pluginsdk.SettingsClient) *UserHandler {
	return &UserHandler{monitorService: svc, settings: settings}
}

// featureEnabled returns the runtime "enabled" master switch. nil settings
// (older host without W3) is treated as enabled to preserve backwards
// compatibility — operators upgrading from V4 won't suddenly lose access.
func (h *UserHandler) featureEnabled(ctx *gin.Context) bool {
	if h.settings == nil {
		return true
	}
	rt := monitorservice.LoadMonitorRuntime(ctx.Request.Context(), h.settings)
	return rt.Enabled
}

// --- Response shapes ---

type channelMonitorUserListItem struct {
	ID                   int64                                `json:"id"`
	Name                 string                               `json:"name"`
	Provider             string                               `json:"provider"`
	GroupName            string                               `json:"group_name"`
	PrimaryModel         string                               `json:"primary_model"`
	PrimaryStatus        string                               `json:"primary_status"`
	PrimaryLatencyMs     *int                                 `json:"primary_latency_ms"`
	PrimaryPingLatencyMs *int                                 `json:"primary_ping_latency_ms"`
	Availability7d       float64                              `json:"availability_7d"`
	ExtraModels          []dto.ChannelMonitorExtraModelStatus `json:"extra_models"`
	Timeline             []channelMonitorUserTimelinePoint    `json:"timeline"`
}

type channelMonitorUserTimelinePoint struct {
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	CheckedAt     string `json:"checked_at"`
}

type channelMonitorUserDetailResponse struct {
	ID        int64                         `json:"id"`
	Name      string                        `json:"name"`
	Provider  string                        `json:"provider"`
	GroupName string                        `json:"group_name"`
	Models    []channelMonitorUserModelStat `json:"models"`
}

type channelMonitorUserModelStat struct {
	Model           string  `json:"model"`
	LatestStatus    string  `json:"latest_status"`
	LatestLatencyMs *int    `json:"latest_latency_ms"`
	Availability7d  float64 `json:"availability_7d"`
	Availability15d float64 `json:"availability_15d"`
	Availability30d float64 `json:"availability_30d"`
	AvgLatency7dMs  *int    `json:"avg_latency_7d_ms"`
}

func userMonitorViewToItem(v *monitorservice.UserMonitorView) channelMonitorUserListItem {
	extras := make([]dto.ChannelMonitorExtraModelStatus, 0, len(v.ExtraModels))
	for _, e := range v.ExtraModels {
		extras = append(extras, dto.ChannelMonitorExtraModelStatus{
			Model:     e.Model,
			Status:    e.Status,
			LatencyMs: e.LatencyMs,
		})
	}
	timeline := make([]channelMonitorUserTimelinePoint, 0, len(v.Timeline))
	for _, p := range v.Timeline {
		timeline = append(timeline, channelMonitorUserTimelinePoint{
			Status:        p.Status,
			LatencyMs:     p.LatencyMs,
			PingLatencyMs: p.PingLatencyMs,
			CheckedAt:     p.CheckedAt.UTC().Format(time.RFC3339),
		})
	}
	return channelMonitorUserListItem{
		ID:                   v.ID,
		Name:                 v.Name,
		Provider:             v.Provider,
		GroupName:            v.GroupName,
		PrimaryModel:         v.PrimaryModel,
		PrimaryStatus:        v.PrimaryStatus,
		PrimaryLatencyMs:     v.PrimaryLatencyMs,
		PrimaryPingLatencyMs: v.PrimaryPingLatencyMs,
		Availability7d:       v.Availability7d,
		ExtraModels:          extras,
		Timeline:             timeline,
	}
}

func userMonitorDetailToResponse(d *monitorservice.UserMonitorDetail) *channelMonitorUserDetailResponse {
	models := make([]channelMonitorUserModelStat, 0, len(d.Models))
	for _, m := range d.Models {
		models = append(models, channelMonitorUserModelStat{
			Model:           m.Model,
			LatestStatus:    m.LatestStatus,
			LatestLatencyMs: m.LatestLatencyMs,
			Availability7d:  m.Availability7d,
			Availability15d: m.Availability15d,
			Availability30d: m.Availability30d,
			AvgLatency7dMs:  m.AvgLatency7dMs,
		})
	}
	return &channelMonitorUserDetailResponse{
		ID:        d.ID,
		Name:      d.Name,
		Provider:  d.Provider,
		GroupName: d.GroupName,
		Models:    models,
	}
}

// --- Handlers ---

// List GET /monitors — user-facing card grid data.
func (h *UserHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	if !h.featureEnabled(c) {
		response.Success(c, gin.H{"items": []channelMonitorUserListItem{}})
		return
	}
	views, err := h.monitorService.ListUserView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]channelMonitorUserListItem, 0, len(views))
	for _, v := range views {
		items = append(items, userMonitorViewToItem(v))
	}
	response.Success(c, gin.H{"items": items})
}

// GetStatus GET /monitors/:id — user-facing per-monitor detail.
func (h *UserHandler) GetStatus(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	if !h.featureEnabled(c) {
		response.ErrorFrom(c, monitorservice.ErrChannelMonitorNotFound)
		return
	}
	id, ok := ParseChannelMonitorID(c)
	if !ok {
		return
	}
	detail, err := h.monitorService.GetUserDetail(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, userMonitorDetailToResponse(detail))
}

// Detail keeps the original method name for backward compatibility with the
// route registration; it forwards to GetStatus.
func (h *UserHandler) Detail(c *gin.Context) { h.GetStatus(c) }
