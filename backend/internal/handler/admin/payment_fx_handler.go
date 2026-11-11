package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// PaymentFXHandler exposes the daily FX sync configuration and a manual sync.
type PaymentFXHandler struct {
	service *service.PaymentFXSyncService
}

func NewPaymentFXHandler(svc *service.PaymentFXSyncService) *PaymentFXHandler {
	return &PaymentFXHandler{service: svc}
}

// GetConfig returns the FX sync configuration and last run state.
// GET /api/v1/admin/settings/payment-fx
func (h *PaymentFXHandler) GetConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, service.ErrPaymentFXSyncUnavailable)
		return
	}
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig replaces the FX sync configuration.
// PUT /api/v1/admin/settings/payment-fx
func (h *PaymentFXHandler) UpdateConfig(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, service.ErrPaymentFXSyncUnavailable)
		return
	}
	var req service.UpdatePaymentFXSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// SyncNow runs one FX sync immediately.
// POST /api/v1/admin/settings/payment-fx/sync
func (h *PaymentFXHandler) SyncNow(c *gin.Context) {
	if h == nil || h.service == nil {
		response.ErrorFrom(c, service.ErrPaymentFXSyncUnavailable)
		return
	}
	cfg, err := h.service.SyncNow(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}
