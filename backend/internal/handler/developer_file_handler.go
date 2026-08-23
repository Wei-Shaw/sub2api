package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DeveloperFileHandler struct {
	service *service.DeveloperFileService
}

func NewDeveloperFileHandler(service *service.DeveloperFileService) *DeveloperFileHandler {
	return &DeveloperFileHandler{service: service}
}

func (h *DeveloperFileHandler) Upload(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Developer key is required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.DeveloperFileMaxBytes+(1<<20))
	header, err := c.FormFile("file")
	if c.Request.MultipartForm != nil {
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	}
	if err != nil {
		response.BadRequest(c, "missing file field")
		return
	}
	if header.Size <= 0 {
		response.BadRequest(c, "uploaded file is empty")
		return
	}
	if header.Size > service.DeveloperFileMaxBytes {
		response.ErrorFrom(c, service.ErrDeveloperFileTooLarge())
		return
	}
	file, err := header.Open()
	if err != nil {
		response.BadRequest(c, "failed to open uploaded file")
		return
	}
	defer func() { _ = file.Close() }()
	result, err := h.service.Upload(c.Request.Context(), subject.UserID, header.Filename, header.Header.Get("Content-Type"), header.Size, file)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, result)
}

func (h *DeveloperFileHandler) Delete(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Developer key is required")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	if err := h.service.Delete(c.Request.Context(), subject.UserID, req.URL); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
