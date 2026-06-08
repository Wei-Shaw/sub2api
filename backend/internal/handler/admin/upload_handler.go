// Package admin contains admin HTTP handlers. UploadHandler accepts admin
// multipart uploads (currently only images) and persists them via
// service.UploadService.
package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UploadHandler handles admin image uploads.
type UploadHandler struct {
	uploadService *service.UploadService
}

// NewUploadHandler constructs an UploadHandler. uploadService must not be nil
// (wire ensures this).
func NewUploadHandler(uploadService *service.UploadService) *UploadHandler {
	return &UploadHandler{uploadService: uploadService}
}

// uploadImageResponse is the success payload returned by UploadImage.
// Field name is `url` so the frontend can plug it directly into <img :src>
// or persist it in a settings record without further transformation.
type uploadImageResponse struct {
	URL string `json:"url"`
}

// UploadImage handles POST /api/v1/admin/uploads/image.
//
// Expects multipart/form-data with a single "file" field. Returns the public
// URL where the file can be retrieved (no auth required) on success.
func (h *UploadHandler) UploadImage(c *gin.Context) {
	header, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file field")
		return
	}
	if header.Size <= 0 {
		response.BadRequest(c, "empty file")
		return
	}
	if header.Size > service.UploadMaxImageSizeBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "file too large")
		return
	}

	mimeType := header.Header.Get("Content-Type")
	// Some clients send empty Content-Type for the part; reject explicitly so
	// service-layer error message is clearer than "unsupported MIME type \"\"".
	if strings.TrimSpace(mimeType) == "" {
		response.BadRequest(c, "missing Content-Type header on uploaded part")
		return
	}

	src, err := header.Open()
	if err != nil {
		response.InternalError(c, "open uploaded file: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()

	filename, err := h.uploadService.SaveImage(c.Request.Context(), src, mimeType, header.Size)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUploadFileTooLarge):
			response.Error(c, http.StatusRequestEntityTooLarge, err.Error())
		case errors.Is(err, service.ErrUploadUnsupportedMIME),
			errors.Is(err, service.ErrUploadEmptyFile):
			response.BadRequest(c, err.Error())
		case errors.Is(err, service.ErrUploadDirNotConfigured):
			response.InternalError(c, "upload directory not configured")
		default:
			response.InternalError(c, "save upload: "+err.Error())
		}
		return
	}

	response.Success(c, uploadImageResponse{URL: "/api/v1/uploads/" + filename})
}
