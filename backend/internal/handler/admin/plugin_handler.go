package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const pluginUISessionTTL = 30 * time.Minute

// PluginHandler 提供插件安装、生命周期、配置和隔离 UI 资源接口。
type PluginHandler struct {
	manager *service.PluginManager
}

func NewPluginHandler(manager *service.PluginManager) *PluginHandler {
	return &PluginHandler{manager: manager}
}

func (h *PluginHandler) List(c *gin.Context) {
	plugins, err := h.manager.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plugins)
}

func (h *PluginHandler) Get(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	plugin, err := h.manager.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plugin)
}

func (h *PluginHandler) Upload(c *gin.Context) {
	maxBytes := h.manager.MaxUploadBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+(1<<20))
	file, header, err := c.Request.FormFile("plugin")
	if err != nil {
		response.BadRequest(c, "请选择有效的 .s2plugin 文件")
		return
	}
	defer func() { _ = file.Close() }()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".s2plugin") {
		response.BadRequest(c, "插件包扩展名必须是 .s2plugin")
		return
	}
	var installedBy *int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		userID := subject.UserID
		installedBy = &userID
	}
	plugin, err := h.manager.Install(c.Request.Context(), file, installedBy)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, plugin)
}

type pluginEnableRequest struct {
	AcceptUntested bool `json:"accept_untested"`
	RolloutPercent int  `json:"rollout_percent"`
}

func (h *PluginHandler) Enable(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	request := pluginEnableRequest{RolloutPercent: 100}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "启用参数无效")
		return
	}
	plugin, err := h.manager.Enable(c.Request.Context(), id, request.AcceptUntested, request.RolloutPercent)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, plugin)
}

func (h *PluginHandler) Disable(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	plugin, err := h.manager.Disable(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plugin)
}

func (h *PluginHandler) Delete(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "插件已卸载"})
}

func (h *PluginHandler) GetConfig(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	configJSON, err := h.manager.GetConfig(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", configJSON)
}

func (h *PluginHandler) SaveConfig(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4*1024*1024))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		response.BadRequest(c, "插件配置必须是有效 JSON")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.BadRequest(c, "插件配置只能包含一个 JSON 值")
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		response.BadRequest(c, "插件配置无法序列化")
		return
	}
	saved, err := h.manager.SaveConfig(c.Request.Context(), id, raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", saved)
}

func (h *PluginHandler) Test(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	result, err := h.manager.Test(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PluginHandler) CreateUISession(c *gin.Context) {
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	assetToken, expires, err := h.manager.CreateUIAssetToken(c.Request.Context(), id, pluginUISessionTTL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	bridgeToken, err := randomPluginToken()
	if err != nil {
		response.InternalError(c, "创建插件 UI Bridge 失败")
		return
	}
	response.Success(c, gin.H{
		"url":               fmt.Sprintf("/api/v1/plugin-ui/%s/index.html#bridge_token=%s", assetToken, bridgeToken),
		"bridge_token":      bridgeToken,
		"ui_bridge_version": 1,
		"expires_at":        expires,
	})
}

// ServeUIAsset 使用短时随机能力 URL 提供插件静态资源，不向 iframe 暴露管理员凭据。
func (h *PluginHandler) ServeUIAsset(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	pluginID, err := h.manager.ResolveUIAssetToken(token)
	if err != nil {
		c.Status(http.StatusGone)
		return
	}
	relative := strings.TrimPrefix(c.Param("path"), "/")
	data, logicalPath, err := h.manager.ReadUIAsset(c.Request.Context(), pluginID, relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(logicalPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	// sandbox iframe 没有 allow-same-origin，会以不透明来源加载自己的 CSS/JS。
	// 资源 URL 由短时随机能力 Token 保护，Bridge Token 只存在于 fragment 中。
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; navigate-to 'none'")
	c.Data(http.StatusOK, contentType, data)
}

func pluginIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "插件 ID 无效")
		return 0, false
	}
	return id, true
}

func randomPluginToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
