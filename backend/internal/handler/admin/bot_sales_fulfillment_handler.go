package admin

import (
	"context"
	"encoding/json"
	"io"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const botSalesFulfillmentIdempotencyScope = "admin.bot_sales.token_fulfillments"

type BotSalesFulfillmentHandler struct {
	fulfillmentService *service.BotSalesFulfillmentService
}

func NewBotSalesFulfillmentHandler(fulfillmentService *service.BotSalesFulfillmentService) *BotSalesFulfillmentHandler {
	return &BotSalesFulfillmentHandler{fulfillmentService: fulfillmentService}
}

func (h *BotSalesFulfillmentHandler) Create(c *gin.Context) {
	if h == nil || h.fulfillmentService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_UNAVAILABLE", "bot-sales fulfillment service is not available"))
		return
	}

	req, err := decodeBotSalesFulfillmentRequest(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := executeAdminIdempotent(c, botSalesFulfillmentIdempotencyScope, req, 24*time.Hour, func(ctx context.Context) (any, error) {
		return h.fulfillmentService.Fulfill(ctx, req)
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	c.JSON(200, result.Data)
}

func decodeBotSalesFulfillmentRequest(c *gin.Context) (service.BotSalesTokenFulfillmentRequest, error) {
	var req service.BotSalesTokenFulfillmentRequest
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return req, infraerrors.BadRequest("INVALID_REQUEST", "request body is required")
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return req, infraerrors.BadRequest("INVALID_REQUEST", "failed to read request body")
	}
	if len(body) == 0 {
		return req, infraerrors.BadRequest("INVALID_REQUEST", "request body is required")
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return req, infraerrors.BadRequest("INVALID_JSON", "invalid JSON request body")
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, infraerrors.BadRequest("INVALID_REQUEST", "invalid bot-sales fulfillment payload")
	}
	normalizeBotSalesFulfillmentAliases(raw, &req)
	req.RawPayload = raw
	return req, nil
}

func normalizeBotSalesFulfillmentAliases(raw map[string]any, req *service.BotSalesTokenFulfillmentRequest) {
	if raw == nil || req == nil {
		return
	}
	if req.PlanID == 0 {
		if v, ok := raw["planId"]; ok {
			req.PlanID = int64FromJSONNumber(v)
		}
	}
	if req.BalancePackageCode == "" {
		if v, ok := raw["balancePackageCode"]; ok {
			if s, ok := v.(string); ok {
				req.BalancePackageCode = s
			}
		}
	}
	if req.Quantity == 0 {
		if v, ok := raw["quantity"]; ok {
			req.Quantity = intFromJSONNumber(v)
		}
	}
	if req.ExternalOrderID == "" {
		if v, ok := raw["external_order_code"]; ok {
			if s, ok := v.(string); ok {
				req.ExternalOrderID = s
			}
		}
	}
	if req.DeviceCode == "" {
		if v, ok := raw["deviceCode"]; ok {
			if s, ok := v.(string); ok {
				req.DeviceCode = s
			}
		}
	}
	if req.ExternalOrderItemID == "" {
		if s := stringFromJSON(raw["externalOrderItemId"]); s != "" {
			req.ExternalOrderItemID = s
		}
	}
	if req.ExternalPaymentID == "" {
		if s := stringFromJSON(raw["externalPaymentId"]); s != "" {
			req.ExternalPaymentID = s
		}
	}
	if req.PaymentAmount == 0 {
		if v, ok := raw["paymentAmount"]; ok {
			req.PaymentAmount = float64FromJSONNumber(v)
		}
	}
	if req.PaymentCurrency == "" {
		if s := stringFromJSON(raw["paymentCurrency"]); s != "" {
			req.PaymentCurrency = s
		}
	}
	if req.PaymentProvider == "" {
		if s := stringFromJSON(raw["paymentProvider"]); s != "" {
			req.PaymentProvider = s
		}
	}
	if req.PaymentProviderTxnID == "" {
		if s := stringFromJSON(raw["paymentProviderTxnId"]); s != "" {
			req.PaymentProviderTxnID = s
		}
	}
	if req.PaidAt == nil {
		if s := stringFromJSON(raw["paidAt"]); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				req.PaidAt = &t
			}
		}
	}
}

func stringFromJSON(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func float64FromJSONNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

func intFromJSONNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func int64FromJSONNumber(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
