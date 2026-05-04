// Package handler — payment plugin user-facing HTTP endpoints.
//
// Adapted from backend/internal/handler. Auth identity is read from the
// V4 X-Plugin-User-* request headers via pluginsdk.RequestMetadata
// instead of the host JWT middleware.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	dbent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// PaymentHandler handles user-facing payment requests.
type PaymentHandler struct {
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
}

// NewPaymentHandler creates a new PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		configService:  configService,
	}
}

// RegisterRoutes attaches the user-facing payment routes onto the
// supplied Gin router group. The plugin host calls this once during
// Init from plugin.go so route names match the manifest declarations.
func (h *PaymentHandler) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/config", h.GetPaymentConfig)
	api.GET("/checkout-info", h.GetCheckoutInfo)
	api.GET("/plans", h.GetPlans)
	api.GET("/limits", h.GetLimits)
	api.POST("/orders", h.CreateOrder)
	api.POST("/orders/verify", h.VerifyOrder)
	api.GET("/orders/my", h.GetMyOrders)
	api.GET("/orders/refund-eligible-providers", h.GetRefundEligibleProviders)
	api.GET("/orders/:id", h.GetOrder)
	api.POST("/orders/:id/cancel", h.CancelOrder)
	api.POST("/orders/:id/refund-request", h.RequestRefund)
}

// RegisterPublicRoutes attaches the anonymous public lookup endpoints
// (verify by out_trade_no, resolve by resume token).
func (h *PaymentHandler) RegisterPublicRoutes(public *gin.RouterGroup) {
	public.POST("/orders/verify", h.VerifyOrderPublic)
	public.POST("/orders/resolve", h.ResolveOrderPublicByResumeToken)
}

// GetPaymentConfig returns the payment system configuration.
func (h *PaymentHandler) GetPaymentConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// planWithPlatform mirrors the legacy /plans response shape, enriched
// with the group's platform string used by frontend color coding.
type planWithPlatform struct {
	ID            int64            `json:"id"`
	GroupID       int64            `json:"group_id"`
	GroupPlatform string           `json:"group_platform"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Price         decimal.Decimal  `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price,omitempty"`
	ValidityDays  int              `json:"validity_days"`
	ValidityUnit  string           `json:"validity_unit"`
	Features      string           `json:"features"`
	ProductName   string           `json:"product_name"`
	ForSale       bool             `json:"for_sale"`
	SortOrder     int              `json:"sort_order"`
}

// GetPlans returns subscription plans available for sale.
func (h *PaymentHandler) GetPlans(c *gin.Context) {
	plans, err := h.configService.ListPlansForSale(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
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

// GetCheckoutInfo returns all data the payment page needs in a single call.
func (h *PaymentHandler) GetCheckoutInfo(c *gin.Context) {
	ctx := c.Request.Context()

	limitsResp, err := h.configService.GetAvailableMethodLimits(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	cfg, err := h.configService.GetPaymentConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	plans, _ := h.configService.ListPlansForSale(ctx)
	groupInfo := h.configService.GetGroupInfoMap(ctx, plans)
	planList := make([]checkoutPlan, 0, len(plans))
	for _, p := range plans {
		gi := groupInfo[p.GroupID]
		planList = append(planList, checkoutPlan{
			ID: int64(p.ID), GroupID: p.GroupID,
			GroupPlatform: gi.Platform, GroupName: gi.Name,
			Name: p.Name, Description: p.Description, Price: p.Price, OriginalPrice: p.OriginalPrice,
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
	GlobalMin                 decimal.Decimal                 `json:"global_min"`
	GlobalMax                 decimal.Decimal                 `json:"global_max"`
	Plans                     []checkoutPlan                  `json:"plans"`
	BalanceDisabled           bool                            `json:"balance_disabled"`
	BalanceRechargeMultiplier decimal.Decimal                 `json:"balance_recharge_multiplier"`
	RechargeFeeRate           decimal.Decimal                 `json:"recharge_fee_rate"`
	HelpText                  string                          `json:"help_text"`
	HelpImageURL              string                          `json:"help_image_url"`
	StripePublishableKey      string                          `json:"stripe_publishable_key"`
}

type checkoutPlan struct {
	ID            int64            `json:"id"`
	GroupID       int64            `json:"group_id"`
	GroupPlatform string           `json:"group_platform"`
	GroupName     string           `json:"group_name"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Price         decimal.Decimal  `json:"price"`
	OriginalPrice *decimal.Decimal `json:"original_price,omitempty"`
	ValidityDays  int              `json:"validity_days"`
	ValidityUnit  string           `json:"validity_unit"`
	Features      []string         `json:"features"`
	ProductName   string           `json:"product_name"`
}

// parseFeatures splits a newline-separated features string into a slice.
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
func (h *PaymentHandler) GetLimits(c *gin.Context) {
	resp, err := h.configService.GetAvailableMethodLimits(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, resp)
}

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

func sanitizePaymentOrdersForResponse(orders []*dbent.PaymentOrder) []*dbent.PaymentOrder {
	if len(orders) == 0 {
		return orders
	}
	out := make([]*dbent.PaymentOrder, 0, len(orders))
	for _, order := range orders {
		out = append(out, sanitizePaymentOrderForResponse(order))
	}
	return out
}

func sanitizePaymentOrderForResponse(order *dbent.PaymentOrder) *dbent.PaymentOrder {
	if order == nil {
		return nil
	}
	cloned := *order
	cloned.ProviderSnapshot = nil
	return &cloned
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
