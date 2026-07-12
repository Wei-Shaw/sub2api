package admin

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AsyncMediaConfigHandler 提供异步媒体（fal 等）reconciler 运行时配置的管理接口。
type AsyncMediaConfigHandler struct {
	svc *service.AsyncMediaConfigService
}

func NewAsyncMediaConfigHandler(svc *service.AsyncMediaConfigService) *AsyncMediaConfigHandler {
	return &AsyncMediaConfigHandler{svc: svc}
}

// GetConfig 返回当前生效的 reconciler 运行时配置。
func (h *AsyncMediaConfigHandler) GetConfig(c *gin.Context) {
	cfg, err := h.svc.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig 更新 reconciler 运行时配置（即时热生效，并持久化到 DB）。
func (h *AsyncMediaConfigHandler) UpdateConfig(c *gin.Context) {
	var req service.AsyncMediaRuntimeConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.svc.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidReconcileInterval) || errors.Is(err, service.ErrInvalidFailTimeout) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}
