package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentHandler handles user-facing payment requests.
type PaymentHandler struct {
	channelService *service.ChannelService
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
}

// NewPaymentHandler creates a new PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService, channelService *service.ChannelService) *PaymentHandler {
	return &PaymentHandler{
		channelService: channelService,
		paymentService: paymentService,
		configService:  configService,
	}
}

// GetPaymentConfig returns the payment system configuration.
// GET /api/v1/payment/config
func (h *PaymentHandler) GetPaymentConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// GetPlans returns subscription plans available for sale.
// GET /api/v1/payment/plans
func (h *PaymentHandler) GetPlans(c *gin.Context) {
	plans, err := h.configService.ListPlansForSale(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// Enrich plans with group platform for frontend color coding
	type planWithPlatform struct {
		ID            int64    `json:"id"`
		GroupID       int64    `json:"group_id"`
		GroupPlatform string   `json:"group_platform"`
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		Price         float64  `json:"price"`
		OriginalPrice *float64 `json:"original_price,omitempty"`
		ValidityDays  int      `json:"validity_days"`
		ValidityUnit  string   `json:"validity_unit"`
		Features      string   `json:"features"`
		ProductName   string   `json:"product_name"`
		ForSale       bool     `json:"for_sale"`
		SortOrder     int      `json:"sort_order"`
	}
	platformMap := h.configService.GetGroupPlatformMap(c.Request.Context(), plans)
	result := make([]planWithPlatform, 0, len(plans))
	for _, p := range plans {
		result = append(result, planWithPlatform{
			ID: int64(p.ID), GroupID: p.GroupID, GroupPlatform: platformMap[p.GroupID],
			Name: p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice,
			ValidityDays: p.ValidityDays, ValidityUnit: p.ValidityUnit, Features: p.Features,
			ProductName: p.ProductName, ForSale: p.ForSale, SortOrder: p.SortOrder,
		})
	}
	response.Success(c, result)
}

// GetChannels returns enabled payment channels.
// GET /api/v1/payment/channels
func (h *PaymentHandler) GetChannels(c *gin.Context) {
	channels, _, err := h.channelService.List(c.Request.Context(), pagination.PaginationParams{Page: 1, PageSize: 1000}, "active", "")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, channels)
}

// GetCheckoutInfo returns all data the payment page needs in a single call:
// payment methods with limits, subscription plans, and configuration.
// GET /api/v1/payment/checkout-info
func (h *PaymentHandler) GetCheckoutInfo(c *gin.Context) {
	ctx := c.Request.Context()

	// Fetch limits (methods + global range)
	limitsResp, err := h.configService.GetAvailableMethodLimits(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Fetch payment config
	cfg, err := h.configService.GetPaymentConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Fetch plans with group info
	plans, _ := h.configService.ListPlansForSale(ctx)
	groupInfo := h.configService.GetGroupInfoMap(ctx, plans)
	planList := make([]checkoutPlan, 0, len(plans))
	for _, p := range plans {
		gi := groupInfo[p.GroupID]
		planList = append(planList, checkoutPlan{
			ID: int64(p.ID), GroupID: p.GroupID,
			GroupPlatform: gi.Platform, GroupName: gi.Name,
			RateMultiplier: gi.RateMultiplier, DailyLimitUSD: gi.DailyLimitUSD,
			WeeklyLimitUSD: gi.WeeklyLimitUSD, MonthlyLimitUSD: gi.MonthlyLimitUSD,
			ModelScopes: gi.ModelScopes,
			Name:        p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice,
			ValidityDays: p.ValidityDays, ValidityUnit: p.ValidityUnit, Features: parseFeatures(p.Features),
			ProductName: p.ProductName,
		})
	}

	response.Success(c, checkoutInfoResponse{
		Methods:                   limitsResp.Methods,
		GlobalMin:                 limitsResp.GlobalMin,
		GlobalMax:                 limitsResp.GlobalMax,
		Plans:                     planList,
		BalanceDisabled:           cfg.BalanceDisabled,
		BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier,
		RechargeFeeRate:           cfg.RechargeFeeRate,
		HelpText:                  cfg.HelpText,
		HelpImageURL:              cfg.HelpImageURL,
		StripePublishableKey:      cfg.StripePublishableKey,
	})
}

type checkoutInfoResponse struct {
	Methods                   map[string]service.MethodLimits `json:"methods"`
	GlobalMin                 float64                         `json:"global_min"`
	GlobalMax                 float64                         `json:"global_max"`
	Plans                     []checkoutPlan                  `json:"plans"`
	BalanceDisabled           bool                            `json:"balance_disabled"`
	BalanceRechargeMultiplier float64                         `json:"balance_recharge_multiplier"`
	RechargeFeeRate           float64                         `json:"recharge_fee_rate"`
	HelpText                  string                          `json:"help_text"`
	HelpImageURL              string                          `json:"help_image_url"`
	StripePublishableKey      string                          `json:"stripe_publishable_key"`
}

type checkoutPlan struct {
	ID              int64    `json:"id"`
	GroupID         int64    `json:"group_id"`
	GroupPlatform   string   `json:"group_platform"`
	GroupName       string   `json:"group_name"`
	RateMultiplier  float64  `json:"rate_multiplier"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
	ModelScopes     []string `json:"supported_model_scopes"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Price           float64  `json:"price"`
	OriginalPrice   *float64 `json:"original_price,omitempty"`
	ValidityDays    int      `json:"validity_days"`
	ValidityUnit    string   `json:"validity_unit"`
	Features        []string `json:"features"`
	ProductName     string   `json:"product_name"`
}

// parseFeatures splits a newline-separated features string into a string slice.
func parseFeatures(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

// GetLimits returns per-payment-type limits derived from enabled provider instances.
// GET /api/v1/payment/limits
func (h *PaymentHandler) GetLimits(c *gin.Context) {
	resp, err := h.configService.GetAvailableMethodLimits(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, resp)
}

// CreateOrderRequest is the request body for creating a payment order.
type CreateOrderRequest struct {
	Amount            float64 `json:"amount"`
	PaymentType       string  `json:"payment_type" binding:"required"`
	OpenID            string  `json:"openid"`
	WechatResumeToken string  `json:"wechat_resume_token"`
	ReturnURL         string  `json:"return_url"`
	PaymentSource     string  `json:"payment_source"`
	OrderType         string  `json:"order_type"`
	PlanID            int64   `json:"plan_id"`
	// IsMobile lets the frontend declare its mobile status directly. When
	// nil we fall back to User-Agent heuristics (which miss iPadOS / some
	// embedded browsers that strip the "Mobile" keyword).
	IsMobile *bool `json:"is_mobile,omitempty"`
}

// CreateOrder creates a new payment order.
// POST /api/v1/payment/orders
func (h *PaymentHandler) CreateOrder(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.WechatResumeToken) != "" {
		claims, err := h.paymentService.ParseWeChatPaymentResumeToken(req.WechatResumeToken)
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
		UserID:          subject.UserID,
		Amount:          req.Amount,
		PaymentType:     req.PaymentType,
		OpenID:          req.OpenID,
		ClientIP:        c.ClientIP(),
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
		amount, err := strconv.ParseFloat(strings.TrimSpace(claims.Amount), 64)
		if err != nil || amount <= 0 {
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
// GET /api/v1/payment/orders/my
func (h *PaymentHandler) GetMyOrders(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}

	page, pageSize := response.ParsePagination(c)
	orders, total, err := h.paymentService.GetUserOrders(c.Request.Context(), subject.UserID, service.OrderListParams{
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
// GET /api/v1/payment/orders/:id
func (h *PaymentHandler) GetOrder(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid order ID")
		return
	}

	order, err := h.paymentService.GetOrder(c.Request.Context(), orderID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sanitizePaymentOrderForResponse(order))
}

// CancelOrder cancels a pending order for the authenticated user.
// POST /api/v1/payment/orders/:id/cancel
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}

	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid order ID")
		return
	}

	msg, err := h.paymentService.CancelOrder(c.Request.Context(), orderID, subject.UserID)
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
// POST /api/v1/payment/orders/:id/refund-request
func (h *PaymentHandler) RequestRefund(c *gin.Context) {
	subject, ok := requireAuth(c)
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

	if err := h.paymentService.RequestRefund(c.Request.Context(), orderID, subject.UserID, req.Reason); err != nil {
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

type ResolveOrderByResumeTokenRequest struct {
	ResumeToken string `json:"resume_token" binding:"required"`
}

// VerifyOrder actively queries the upstream payment provider to check
// if payment was made, and processes it if so.
// POST /api/v1/payment/orders/verify
func (h *PaymentHandler) VerifyOrder(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}

	var req VerifyOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	order, err := h.paymentService.VerifyOrderByOutTradeNo(c.Request.Context(), req.OutTradeNo, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, sanitizePaymentOrderForResponse(order))
}

// PublicOrderResult is the limited order info returned by the public verify endpoint.
// No user details are exposed — only payment status information.
type PublicOrderResult struct {
	ID                  int64      `json:"id"`
	OutTradeNo          string     `json:"out_trade_no"`
	Amount              float64    `json:"amount"`
	PayAmount           float64    `json:"pay_amount"`
	FeeRate             float64    `json:"fee_rate"`
	Currency            string     `json:"currency"`
	PaymentType         string     `json:"payment_type"`
	OrderType           string     `json:"order_type"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	RefundAmount        float64    `json:"refund_amount"`
	RefundReason        *string    `json:"refund_reason,omitempty"`
	RefundRequestedAt   *time.Time `json:"refund_requested_at,omitempty"`
	RefundRequestedBy   *string    `json:"refund_requested_by,omitempty"`
	RefundRequestReason *string    `json:"refund_request_reason,omitempty"`
	PlanID              *int64     `json:"plan_id,omitempty"`
}

func buildPublicOrderResult(order *dbent.PaymentOrder) PublicOrderResult {
	return PublicOrderResult{
		ID:                  order.ID,
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

// VerifyOrderPublic keeps the legacy anonymous out_trade_no lookup available as
// a compatibility path for older result pages and staggered deploys.
// POST /api/v1/payment/public/orders/verify
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
// POST /api/v1/payment/public/orders/resolve
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

// requireAuth extracts the authenticated subject from the context.
// Returns the subject and true on success; on failure it writes an Unauthorized response and returns false.
func requireAuth(c *gin.Context) (middleware2.AuthSubject, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return middleware2.AuthSubject{}, false
	}
	return subject, true
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
	ID                  int64      `json:"id"`
	UserID              int64      `json:"user_id"`
	Amount              float64    `json:"amount"`
	PayAmount           float64    `json:"pay_amount"`
	FeeRate             float64    `json:"fee_rate"`
	Currency            string     `json:"currency"`
	PaymentType         string     `json:"payment_type"`
	OutTradeNo          string     `json:"out_trade_no"`
	Status              string     `json:"status"`
	OrderType           string     `json:"order_type"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	RefundAmount        float64    `json:"refund_amount"`
	RefundReason        *string    `json:"refund_reason,omitempty"`
	RefundRequestedAt   *time.Time `json:"refund_requested_at,omitempty"`
	RefundRequestedBy   *string    `json:"refund_requested_by,omitempty"`
	RefundRequestReason *string    `json:"refund_request_reason,omitempty"`
	PlanID              *int64     `json:"plan_id,omitempty"`
	ProviderInstanceID  *string    `json:"provider_instance_id,omitempty"`
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
