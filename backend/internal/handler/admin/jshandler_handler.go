package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
)

type JSHandlerAdminHandler struct {
	svc *jshandler.Service
}

func NewJSHandlerAdminHandler(svc *jshandler.Service) *JSHandlerAdminHandler {
	return &JSHandlerAdminHandler{svc: svc}
}

func (h *JSHandlerAdminHandler) GetConfig(c *gin.Context) {
	raw, err := h.svc.ConfigJSON(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"config": string(raw)})
}

func (h *JSHandlerAdminHandler) UpdateConfig(c *gin.Context) {
	var cfg jshandler.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.svc.UpdateConfig(c.Request.Context(), cfg)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}