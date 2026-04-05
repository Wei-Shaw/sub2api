package handler

import (
	"io"
	"log"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentWebhookHandler handles payment provider webhook callbacks.
type PaymentWebhookHandler struct {
	paymentService *service.PaymentService
	registry       *payment.Registry
}

// NewPaymentWebhookHandler creates a new PaymentWebhookHandler.
func NewPaymentWebhookHandler(paymentService *service.PaymentService, registry *payment.Registry) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentService: paymentService,
		registry:       registry,
	}
}

// EasyPayNotify handles EasyPay payment notifications.
// POST /api/v1/payment/webhook/easypay
func (h *PaymentWebhookHandler) EasyPayNotify(c *gin.Context) {
	h.handleNotify(c, "easypay")
}

// AlipayNotify handles Alipay payment notifications.
// POST /api/v1/payment/webhook/alipay
func (h *PaymentWebhookHandler) AlipayNotify(c *gin.Context) {
	h.handleNotify(c, "alipay")
}

// WxpayNotify handles WeChat Pay payment notifications.
// POST /api/v1/payment/webhook/wxpay
func (h *PaymentWebhookHandler) WxpayNotify(c *gin.Context) {
	h.handleNotify(c, "wxpay")
}

// StripeWebhook handles Stripe webhook events.
// POST /api/v1/payment/webhook/stripe
func (h *PaymentWebhookHandler) StripeWebhook(c *gin.Context) {
	h.handleNotify(c, "stripe")
}

// handleNotify is the shared logic for all provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
	if err != nil {
		log.Printf("[Payment Webhook] failed to read body for %s: %v", providerKey, err)
		c.String(http.StatusBadRequest, "failed to read body")
		return
	}

	provider, err := h.registry.GetProviderByKey(providerKey)
	if err != nil {
		log.Printf("[Payment Webhook] provider %s not registered: %v", providerKey, err)
		c.String(http.StatusOK, successResponse(providerKey))
		return
	}

	headers := make(map[string]string)
	for k := range c.Request.Header {
		headers[k] = c.GetHeader(k)
	}

	notification, err := provider.VerifyNotification(c.Request.Context(), string(body), headers)
	if err != nil {
		log.Printf("[Payment Webhook] %s verify failed: %v", providerKey, err)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}

	// nil notification means irrelevant event (e.g. Stripe non-payment event); return success.
	if notification == nil {
		c.String(http.StatusOK, successResponse(providerKey))
		return
	}

	if err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, providerKey); err != nil {
		log.Printf("[Payment Webhook] %s handle notification failed: %v", providerKey, err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	c.String(http.StatusOK, successResponse(providerKey))
}

// successResponse returns the provider-specific success response string.
func successResponse(providerKey string) string {
	switch providerKey {
	case "stripe":
		return ""
	default:
		return "success"
	}
}
