package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentWebhookHandler handles payment provider webhook callbacks.
type PaymentWebhookHandler struct {
	paymentService *service.PaymentService
	registry       *payment.Registry
}

// maxWebhookBodySize is the maximum allowed webhook request body size (1 MB).
const maxWebhookBodySize = 1 << 20

// webhookLogTruncateLen is the maximum length of raw body logged on verify failure.
const webhookLogTruncateLen = 200

// NewPaymentWebhookHandler creates a new PaymentWebhookHandler.
func NewPaymentWebhookHandler(paymentService *service.PaymentService, registry *payment.Registry) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentService: paymentService,
		registry:       registry,
	}
}

// SePayNotify handles SePay bank-transfer notifications.
// POST /api/v1/payment/webhook/sepay
func (h *PaymentWebhookHandler) SePayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeSePay)
}

// NOWPaymentsNotify handles NOWPayments IPN callbacks.
// POST /api/v1/payment/webhook/nowpayments
func (h *PaymentWebhookHandler) NOWPaymentsNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeNowPayments)
}

// handleNotify is the shared logic for all provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize))
	if err != nil {
		slog.Error("[Payment Webhook] failed to read body", "provider", providerKey, "error", err)
		c.String(http.StatusBadRequest, "failed to read body")
		return
	}
	rawBody := string(body)

	// Extract out_trade_no to look up the order's specific provider instance.
	// This is needed when multiple instances of the same provider exist.
	outTradeNo := extractOutTradeNo(rawBody, providerKey)

	providers, err := h.paymentService.GetWebhookProviders(c.Request.Context(), providerKey, outTradeNo)
	if err != nil {
		slog.Warn("[Payment Webhook] provider not found", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
		writeSuccessResponse(c, providerKey)
		return
	}

	headers := make(map[string]string)
	for k := range c.Request.Header {
		headers[strings.ToLower(k)] = c.GetHeader(k)
	}

	resolvedProviderKey, notification, err := verifyNotificationWithProviders(c.Request.Context(), providers, rawBody, headers)
	if err != nil {
		truncatedBody := rawBody
		if len(truncatedBody) > webhookLogTruncateLen {
			truncatedBody = truncatedBody[:webhookLogTruncateLen] + "...(truncated)"
		}
		slog.Error("[Payment Webhook] verify failed", "provider", providerKey, "error", err, "method", c.Request.Method, "bodyLen", len(rawBody))
		slog.Debug("[Payment Webhook] verify failed body", "provider", providerKey, "rawBody", truncatedBody)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}

	// A nil notification means an event we have nothing to do with — an outgoing
	// bank transfer, or a NOWPayments status still in flight. Ack it.
	if notification == nil {
		writeSuccessResponse(c, resolvedProviderKey)
		return
	}

	if err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, resolvedProviderKey); err != nil {
		// Unknown order: ack with 2xx so the provider stops retrying. This
		// guards against foreign environments whose webhook endpoints are
		// (mis)configured to point at us — without a 2xx, the provider will
		// retry for days and spam our error logs. We still emit a WARN so the
		// event is discoverable in logs.
		if errors.Is(err, service.ErrOrderNotFound) {
			slog.Warn("[Payment Webhook] unknown order, acking to stop retries",
				"provider", resolvedProviderKey,
				"outTradeNo", notification.OrderID,
				"tradeNo", notification.TradeNo,
			)
			writeSuccessResponse(c, resolvedProviderKey)
			return
		}
		slog.Error("[Payment Webhook] handle notification failed", "provider", resolvedProviderKey, "error", err)
		c.String(http.StatusInternalServerError, "handle failed")
		return
	}

	writeSuccessResponse(c, resolvedProviderKey)
}

// extractOutTradeNo parses the webhook body to find the out_trade_no.
// This allows looking up the correct provider instance before verification.
func extractOutTradeNo(rawBody, providerKey string) string {
	switch providerKey {
	case payment.TypeSePay:
		var payload struct {
			Code        *string `json:"code"`
			Content     string  `json:"content"`
			Description string  `json:"description"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
			return ""
		}
		if payload.Code != nil {
			if code := payment.ExtractOrderCode(*payload.Code); code != "" {
				return code
			}
		}
		if code := payment.ExtractOrderCode(payload.Content); code != "" {
			return code
		}
		return payment.ExtractOrderCode(payload.Description)
	case payment.TypeNowPayments:
		var payload struct {
			OrderID string `json:"order_id"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err != nil {
			return ""
		}
		return strings.TrimSpace(payload.OrderID)
	}
	return ""
}

func verifyNotificationWithProviders(ctx context.Context, providers []payment.Provider, rawBody string, headers map[string]string) (string, *payment.PaymentNotification, error) {
	var lastErr error
	for _, prov := range providers {
		if prov == nil {
			continue
		}
		notification, err := prov.VerifyNotification(ctx, rawBody, headers)
		if err != nil {
			lastErr = err
			continue
		}
		return prov.ProviderKey(), notification, nil
	}
	if lastErr != nil {
		return "", nil, lastErr
	}
	return "", nil, fmt.Errorf("no webhook provider could verify notification")
}

// sepaySuccessResponse is the JSON body SePay expects so it marks the callback
// as delivered instead of retrying it.
type sepaySuccessResponse struct {
	Success bool `json:"success"`
}

// writeSuccessResponse returns the acknowledgement each provider expects.
func writeSuccessResponse(c *gin.Context, providerKey string) {
	switch providerKey {
	case payment.TypeSePay:
		c.JSON(http.StatusOK, sepaySuccessResponse{Success: true})
	default:
		c.String(http.StatusOK, "")
	}
}
