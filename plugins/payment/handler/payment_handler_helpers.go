package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	dbent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// requireUserID extracts the authenticated user id from V4 X-Plugin-User-*
// headers via the SDK helper. Writes a 401 response and returns false when
// the request is anonymous.
func requireUserID(c *gin.Context) (int64, bool) {
	meta := pluginsdk.RequestMetadata(c.Request)
	if meta.UserID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "User not authenticated"})
		return 0, false
	}
	return meta.UserID, true
}

// clientIP returns the SDK-injected client IP, falling back to gin's
// default resolution if the host did not set the header.
func clientIP(c *gin.Context) string {
	meta := pluginsdk.RequestMetadata(c.Request)
	if meta.ClientIP != "" {
		return meta.ClientIP
	}
	return c.ClientIP()
}

// isMobile detects mobile user agents.
func isMobile(c *gin.Context) bool {
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	for _, kw := range []string{"mobile", "android", "iphone", "ipad", "ipod"} {
		if strings.Contains(ua, kw) {
			return true
		}
	}
	return false
}

type PaymentOrderResult struct {
	ID                  int             `json:"id"`
	UserID              int64           `json:"user_id"`
	Amount              decimal.Decimal `json:"amount"`
	PayAmount           decimal.Decimal `json:"pay_amount"`
	FeeRate             decimal.Decimal `json:"fee_rate"`
	Currency            string          `json:"currency"`
	PaymentType         string          `json:"payment_type"`
	OutTradeNo          string          `json:"out_trade_no"`
	Status              string          `json:"status"`
	OrderType           string          `json:"order_type"`
	CreatedAt           time.Time       `json:"created_at"`
	ExpiresAt           time.Time       `json:"expires_at"`
	PaidAt              *time.Time      `json:"paid_at,omitempty"`
	CompletedAt         *time.Time      `json:"completed_at,omitempty"`
	RefundAmount        decimal.Decimal `json:"refund_amount"`
	RefundReason        *string         `json:"refund_reason,omitempty"`
	RefundRequestedAt   *time.Time      `json:"refund_requested_at,omitempty"`
	RefundRequestedBy   *string         `json:"refund_requested_by,omitempty"`
	RefundRequestReason *string         `json:"refund_request_reason,omitempty"`
	PlanID              *int64          `json:"plan_id,omitempty"`
	ProviderInstanceID  *string         `json:"provider_instance_id,omitempty"`
}

func sanitizePaymentOrdersForResponse(orders []*dbent.PaymentOrder) []PaymentOrderResult {
	out := make([]PaymentOrderResult, 0, len(orders))
	for _, order := range orders {
		if item := sanitizePaymentOrderForResponse(order); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

func sanitizePaymentOrderForResponse(order *dbent.PaymentOrder) *PaymentOrderResult {
	if order == nil {
		return nil
	}
	return &PaymentOrderResult{
		ID:                  order.ID,
		UserID:              order.UserID,
		Amount:              order.Amount,
		PayAmount:           order.PayAmount,
		FeeRate:             order.FeeRate,
		Currency:            service.PaymentOrderCurrency(order),
		PaymentType:         order.PaymentType,
		OutTradeNo:          order.OutTradeNo,
		Status:              order.Status,
		OrderType:           order.OrderType,
		CreatedAt:           order.CreatedAt,
		ExpiresAt:           order.ExpiresAt,
		PaidAt:              order.PaidAt,
		CompletedAt:         order.CompletedAt,
		RefundAmount:        order.RefundAmount,
		RefundReason:        order.RefundReason,
		RefundRequestedAt:   order.RefundRequestedAt,
		RefundRequestedBy:   order.RefundRequestedBy,
		RefundRequestReason: order.RefundRequestReason,
		PlanID:              order.PlanID,
		ProviderInstanceID:  order.ProviderInstanceID,
	}
}

func isWeChatBrowser(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("User-Agent")), "micromessenger")
}

// parsePagination reads page / page_size from the query string and clamps
// them to safe defaults. Mirrors backend/internal/pkg/response.ParsePagination
// without dragging the host package into the plugin.
func parsePagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v >= 1 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v >= 1 {
		if v > 100 {
			v = 100
		}
		pageSize = v
	}
	return page, pageSize
}
