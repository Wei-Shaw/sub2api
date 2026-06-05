// Package handler — payment plugin webhook callback handlers.
//
// Adapted from backend/internal/handler. Webhook routes are anonymous
// (no auth) — providers POST signed bodies that the registry verifies
// before HandlePaymentNotification kicks off the fulfillment pipeline.
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

	"github.com/gin-gonic/gin"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// PaymentWebhookHandler handles payment provider webhook callbacks.
type PaymentWebhookHandler struct {
	paymentService *service.PaymentService
	registry       *payment.Registry
	// dedup guards against replay of an already-processed provider
	// notification using a Redis SETNX-backed seen-set keyed on
	// (provider, TradeNo). Constructed nil when the SDK Redis client is
	// not wired — the dedup degrades to fail-open in that case.
	dedup *webhookDedup
}

// maxWebhookBodySize is the maximum allowed webhook request body size (1 MB).
const maxWebhookBodySize = 1 << 20

// webhookLogTruncateLen is the maximum length of raw body logged on verify failure.
const webhookLogTruncateLen = 200

// NewPaymentWebhookHandler creates a new PaymentWebhookHandler. redis may
// be nil when the host has not wired a Redis proxy; the constructor still
// builds a dedup helper but Reserve becomes a fail-open no-op (see
// webhook_dedup.go).
func NewPaymentWebhookHandler(paymentService *service.PaymentService, registry *payment.Registry, redis pluginsdk.RedisClient, logger *slog.Logger) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		paymentService: paymentService,
		registry:       registry,
		dedup:          newWebhookDedup(redis, logger),
	}
}

// RegisterRoutes attaches the webhook callback endpoints onto the
// supplied router group. Webhooks accept both GET (EasyPay query-string
// callbacks) and POST so the same handler is bound twice for EasyPay.
func (h *PaymentWebhookHandler) RegisterRoutes(webhook *gin.RouterGroup) {
	webhook.GET("/easypay", h.EasyPayNotify)
	webhook.POST("/easypay", h.EasyPayNotify)
	webhook.POST("/alipay", h.AlipayNotify)
	webhook.POST("/wxpay", h.WxpayNotify)
	webhook.POST("/stripe", h.StripeWebhook)
	webhook.POST("/airwallex", h.AirwallexWebhook)
}

// EasyPayNotify handles EasyPay payment notifications.
func (h *PaymentWebhookHandler) EasyPayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeEasyPay)
}

// AlipayNotify handles Alipay payment notifications.
func (h *PaymentWebhookHandler) AlipayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeAlipay)
}

// WxpayNotify handles WeChat Pay payment notifications.
func (h *PaymentWebhookHandler) WxpayNotify(c *gin.Context) {
	h.handleNotify(c, payment.TypeWxpay)
}

// StripeWebhook handles Stripe webhook events.
func (h *PaymentWebhookHandler) StripeWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeStripe)
}

// AirwallexWebhook 处理空中云汇 Webhook 事件。
// POST /api/v1/payment/webhook/airwallex
func (h *PaymentWebhookHandler) AirwallexWebhook(c *gin.Context) {
	h.handleNotify(c, payment.TypeAirwallex)
}

// handleNotify is the shared logic for all provider webhook handlers.
func (h *PaymentWebhookHandler) handleNotify(c *gin.Context, providerKey string) {
	rawBody, ok := h.readWebhookBody(c, providerKey)
	if !ok {
		return
	}

	outTradeNo := extractOutTradeNo(rawBody, providerKey)

	providers, err := h.paymentService.GetWebhookProviders(c.Request.Context(), providerKey, outTradeNo)
	if err != nil {
		slog.Warn("[Payment Webhook] provider not found", "provider", providerKey, "outTradeNo", outTradeNo, "error", err)
		if providerKey == payment.TypeWxpay {
			c.String(http.StatusBadRequest, "verify failed")
			return
		}
		writeSuccessResponse(c, providerKey)
		return
	}

	headers := collectHeaders(c)

	resolvedProviderKey, notification, err := verifyNotificationWithProviders(c.Request.Context(), providers, rawBody, headers)
	if err != nil {
		logVerifyFailure(providerKey, rawBody, c.Request.Method, err)
		c.String(http.StatusBadRequest, "verify failed")
		return
	}
	if notification == nil {
		writeSuccessResponse(c, resolvedProviderKey)
		return
	}

	if !h.checkReplayProtection(c, resolvedProviderKey, notification) {
		return
	}

	h.dispatchNotification(c, resolvedProviderKey, notification)
}

// checkReplayProtection performs dedup reservation for the notification.
// Returns true if the notification should proceed, false if it was a
// replay (response already written).
func (h *PaymentWebhookHandler) checkReplayProtection(c *gin.Context, providerKey string, notification *payment.PaymentNotification) bool {
	err := h.dedup.Reserve(c.Request.Context(), providerKey, notification.TradeNo)
	if err == nil {
		return true
	}
	if errors.Is(err, ErrWebhookReplay) {
		slog.Info("[Payment Webhook] duplicate notification suppressed",
			"provider", providerKey,
			"outTradeNo", notification.OrderID,
			"tradeNo", notification.TradeNo,
		)
		writeSuccessResponse(c, providerKey)
		return false
	}
	slog.Warn("[Payment Webhook] dedup reserve unexpected error; continuing fail-open",
		"provider", providerKey, "error", err)
	return true
}

// dispatchNotification sends the verified notification through the
// fulfillment pipeline and writes the appropriate HTTP response.
func (h *PaymentWebhookHandler) dispatchNotification(c *gin.Context, providerKey string, notification *payment.PaymentNotification) {
	err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, providerKey)
	if err == nil {
		writeSuccessResponse(c, providerKey)
		return
	}
	if errors.Is(err, service.ErrOrderNotFound) {
		slog.Warn("[Payment Webhook] unknown order, acking to stop retries",
			"provider", providerKey,
			"outTradeNo", notification.OrderID,
			"tradeNo", notification.TradeNo,
		)
		writeSuccessResponse(c, providerKey)
		return
	}
	slog.Error("[Payment Webhook] handle notification failed", "provider", providerKey, "error", err)
	c.String(http.StatusInternalServerError, "handle failed")
}

// readWebhookBody returns the raw body of the webhook request. For GET
// callbacks (e.g. EasyPay) it pulls the URL query string; for POST it
// reads up to maxWebhookBodySize bytes. Writes a 400 response and returns
// false on read failure.
func (h *PaymentWebhookHandler) readWebhookBody(c *gin.Context, providerKey string) (string, bool) {
	if c.Request.Method == http.MethodGet {
		return c.Request.URL.RawQuery, true
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize))
	if err != nil {
		slog.Error("[Payment Webhook] failed to read body", "provider", providerKey, "error", err)
		c.String(http.StatusBadRequest, "failed to read body")
		return "", false
	}
	return string(body), true
}

// collectHeaders snapshots the inbound headers into a lowercase-keyed map
// so provider verifiers can do case-insensitive lookups.
func collectHeaders(c *gin.Context) map[string]string {
	headers := make(map[string]string, len(c.Request.Header))
	for k := range c.Request.Header {
		headers[strings.ToLower(k)] = c.GetHeader(k)
	}
	return headers
}

// logVerifyFailure emits the structured log lines for a verification failure.
func logVerifyFailure(providerKey, rawBody, method string, err error) {
	truncated := rawBody
	if len(truncated) > webhookLogTruncateLen {
		truncated = truncated[:webhookLogTruncateLen] + "...(truncated)"
	}
	slog.Error("[Payment Webhook] verify failed", "provider", providerKey, "error", err, "method", method, "bodyLen", len(rawBody))
	slog.Debug("[Payment Webhook] verify failed body", "provider", providerKey, "rawBody", truncated)
}

// extractOutTradeNo parses the webhook body to find the out_trade_no.
// This allows looking up the correct provider instance before verification.
func extractOutTradeNo(rawBody, providerKey string) string {
	switch providerKey {
	case payment.TypeEasyPay, payment.TypeAlipay:
		values, err := url.ParseQuery(rawBody)
		if err == nil {
			return values.Get("out_trade_no")
		}
	case payment.TypeAirwallex:
		var payload struct {
			Data struct {
				Object struct {
					MerchantOrderID string `json:"merchant_order_id"`
				} `json:"object"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(rawBody), &payload); err == nil {
			return strings.TrimSpace(payload.Data.Object.MerchantOrderID)
		}
	}
	return ""
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

// wxpaySuccessResponse is the JSON response expected by WeChat Pay webhook.
type wxpaySuccessResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	wxpaySuccessCode    = "SUCCESS"
	wxpaySuccessMessage = "成功"
)

// writeSuccessResponse 返回各支付服务商要求的成功响应。
// 微信支付需要 JSON {"code":"SUCCESS","message":"成功"}；
// Stripe 和空中云汇接受空 200，其它服务商接受纯文本 "success"。
func writeSuccessResponse(c *gin.Context, providerKey string) {
	switch providerKey {
	case payment.TypeWxpay:
		c.JSON(http.StatusOK, wxpaySuccessResponse{Code: wxpaySuccessCode, Message: wxpaySuccessMessage})
	case payment.TypeStripe, payment.TypeAirwallex:
		c.String(http.StatusOK, "")
	default:
		c.String(http.StatusOK, "success")
	}
}
