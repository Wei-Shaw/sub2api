package admin

import (
	"context"
	"errors"
	"fmt"
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
	// ListExt 暴露包含软卸载条目的列表; admin "已卸载" 视图通过 query 参数
	// include_uninstalled=true 触发, 落到 List 时 include=false 即等价于 List。
	ListExt(ctx context.Context, includeUninstalled bool) ([]plugin.PluginInfo, error)
	Get(ctx context.Context, name string) (*plugin.PluginInfo, error)
	Enable(ctx context.Context, name string) error
	Disable(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	UpdateConfig(ctx context.Context, name string, cfg map[string]any) error
	// Uninstall 软卸载: 关进程 + 标记 uninstalled_at, 不删数据。
	Uninstall(ctx context.Context, name string) error
	// Install 撤销软卸载, 让插件回到 disabled / enabled 状态。
	Install(ctx context.Context, name string) error
	// Purge 硬卸载: 物理清除插件全部数据 (含 plugin_migrations / plugin_settings /
	// plugin_settings_schemas / plugins 行)。仅对已经软卸载的插件有效。
	Purge(ctx context.Context, name string) error
	// RemoveFiles 删除插件的二进制文件目录, 保留所有数据库数据。
	// 插件必须先禁用。
	RemoveFiles(ctx context.Context, name string) error
}

// ErrPluginNotFound 由 PluginManager 实现返回,表示请求的插件不存在。
// handler 据此映射为 HTTP 404,而非 500。
//
// 别名指向 plugin.ErrPluginNotFound,以便上层判断时可以直接使用本包的导出符号,
// 无需额外导入 plugin 包。
var ErrPluginNotFound = plugin.ErrPluginNotFound

// ErrPluginIsBuiltin 表示尝试对内置插件执行 Uninstall。映射成 HTTP 400。
var ErrPluginIsBuiltin = plugin.ErrPluginIsBuiltin

// ErrInvalidPluginName 由 PluginManager 实现返回, 表示请求的插件名违反命名
// 规则。映射为 HTTP 400。
var ErrInvalidPluginName = plugin.ErrInvalidPluginName

// ErrPluginNotSoftUninstalled 表示在仍处于 active 的插件上调用 Purge。
// handler 据此映射为 HTTP 409 Conflict, 让前端提示 "先 Uninstall 再 Purge"。
var ErrPluginNotSoftUninstalled = plugin.ErrPluginNotSoftUninstalled

// ErrPluginNotDisabled 表示插件仍在运行, 必须先禁用才能删除文件。
// handler 映射为 HTTP 409 Conflict。
var ErrPluginNotDisabled = plugin.ErrPluginNotDisabled

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
//
// Query parameters:
//   - include_uninstalled: "true" / "1" 时把软卸载条目也带回来 (admin "已卸载"
//     视图)。默认 false: 只返回活动 (uninstalled_at IS NULL) 插件, 与 sidebar /
//     菜单的可见性一致。
func (h *PluginHandler) List(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	includeUninstalled := parseBoolQuery(c.Query("include_uninstalled"))
	items, err := h.manager.ListExt(c.Request.Context(), includeUninstalled)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if items == nil {
		items = []plugin.PluginInfo{}
	}
	response.Success(c, gin.H{"items": items})
}

// parseBoolQuery 解析 admin API 中常见的 "true"/"1"/"yes" → true。空串与
// 其余值落到 false, 与 Gin 默认行为一致。
func parseBoolQuery(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// Uninstall 软卸载插件: 停子进程 + 标记 uninstalled_at, 数据保留, 可通过 Install 撤回。
// POST /api/v1/admin/plugins/:name/uninstall
//
// 内置插件 (BuiltinDir 下的官方插件) 一律拒绝, 返回 400 + ErrPluginIsBuiltin.
// 已经软卸载的条目幂等返回 200。
func (h *PluginHandler) Uninstall(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.Uninstall(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "status": "uninstalled", "mode": "soft"})
}

// Install 撤销软卸载, 让插件回到 disabled 状态 (enabled 字段保持原值)。
// POST /api/v1/admin/plugins/:name/install
//
// 在已经活动的插件上调用是 no-op, 返回 200。
func (h *PluginHandler) Install(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.Install(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "status": "installed"})
}

// purgeRequest 是 Delete (硬卸载) 接口的请求体。 body.name 必须与 URL :name
// 一致 — 这是不可逆操作的 "二次确认" 模式, 与 Kubernetes / Stripe / GitHub
// 的危险操作 UX 一致, 防止误点产生的连锁删除。
type purgeRequest struct {
	Name string `json:"name" binding:"required"`
}

// Delete 硬卸载插件: 物理清除数据 (plugin_migrations / plugin_settings /
// plugin_settings_schemas / plugins), 不可逆。
// DELETE /api/v1/admin/plugins/:name?purge=true
//
// 三道防线:
//  1. URL query 必须带 purge=true — 防止 admin 误用 DELETE 时误删。
//  2. JSON body 必须携带 name 且与 URL :name 一致 — 二次确认避免输错对象。
//  3. plugin manager 拒绝未软卸载的目标 (409 Conflict), 强制走完
//     "先 Uninstall 再 Purge" 的流程。
//
// 内置插件返回 400 (ErrPluginIsBuiltin); 不存在返回 404; 软卸载校验失败返回
// 409; 其他错误 500。
func (h *PluginHandler) Delete(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if !parseBoolQuery(c.Query("purge")) {
		response.BadRequest(c, "purge=true query parameter is required to confirm hard uninstall")
		return
	}
	var req purgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "request body must be {\"name\":\"<plugin>\"}: "+err.Error())
		return
	}
	if req.Name != name {
		response.BadRequest(c, fmt.Sprintf("body name %q does not match URL plugin %q", req.Name, name))
		return
	}
	if h.handleManagerError(c, h.manager.Purge(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "status": "purged"})
}

// RemoveFiles 删除插件的二进制文件, 保留数据库数据。
// POST /api/v1/admin/plugins/:name/remove-files
//
// 前置条件: 插件必须已禁用 (409 if enabled), 不能是内置插件 (400)。
func (h *PluginHandler) RemoveFiles(c *gin.Context) {
	if !h.requireManager(c) {
		return
	}
	name, ok := h.parseName(c)
	if !ok {
		return
	}
	if h.handleManagerError(c, h.manager.RemoveFiles(c.Request.Context(), name)) {
		return
	}
	response.Success(c, gin.H{"name": name, "status": "files_removed"})
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
	if errors.Is(err, ErrPluginIsBuiltin) {
		response.BadRequest(c, "builtin plugins cannot be uninstalled")
		return true
	}
	if errors.Is(err, ErrPluginNotSoftUninstalled) {
		response.Error(c, http.StatusConflict, "plugin must be soft-uninstalled before purge")
		return true
	}
	if errors.Is(err, ErrPluginNotDisabled) {
		response.Error(c, http.StatusConflict, "plugin must be disabled before removing files")
		return true
	}
	if errors.Is(err, ErrInvalidPluginName) {
		response.BadRequest(c, "plugin name contains invalid characters or is too long")
		return true
	}
	response.InternalError(c, err.Error())
	return true
}
