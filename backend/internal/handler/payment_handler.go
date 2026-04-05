package handler

import (
	"strconv"
	"strings"

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
	response.Success(c, plans)
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

// GetLimits returns per-payment-type limits.
// GET /api/v1/payment/limits
func (h *PaymentHandler) GetLimits(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	limits, err := h.configService.GetMethodLimits(c.Request.Context(), cfg.EnabledTypes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, limits)
}

// CreateOrderRequest is the request body for creating a payment order.
type CreateOrderRequest struct {
	Amount      float64 `json:"amount"`
	PaymentType string  `json:"paymentType" binding:"required"`
	OrderType   string  `json:"orderType"`
	PlanID      int64   `json:"planId"`
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

	result, err := h.paymentService.CreateOrder(c.Request.Context(), service.CreateOrderRequest{
		UserID:      subject.UserID,
		Amount:      req.Amount,
		PaymentType: req.PaymentType,
		ClientIP:    c.ClientIP(),
		IsMobile:    isMobile(c),
		SrcHost:     c.Request.Host,
		SrcURL:      c.Request.Referer(),
		OrderType:   req.OrderType,
		PlanID:      req.PlanID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
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
	response.Paginated(c, orders, int64(total), page, pageSize)
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
	response.Success(c, order)
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
	return strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone")
}
