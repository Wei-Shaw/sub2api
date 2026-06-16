// Package handler hosts the public-facing UploadHandler used to serve
// admin-uploaded images at /api/v1/uploads/:filename. No auth: these files
// are intentionally public assets (e.g. payment "help" graphics shown on
// the recharge page).
package handler

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// uploadFilenamePattern enforces the filename shape produced by
// UploadService.SaveImage: <hex>.<ext>. Keeping the regex strict prevents
// path traversal and disallows arbitrary user-supplied paths.
var uploadFilenamePattern = regexp.MustCompile(`^[a-f0-9]{8,64}\.(jpg|jpeg|png|webp|gif|svg)$`)

// UploadHandler serves stored uploads.
type UploadHandler struct {
	uploadService *service.UploadService
}

// NewUploadHandler constructs the public download handler.
func NewUploadHandler(uploadService *service.UploadService) *UploadHandler {
	return &UploadHandler{uploadService: uploadService}
}

// Get handles GET /api/v1/uploads/:filename. The :filename param must match
// uploadFilenamePattern; otherwise 404 is returned (we don't leak the disk
// layout via a 400 error message).
func (h *UploadHandler) Get(c *gin.Context) {
	name := strings.TrimSpace(c.Param("filename"))
	if name == "" || !uploadFilenamePattern.MatchString(strings.ToLower(name)) {
		response.NotFound(c, "not found")
		return
	}

	dir := h.uploadService.UploadDir()
	if dir == "" {
		response.NotFound(c, "not found")
		return
	}

	full := filepath.Join(dir, name)
	// Belt-and-suspenders: even though the regex rules out "..", make sure
	// the resolved path is still under the upload dir before serving.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		response.NotFound(c, "not found")
		return
	}
	absFull, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(absFull, absDir+string(filepath.Separator)) {
		response.NotFound(c, "not found")
		return
	}

	// Cache for 24h; uploads are content-addressed so the URL is effectively
	// immutable for any given filename.
	c.Header("Cache-Control", "public, max-age=86400, immutable")
	c.File(absFull)
	if c.Writer.Status() == http.StatusNotFound {
		// c.File writes 404 directly; nothing else to do.
		return
	}
}
