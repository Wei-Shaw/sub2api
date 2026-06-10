package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterThemeAssetRoute registers the public theme asset serving route
func RegisterThemeAssetRoute(r *gin.Engine, h *handler.ThemeAssetHandler) {
	if h == nil {
		return
	}
	r.GET("/api/v1/themes/assets/:short/*filepath", h.ServeAsset)
}
