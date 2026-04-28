package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

type GeneratedImageHandler struct {
	store *service.OpenAIGeneratedImageStore
}

func NewGeneratedImageHandler(store *service.OpenAIGeneratedImageStore) *GeneratedImageHandler {
	return &GeneratedImageHandler{store: store}
}

func (h *GeneratedImageHandler) Get(c *gin.Context) {
	if h == nil || h.store == nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	filename := c.Param("filename")
	rec, data, err := h.store.LoadByFilename(c.Request.Context(), filename)
	if err != nil {
		h.handleLoadError(c, err)
		return
	}

	c.Header("Content-Disposition", `inline; filename="`+rec.Filename+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Data(http.StatusOK, rec.MIME, data)
}

func (h *GeneratedImageHandler) handleLoadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrOpenAIGeneratedImageInvalid):
		c.Status(http.StatusBadRequest)
	case errors.Is(err, service.ErrOpenAIGeneratedImageTooLarge):
		c.Status(http.StatusRequestEntityTooLarge)
	case errors.Is(err, service.ErrOpenAIGeneratedImageNotFound), errors.Is(err, service.ErrOpenAIGeneratedImageExpired):
		c.Status(http.StatusNotFound)
	case errors.Is(err, context.Canceled):
		c.Status(statusClientClosedRequest)
	case errors.Is(err, context.DeadlineExceeded):
		c.Status(http.StatusGatewayTimeout)
	default:
		c.Status(http.StatusInternalServerError)
	}
}
