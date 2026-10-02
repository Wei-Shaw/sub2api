package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

// SePayNotify handles SePay payment notifications.
// POST /api/v1/payment/webhook/sepay
func (h *PaymentWebhookHandler) SePayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeSePay)
}

// NowPaymentsNotify handles NOWPayments IPN callbacks.
// POST /api/v1/payment/webhook/nowpayments
func (h *PaymentWebhookHandler) NowPaymentsNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeNowPayments)
}

// GPMPayNotify handles GPM Pay bank-transfer webhooks.
// POST /api/v1/payment/webhook/gpmpay
func (h *PaymentWebhookHandler) GPMPayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeGPMPay)
}

// handleNotify is the shared logic for provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	var rawBody string
	if c.Request.Method == http.MethodGet {
		// GET callbacks carry their params in the query string.
		rawBody = c.Request.URL.RawQuery
	} else {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize))
		if err != nil {
			slog.Error("[Payment Webhook] failed to read body", "provider", providerKey, "error", err)
			c.String(http.StatusBadRequest, "failed to read body")
			return
		}
		rawBody = string(body)
	}

	// Extract out_trade_no to look up the order's specific provider instance.
	// This is needed when several instances of the same gateway are configured.
	outTradeNo := extractOutTradeNo(rawBody)

	providers, err := h.paymentService.GetWebhookProviders(c.Request.Context(), providerKey, outTradeNo)
	if err != nil {
		// 只有确定性的「找不到/无法区分实例」才 ACK；DB 等瞬时错误回 5xx 让服务商重试。
		if errors.Is(err, payment.ErrProviderNotFound) || errors.Is(err, service.ErrWebhookProviderAmbiguous) {
			slog.Warn("[Payment Webhook] provider not found", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
			writeSuccessResponse(c, providerKey)
			return
		}
		slog.Error("[Payment Webhook] provider lookup failed", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
		c.String(http.StatusInternalServerError, "provider lookup failed")
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

	// nil notification means an irrelevant event (e.g. the order is not settled yet); return success.
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
// SePay posts JSON, but form-encoded bodies are accepted too so a gateway that
// switches encodings does not silently lose instance pinning.
func extractOutTradeNo(rawBody string) string {
	trimmed := strings.TrimSpace(rawBody)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") {
		// order_invoice_number is SePay's name for it, order_id is NOWPayments'.
		var payload struct {
			OrderInvoiceNumber string          `json:"order_invoice_number"`
			OrderID            string          `json:"order_id"`
			Order              json.RawMessage `json:"order"`
			Content            string          `json:"content"`
			Data               struct {
				OrderInvoiceNumber string `json:"order_invoice_number"`
				OrderID            string `json:"order_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return ""
		}
		// SePay 真实 IPN 把订单嵌在 order 下；order.order_id 是 SePay 自己的单号，不能用。
		var nestedOrder struct {
			OrderInvoiceNumber string `json:"order_invoice_number"`
		}
		_ = json.Unmarshal(payload.Order, &nestedOrder)
		for _, candidate := range []string{
			payload.OrderInvoiceNumber,
			nestedOrder.OrderInvoiceNumber,
			payload.OrderID,
			payload.Data.OrderInvoiceNumber,
			payload.Data.OrderID,
			// GPM Pay only echoes the order code inside the transfer memo.
			payment.OutTradeNoFromTransferContent(payload.Content),
		} {
			if v := strings.TrimSpace(candidate); v != "" {
				return v
			}
		}
		return ""
	}
	values, err := url.ParseQuery(trimmed)
	if err != nil {
		return ""
	}
	if v := strings.TrimSpace(values.Get("order_invoice_number")); v != "" {
		return v
	}
	return strings.TrimSpace(values.Get("order_id"))
}

func verifyNotificationWithProviders(ctx context.Context, providers []payment.Provider, rawBody string, headers map[string]string) (string, *payment.PaymentNotification, error) {
	var lastErr error
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		notification, err := provider.VerifyNotification(ctx, rawBody, headers)
		if err != nil {
			lastErr = err
			continue
		}
		return provider.ProviderKey(), notification, nil
	}
	if lastErr != nil {
		return "", nil, lastErr
	}
	return "", nil, fmt.Errorf("no webhook provider could verify notification")
}

// writeSuccessResponse 返回服务商要求的成功响应。SePay 接受纯文本 "success"。
func writeSuccessResponse(c *gin.Context, providerKey string) {
	_ = providerKey
	c.String(http.StatusOK, "success")
}
