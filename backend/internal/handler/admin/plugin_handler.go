package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"

	"github.com/gin-gonic/gin"
)

// PluginManager 是 PluginHandler 依赖的最小化接口,
// 仅包含管理 API 所需的方法。具体实现由 internal/plugin 包提供,
// 通过 wire 在 NewPluginHandler 处注入。
//
// 设计原则:保持接口最小化以便 mock 测试,并避免 handler 与具体实现耦合。
// PluginInfo 直接复用 plugin 包中的类型,统一序列化语义,
// 避免在 handler 内重复定义同名结构体导致字段漂移。
type PluginManager interface {
	List(ctx context.Context) ([]plugin.PluginInfo, error)
	Get(ctx context.Context, name string) (*plugin.PluginInfo, error)
	Enable(ctx context.Context, name string) error
	Disable(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	UpdateConfig(ctx context.Context, name string, cfg map[string]any) error
}

// ErrPluginNotFound 由 PluginManager 实现返回,表示请求的插件不存在。
// handler 据此映射为 HTTP 404,而非 500。
//
// 别名指向 plugin.ErrPluginNotFound,以便上层判断时可以直接使用本包的导出符号,
// 无需额外导入 plugin 包。
var ErrPluginNotFound = plugin.ErrPluginNotFound

// PluginHandler 提供插件管理的 admin HTTP 接口。
//
// manager 可能为 nil（当插件功能未启用时），此时所有接口返回 503。
type PluginHandler struct {
	manager PluginManager
}

// NewPluginHandler 创建 PluginHandler。manager 可为 nil。
func NewPluginHandler(manager PluginManager) *PluginHandler {
	return &PluginHandler{manager: manager}
}

// UpdatePluginConfigRequest 是 UpdateConfig 接口的请求体。
type UpdatePluginConfigRequest struct {
	Config map[string]any `json:"config" binding:"required"`
}

// List 列出所有已注册的插件。
// GET /api/v1/admin/plugins
func (h *PluginHandler) List(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	items, err := h.manager.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if items == nil {
		items = []plugin.PluginInfo{}
	}
	response.Success(c, gin.H{"items": items})
}

// Get 返回单个插件的详细信息。
// GET /api/v1/admin/plugins/:name
func (h *PluginHandler) Get(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	info, err := h.manager.Get(c.Request.Context(), name)
	if h.handleManagerError(c, err) {
		return
	}
	response.Success(c, info)
}

// Enable 启用插件并启动子进程。
// POST /api/v1/admin/plugins/:name/enable
func (h *PluginHandler) Enable(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.Enable(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "enabled": true})
}

// Disable 禁用插件并停止子进程。
// POST /api/v1/admin/plugins/:name/disable
func (h *PluginHandler) Disable(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.Disable(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "enabled": false})
}

// Restart 强制重启插件子进程。
// POST /api/v1/admin/plugins/:name/restart
func (h *PluginHandler) Restart(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.Restart(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "restarted": true})
}

// UpdateConfig 更新插件的配置 JSON。具体生效语义(立即生效或重启后生效)
// 由 PluginManager 实现决定。
// PUT /api/v1/admin/plugins/:name/config
func (h *PluginHandler) UpdateConfig(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	var req UpdatePluginConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.handleManagerError(c, h.manager.UpdateConfig(c.Request.Context(), name, req.Config)) {
		return
	}
	response.Success(c, gin.H{"name": name, "config": req.Config})
}

// requireManager 在 manager 未注入时返回 503，避免后续 nil panic。
func (h *PluginHandler) requireManager(c *gin.Context) bool {
	if h.manager == nil {
		response.Error(c, http.StatusServiceUnavailable, "plugin manager not enabled")
		return false
	}
	return true
}

// parseName 提取并校验路径参数 :name。
// 出错时已写入 400 响应，调用方直接 return。
// 字符集 + 长度都通过 plugin.IsValidPluginName 统一控制，挡住路径穿越类
// 输入 (foo/../bar / "..", 大写, NUL, unicode 等)。
func (h *PluginHandler) parseName(c *gin.Context) (string, bool) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		response.BadRequest(c, "plugin name is required")
		return "", false
	}
	if !plugin.IsValidPluginName(name) {
		response.BadRequest(c, "plugin name contains invalid characters or is too long")
		return "", false
	}
	return name, true
}

// handleManagerError 把 PluginManager 返回的错误映射成 HTTP 响应。
// 返回 true 表示已写入响应，调用方应直接 return。
func (h *PluginHandler) handleManagerError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPluginNotFound) {
		response.NotFound(c, "plugin not found")
		return true
	}
	response.InternalError(c, err.Error())
	return true
}
