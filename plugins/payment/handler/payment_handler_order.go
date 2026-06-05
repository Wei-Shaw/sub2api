package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	dbent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// CreateOrderRequest is the request body for creating a payment order.
//
// Amount accepts both JSON numbers and quoted decimal strings; binding via
// shopspring/decimal preserves cent precision through the request boundary
// so the validator and pricing layer never see a float64.
type CreateOrderRequest struct {
	Amount            decimal.Decimal `json:"amount"`
	PaymentType       string          `json:"payment_type" binding:"required"`
	OpenID            string          `json:"openid"`
	WechatResumeToken string          `json:"wechat_resume_token"`
	ReturnURL         string          `json:"return_url"`
	PaymentSource     string          `json:"payment_source"`
	OrderType         string          `json:"order_type"`
	PlanID            int64           `json:"plan_id"`
	IsMobile          *bool           `json:"is_mobile,omitempty"`
}

// CreateOrder creates a new payment order for the authenticated user.
func (h *PaymentHandler) CreateOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.WechatResumeToken) != "" {
		claims, err := h.paymentService.ParseWeChatPaymentResumeToken(c.Request.Context(), req.WechatResumeToken)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if err := applyWeChatPaymentResumeClaims(&req, claims); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	mobile := isMobile(c)
	if req.IsMobile != nil {
		mobile = *req.IsMobile
	}
	result, err := h.paymentService.CreateOrder(c.Request.Context(), service.CreateOrderRequest{
		UserID:          userID,
		Amount:          req.Amount,
		PaymentType:     req.PaymentType,
		OpenID:          req.OpenID,
		ClientIP:        clientIP(c),
		IsMobile:        mobile,
		IsWeChatBrowser: isWeChatBrowser(c),
		SrcHost:         c.Request.Host,
		SrcURL:          c.Request.Referer(),
		ReturnURL:       req.ReturnURL,
		PaymentSource:   req.PaymentSource,
		OrderType:       req.OrderType,
		PlanID:          req.PlanID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func applyWeChatPaymentResumeClaims(req *CreateOrderRequest, claims *service.WeChatPaymentResumeClaims) error {
	if req == nil || claims == nil {
		return infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume context is missing")
	}
	openid := strings.TrimSpace(claims.OpenID)
	if openid == "" {
		return infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token missing openid")
	}

	paymentType := service.NormalizeVisibleMethod(claims.PaymentType)
	if paymentType == "" {
		paymentType = payment.TypeWxpay
	}
	if req.PaymentType != "" {
		requestPaymentType := service.NormalizeVisibleMethod(req.PaymentType)
		if requestPaymentType != "" && requestPaymentType != paymentType {
			return infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", "wechat payment resume token payment type mismatch")
		}
	}
	req.PaymentType = paymentType
	req.OpenID = openid

	if strings.TrimSpace(claims.Amount) != "" {
		amount, err := decimal.NewFromString(strings.TrimSpace(claims.Amount))
		if err != nil || !amount.IsPositive() {
			return infraerrors.BadRequest("INVALID_WECHAT_PAYMENT_RESUME_TOKEN", fmt.Sprintf("invalid resume amount: %s", claims.Amount))
		}
		req.Amount = amount
	}
	if claims.OrderType != "" {
		req.OrderType = claims.OrderType
	}
	if claims.PlanID > 0 {
		req.PlanID = claims.PlanID
	}
	return nil
}

// GetMyOrders returns the authenticated user's orders.
func (h *PaymentHandler) GetMyOrders(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	page, pageSize := parsePagination(c)
	orders, total, err := h.paymentService.GetUserOrders(c.Request.Context(), userID, service.OrderListParams{
		Page:        page,
		PageSize:    pageSize,
		Status:      c.Query("status"),
		OrderType:   c.Query("order_type"),
		PaymentType: c.Query("payment_type"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, sanitizePaymentOrdersForResponse(orders), int64(total), page, pageSize)
}

// GetOrder returns a single order for the authenticated user.
func (h *PaymentHandler) GetOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid order ID")
		return
	}

	order, err := h.paymentService.GetOrder(c.Request.Context(), orderID, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sanitizePaymentOrderForResponse(order))
}

// CancelOrder cancels a pending order for the authenticated user.
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid order ID")
		return
	}

	msg, err := h.paymentService.CancelOrder(c.Request.Context(), orderID, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": msg})
}

// RefundRequestBody is the request body for requesting a refund.
type RefundRequestBody struct {
	Reason string `json:"reason"`
}

// RequestRefund submits a refund request for a completed order.
func (h *PaymentHandler) RequestRefund(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid order ID")
		return
	}

	var req RefundRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := h.paymentService.RequestRefund(c.Request.Context(), orderID, userID, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "refund requested"})
}

// GetRefundEligibleProviders returns provider instance IDs that allow user refund.
func (h *PaymentHandler) GetRefundEligibleProviders(c *gin.Context) {
	ids, err := h.configService.GetUserRefundEligibleInstanceIDs(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"provider_instance_ids": ids})
}

// VerifyOrderRequest is the request body for verifying a payment order.
type VerifyOrderRequest struct {
	OutTradeNo string `json:"out_trade_no" binding:"required"`
}

// ResolveOrderByResumeTokenRequest is the request body for resume-token resolution.
type ResolveOrderByResumeTokenRequest struct {
	ResumeToken string `json:"resume_token" binding:"required"`
}

// VerifyOrder actively queries the upstream provider for the order's pay state.
func (h *PaymentHandler) VerifyOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var req VerifyOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	order, err := h.paymentService.VerifyOrderByOutTradeNo(c.Request.Context(), req.OutTradeNo, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sanitizePaymentOrderForResponse(order))
}

// PublicOrderResult is the limited order info returned by the public verify endpoint.
type PublicOrderResult struct {
	ID                  int64           `json:"id"`
	OutTradeNo          string          `json:"out_trade_no"`
	Amount              decimal.Decimal `json:"amount"`
	PayAmount           decimal.Decimal `json:"pay_amount"`
	FeeRate             decimal.Decimal `json:"fee_rate"`
	Currency            string          `json:"currency"`
	PaymentType         string          `json:"payment_type"`
	OrderType           string          `json:"order_type"`
	Status              string          `json:"status"`
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
}

func buildPublicOrderResult(order *dbent.PaymentOrder) PublicOrderResult {
	return PublicOrderResult{
		ID:                  int64(order.ID),
		OutTradeNo:          order.OutTradeNo,
		Amount:              order.Amount,
		PayAmount:           order.PayAmount,
		FeeRate:             order.FeeRate,
		Currency:            service.PaymentOrderCurrency(order),
		PaymentType:         order.PaymentType,
		OrderType:           order.OrderType,
		Status:              order.Status,
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
	}
}

// VerifyOrderPublic keeps the legacy anonymous out_trade_no lookup available.
func (h *PaymentHandler) VerifyOrderPublic(c *gin.Context) {
	var req VerifyOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	order, err := h.paymentService.VerifyOrderPublic(c.Request.Context(), req.OutTradeNo)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, buildPublicOrderResult(order))
}

// ResolveOrderPublicByResumeToken resolves a payment order from a signed resume token.
func (h *PaymentHandler) ResolveOrderPublicByResumeToken(c *gin.Context) {
	var req ResolveOrderByResumeTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	order, err := h.paymentService.GetPublicOrderByResumeToken(c.Request.Context(), req.ResumeToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, buildPublicOrderResult(order))
}
