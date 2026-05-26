package monitorhandler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/response"

	"github.com/gin-gonic/gin"
)

// monitorServiceUnavailableMessage 是 service 缺失时返回的统一错误码描述。
// 之前 admin_handler.go / user_handler.go 共 9 处一字不差地写
// `response.ErrorWithDetails(c, 503, "monitor service unavailable", "MONITOR_DISABLED", nil)`，
// 现在只在这一处定义；handler 通过 requireMonitorService 调用，
// 路由若想在注册阶段拦截可直接 Use(MonitorServiceGuard(svc))。
const (
	monitorServiceUnavailableMessage = "monitor service unavailable"
	monitorServiceUnavailableCode    = "MONITOR_DISABLED"
)

// writeMonitorServiceUnavailable 写入 503 + MONITOR_DISABLED 错误。
// 单点封装，禁止其它文件直接拼这一对字符串。
func writeMonitorServiceUnavailable(c *gin.Context) {
	response.ErrorWithDetails(
		c,
		http.StatusServiceUnavailable,
		monitorServiceUnavailableMessage,
		monitorServiceUnavailableCode,
		nil,
	)
}

// requireService 是 AdminHandler 的就绪保护：service 未注入时写 503 并返回 false，
// 调用方收到 false 应立即 return，不再访问 h.monitorService。
//
// 这是一个 helper 而非 gin middleware：因为路由组在 plugin.go 注册时已经用
// `if p.monitorAdminHandler != nil` 做过守卫，handler 内部只剩"service 字段是否为 nil"
// 的二次校验——没必要再启一层 middleware 包装。
func (h *AdminHandler) requireService(c *gin.Context) bool {
	if h.monitorService == nil {
		writeMonitorServiceUnavailable(c)
		return false
	}
	return true
}

// requireService 与 AdminHandler 同义，挂在 UserHandler 上以避免跨 receiver 共享。
func (h *UserHandler) requireService(c *gin.Context) bool {
	if h.monitorService == nil {
		writeMonitorServiceUnavailable(c)
		return false
	}
	return true
}
