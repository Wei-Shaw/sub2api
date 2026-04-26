package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/gin-gonic/gin"
)

// 静态 mime 推断, 不依赖 stdlib 的 net/http.DetectContentType (它对 ESM JS
// 经常误识别成 text/plain). 仅覆盖插件 frontend bundle 实际会用到的扩展名.
var pluginAssetMimeByExt = map[string]string{
	".js":    "application/javascript; charset=utf-8",
	".mjs":   "application/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".map":   "application/json; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".html":  "text/html; charset=utf-8",
}

const (
	// pluginAssetCacheControl 给 5 分钟的可缓存窗口, 加 must-revalidate 让浏览器
	// 在 ETag 命中时仍走 304, 避免插件热更新后客户端长时间持有旧 bundle.
	pluginAssetCacheControl = "public, max-age=300, must-revalidate"
)

// RegisterPluginAssetRoutes 挂载 /api/v1/plugin-assets/:plugin/*path,
// 把请求转换为 PluginManager.FetchFrontendAsset 拉到的字节流并返回给浏览器.
//
// pm 为 nil 时不注册任何路由; 这种场景出现在 PluginManager 初始化失败的降级路径,
// 此时插件功能整体不可用, 路由也不应存在以免误导客户端.
func RegisterPluginAssetRoutes(r *gin.Engine, pm *plugin.PluginManager) {
	if pm == nil {
		return
	}
	r.GET("/api/v1/plugin-assets/:plugin/*path", func(c *gin.Context) {
		pluginName := c.Param("plugin")
		assetPath := strings.TrimPrefix(c.Param("path"), "/")

		asset, err := pm.FetchFrontendAsset(c.Request.Context(), pluginName, assetPath)
		if err != nil {
			if errors.Is(err, plugin.ErrPluginNotRunning) {
				c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "plugin not running"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}

		// 304 Not Modified
		if match := c.GetHeader("If-None-Match"); match != "" && match == asset.ETag {
			c.Header("ETag", asset.ETag)
			c.Status(http.StatusNotModified)
			return
		}

		ext := lowerExt(assetPath)
		mime := pluginAssetMimeByExt[ext]
		if mime == "" {
			mime = "application/octet-stream"
		}
		c.Header("ETag", asset.ETag)
		c.Header("Cache-Control", pluginAssetCacheControl)
		c.Data(http.StatusOK, mime, asset.Data)
	})
}

// lowerExt 抽取最后一个 "." 之后的扩展名, 包含点号. 没有扩展名时返回空字符串.
func lowerExt(p string) string {
	idx := strings.LastIndex(p, ".")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(p[idx:])
}
