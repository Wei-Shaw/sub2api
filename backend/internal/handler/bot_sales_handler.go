package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BotSalesHandler exposes the narrow admin-to-admin contract used by bot-sales.
type BotSalesHandler struct {
	service *service.BotSalesFulfillmentService
}

func NewBotSalesHandler(fulfillmentService *service.BotSalesFulfillmentService) *BotSalesHandler {
	return &BotSalesHandler{service: fulfillmentService}
}

// Fulfill applies one paid bot-sales order to a Sub2API balance and optionally
// returns a DLG device login code.
// POST /api/v1/admin/bot-sales/fulfill
func (h *BotSalesHandler) Fulfill(c *gin.Context) {
	var req service.BotSalesFulfillmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.IdempotencyKey = c.GetHeader("Idempotency-Key")
	result, err := h.service.Fulfill(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
