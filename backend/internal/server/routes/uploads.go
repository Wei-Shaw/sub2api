package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterUploadRoutes registers the public download endpoint for
// admin-uploaded images. Mounted at /api/v1/uploads/:filename and
// intentionally outside the admin auth group — uploads are public assets
// (e.g. payment "help" images shown on the recharge page).
//
// The matching admin upload (POST /api/v1/admin/uploads/image) lives in
// admin.go's RegisterAdminRoutes group and uses x-api-key auth.
func RegisterUploadRoutes(r *gin.Engine, h *handler.Handlers) {
	if h == nil || h.Upload == nil {
		return
	}
	r.GET("/api/v1/uploads/:filename", h.Upload.Get)
}
