// Package handler — provider-instance CRUD for the payment admin API.
//
// Split from admin_payment_handler.go to stay within the 500-line limit.
// All methods share the same *AdminPaymentHandler receiver.
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// ListProviders returns all payment provider instances.
func (h *AdminPaymentHandler) ListProviders(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	providers, err := h.configService.ListProviderInstancesWithConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, providers)
}

// CreateProvider creates a new payment provider instance.
func (h *AdminPaymentHandler) CreateProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var req service.CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.CreateProviderInstance(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Created(c, inst)
}

// UpdateProvider updates an existing payment provider instance.
func (h *AdminPaymentHandler) UpdateProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.UpdateProviderInstance(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, inst)
}

// DeleteProvider deletes a payment provider instance.
func (h *AdminPaymentHandler) DeleteProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configService.DeleteProviderInstance(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, gin.H{"message": "deleted"})
}
