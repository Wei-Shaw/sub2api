package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ChannelMonitorV1MatrixHandler 渠道监控 V1 主动探测的「模型广场 Pro」矩阵 handler。
//
// 响应 JSON 与 GET /api/v1/channel-monitor-v2/matrix 完全同形，
// 前端可零改动解析（唯一的语义差别：一个 V1 渠道监控 = 一张卡）。
type ChannelMonitorV1MatrixHandler struct {
	matrixService  *service.ChannelMonitorV1MatrixService
	settingService *service.SettingService
}

// NewChannelMonitorV1MatrixHandler 创建 handler。
// settingService 用于每次请求读取功能开关；未开启或模式不是 v1 时返回 200 + 空 items。
func NewChannelMonitorV1MatrixHandler(
	matrixService *service.ChannelMonitorV1MatrixService,
	settingService *service.SettingService,
) *ChannelMonitorV1MatrixHandler {
	return &ChannelMonitorV1MatrixHandler{
		matrixService:  matrixService,
		settingService: settingService,
	}
}

// featureEnabled 返回当前是否处于 V1 主动探测模式（enabled=true 且 mode=v1）。
// settingService 为 nil（测试场景）视为启用，与 ChannelMonitorUserHandler.featureEnabled 一致。
func (h *ChannelMonitorV1MatrixHandler) featureEnabled(c *gin.Context) bool {
	if h.settingService == nil {
		return true
	}
	runtime := h.settingService.GetChannelMonitorRuntime(c.Request.Context())
	return runtime.Enabled && runtime.Mode == service.ChannelMonitorModeV1
}

// Matrix GET /api/v1/channel-monitors/matrix?range=<token>&group_by=platform_group
//
// 门禁与官方 V1 用户端一致：功能关闭或模式为 v2 时返回 200 + 空 items（不返回 403）。
// 非法 range / group_by 走与 V2 handler 相同的 400 错误路径。
func (h *ChannelMonitorV1MatrixHandler) Matrix(c *gin.Context) {
	window, err := service.ParseChannelMonitorV1MatrixWindow(c.Query("range"), time.Now().UTC())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupBy, err := service.ParseChannelMonitorV2GroupBy(c.Query("group_by"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if groupBy != service.ChannelMonitorV2GroupByPlatformGroup {
		// V1 一行 = 一个渠道监控，只有 platform_group 这一种形状；
		// 其余合法的 V2 分组在这里无法如实表达，明确报 400 而不是静默换维度。
		response.BadRequest(c, "unsupported group_by for channel monitor v1 matrix: "+string(groupBy))
		return
	}
	if !h.featureEnabled(c) {
		response.Success(c, service.NewEmptyChannelMonitorV1Matrix(window))
		return
	}
	if h.matrixService == nil {
		response.Error(c, 500, "channel monitor v1 matrix service unavailable")
		return
	}
	matrix, err := h.matrixService.Matrix(c.Request.Context(), window, false)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, matrix)
}
