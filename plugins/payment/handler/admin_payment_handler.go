// Package handler — payment plugin admin HTTP endpoints.
//
// Adapted from backend/internal/handler. Admin identity is verified via
// pluginsdk.RequestMetadata.UserRole == "admin" — when the host's auth
// middleware did not flag the caller as admin we return 403.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	dbent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/response"
	"github.com/Wei-Shaw/sub2api/plugins/payment/service"
)

// adminUserRole is the role string the host's auth layer injects into
// X-Plugin-User-Role for administrator JWTs.
const adminUserRole = "admin"

// AdminPaymentHandler handles admin payment management.
type AdminPaymentHandler struct {
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
}

// NewAdminPaymentHandler creates a new admin AdminPaymentHandler.
func NewAdminPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService) *AdminPaymentHandler {
	return &AdminPaymentHandler{
		paymentService: paymentService,
		configService:  configService,
	}
}

// RegisterRoutes attaches admin payment routes onto the supplied router
// group. The plugin host calls this once during Init; every handler
// gates itself on requireAdmin so a non-admin caller gets a 403 even if
// the upstream gateway misroutes a user JWT here.
func (h *AdminPaymentHandler) RegisterRoutes(admin *gin.RouterGroup) {
	admin.GET("/dashboard", h.GetDashboard)
	admin.GET("/config", h.GetConfig)
	admin.PUT("/config", h.UpdateConfig)
	admin.GET("/orders", h.ListOrders)
	admin.GET("/orders/:id", h.GetOrderDetail)
	admin.POST("/orders/:id/cancel", h.CancelOrder)
	admin.POST("/orders/:id/retry", h.RetryFulfillment)
	admin.POST("/orders/:id/refund", h.ProcessRefund)
	admin.GET("/plans", h.ListPlans)
	admin.POST("/plans", h.CreatePlan)
	admin.PUT("/plans/:id", h.UpdatePlan)
	admin.DELETE("/plans/:id", h.DeletePlan)
	admin.GET("/providers", h.ListProviders)
	admin.POST("/providers", h.CreateProvider)
	admin.PUT("/providers/:id", h.UpdateProvider)
	admin.DELETE("/providers/:id", h.DeleteProvider)
}

// requireAdmin verifies the caller is an authenticated admin via the
// X-Plugin-User-Role header. Writes a 401 / 403 response and returns
// false when the request fails the gate.
func requireAdmin(c *gin.Context) bool {
	meta := pluginsdk.RequestMetadata(c.Request)
	if meta.UserID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "User not authenticated"})
		return false
	}
	if meta.UserRole != adminUserRole {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "Admin role required"})
		return false
	}
	return true
}

// GetDashboard returns payment dashboard statistics.
func (h *AdminPaymentHandler) GetDashboard(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	days := 30
	if d := c.Query("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}
	stats, err := h.paymentService.GetDashboardStats(c.Request.Context(), days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// ListOrders returns a paginated list of all payment orders.
func (h *AdminPaymentHandler) ListOrders(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	page, pageSize := parsePagination(c)
	var userID int64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = v
		}
	}
	orders, total, err := h.paymentService.AdminListOrders(c.Request.Context(), userID, service.OrderListParams{
		Page:        page,
		PageSize:    pageSize,
		Status:      c.Query("status"),
		OrderType:   c.Query("order_type"),
		PaymentType: c.Query("payment_type"),
		Keyword:     c.Query("keyword"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, sanitizeAdminPaymentOrdersForResponse(orders), int64(total), page, pageSize)
}

// GetOrderDetail returns detailed information about a single order.
func (h *AdminPaymentHandler) GetOrderDetail(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	order, err := h.paymentService.GetOrderByID(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditLogs, _ := h.paymentService.GetOrderAuditLogs(c.Request.Context(), orderID)
	response.Success(c, gin.H{"order": sanitizeAdminPaymentOrderForResponse(order), "auditLogs": auditLogs})
}

// CancelOrder cancels a pending order (admin).
func (h *AdminPaymentHandler) CancelOrder(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	msg, err := h.paymentService.AdminCancelOrder(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": msg})
}

// RetryFulfillment retries fulfillment for a paid order.
func (h *AdminPaymentHandler) RetryFulfillment(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.paymentService.RetryFulfillment(c.Request.Context(), orderID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "fulfillment retried"})
}

func sanitizeAdminPaymentOrdersForResponse(orders []*dbent.PaymentOrder) []*dbent.PaymentOrder {
	if len(orders) == 0 {
		return orders
	}
	out := make([]*dbent.PaymentOrder, 0, len(orders))
	for _, order := range orders {
		out = append(out, sanitizeAdminPaymentOrderForResponse(order))
	}
	return out
}

func sanitizeAdminPaymentOrderForResponse(order *dbent.PaymentOrder) *dbent.PaymentOrder {
	if order == nil {
		return nil
	}
	cloned := *order
	cloned.ProviderSnapshot = nil
	return &cloned
}

// AdminProcessRefundRequest is the request body for admin refund processing.
//
// Amount accepts either a JSON number or a string. Strings are parsed via
// shopspring/decimal so callers can avoid float64 round-trip drift; the
// numeric path is kept for backwards compatibility but every value is
// validated against NaN/Inf/negative-zero/sub-cent precision before reaching
// the service layer.
//
// ConfirmNoBalanceDeduction is required when Force=true && DeductBalance=false.
// Without this explicit acknowledgement the handler refuses the request:
// the (force, no-deduct) combination is the silent free-money path —
// refund is sent to the gateway without debiting the user's balance, so
// the user keeps both the credited balance and the refunded payment. We
// require operators to opt in to that behaviour explicitly and we mark
// the audit log so security review can flag the request.
type AdminProcessRefundRequest struct {
	// Amount is the refund amount in CNY yuan. JSON numbers and strings
	// are both accepted. <= 0 / NaN / Inf are rejected. Sub-cent precision
	// is rejected. Empty/missing falls through to a full refund (service
	// layer normalises to o.Amount when amt <= 0).
	Amount        decimal.Decimal `json:"amount"`
	Reason        string          `json:"reason"`
	Force         bool            `json:"force"`
	DeductBalance bool            `json:"deduct_balance"`
	// ConfirmNoBalanceDeduction must be true when Force=true and
	// DeductBalance=false. See the type doc for rationale.
	ConfirmNoBalanceDeduction bool `json:"confirm_no_balance_deduction"`
}

// validateAdminRefundAmount enforces the strict numeric envelope:
// negative values rejected, sub-cent precision rejected. Decimal types
// have no NaN/Inf so the previous IEEE-754 guard is no longer required.
// Returns the validated decimal (passed straight to the service layer)
// plus an error string suitable for a 400 response.
func validateAdminRefundAmount(amt decimal.Decimal) (decimal.Decimal, string) {
	// Zero is allowed and treated as "full refund" by the service layer.
	if amt.IsZero() {
		return decimal.Zero, ""
	}
	if amt.IsNegative() {
		return decimal.Zero, "amount must be non-negative"
	}
	// Reject sub-cent precision: yuan*100 must be an integer at decimal
	// precision (no rounding required). decimal.Equal compares values, so
	// .Truncate(2) catches anything below 0.01 CNY.
	if !amt.Equal(amt.Truncate(2)) {
		return decimal.Zero, "amount cannot have sub-cent precision"
	}
	return amt, ""
}

// ProcessRefund processes a refund for an order (admin).
func (h *AdminPaymentHandler) ProcessRefund(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req AdminProcessRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	amount, validationErr := validateAdminRefundAmount(req.Amount)
	if validationErr != "" {
		response.BadRequest(c, "Invalid request: "+validationErr)
		return
	}

	meta := pluginsdk.RequestMetadata(c.Request)
	if req.Force && !req.DeductBalance {
		if !req.ConfirmNoBalanceDeduction {
			response.BadRequest(c,
				"Invalid request: force refund without deducting user balance requires confirm_no_balance_deduction=true")
			return
		}
		// Audit-flag the dangerous combination so security review can
		// follow the trail. The service layer also writes its own audit
		// row on success — this entry captures the admin intent before
		// the refund runs.
		c.Set("payment.admin.no_deduct_force",
			strings.Join([]string{
				"order_id=" + strconv.FormatInt(orderID, 10),
				"admin_id=" + strconv.FormatInt(meta.UserID, 10),
				"reason=" + req.Reason,
			}, " "))
	}

	plan, earlyResult, err := h.paymentService.PrepareRefund(c.Request.Context(), orderID, amount, req.Reason, req.Force, req.DeductBalance)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if earlyResult != nil {
		response.Success(c, earlyResult)
		return
	}

	if req.Force && !req.DeductBalance && h.paymentService != nil {
		h.paymentService.LogAdminForceRefundNoDeduct(c.Request.Context(), orderID, meta.UserID, amount, req.Reason)
	}

	result, err := h.paymentService.ExecuteRefund(c.Request.Context(), plan)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ListPlans returns all subscription plans.
func (h *AdminPaymentHandler) ListPlans(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	plans, err := h.configService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plans)
}

// CreatePlan creates a new subscription plan.
//
// TODO(payment-migration): the plugin no longer owns subscription_plans —
// the underlying service returns errPlanWriteNotSupported. Once host admin
// owns plan CRUD this endpoint can be removed; for now we forward the
// service error so the frontend renders the same "use host admin" hint.
func (h *AdminPaymentHandler) CreatePlan(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var req service.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	plan, err := h.configService.CreatePlan(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, plan)
}

// UpdatePlan updates an existing subscription plan.
//
// TODO(payment-migration): plan writes are stubbed at the service layer;
// see CreatePlan for context.
func (h *AdminPaymentHandler) UpdatePlan(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	plan, err := h.configService.UpdatePlan(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// DeletePlan deletes a subscription plan.
//
// TODO(payment-migration): plan writes are stubbed at the service layer.
func (h *AdminPaymentHandler) DeletePlan(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configService.DeletePlan(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

// ListProviders returns all payment provider instances.
func (h *AdminPaymentHandler) ListProviders(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	providers, err := h.configService.ListProviderInstancesWithConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, providers)
}

// CreateProvider creates a new payment provider instance.
func (h *AdminPaymentHandler) CreateProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var req service.CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.CreateProviderInstance(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Created(c, inst)
}

// UpdateProvider updates an existing payment provider instance.
func (h *AdminPaymentHandler) UpdateProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.UpdateProviderInstance(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, inst)
}

// DeleteProvider deletes a payment provider instance.
func (h *AdminPaymentHandler) DeleteProvider(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configService.DeleteProviderInstance(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, gin.H{"message": "deleted"})
}

// parseIDParam parses an int64 path parameter.
func parseIDParam(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid "+paramName)
		return 0, false
	}
	return id, true
}

// GetConfig returns the payment configuration (admin view).
func (h *AdminPaymentHandler) GetConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig saves payment configuration via SDK Settings + refreshes
// providers so changes take effect immediately. Mirrors the release
// branch's setting_handler.go behaviour: settings persist first, then
// the in-memory provider registry is invalidated.
func (h *AdminPaymentHandler) UpdateConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var req service.UpdatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.configService.UpdatePaymentConfig(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.paymentService != nil {
		h.paymentService.RefreshProviders(c.Request.Context())
	}
	response.Success(c, gin.H{"message": "updated"})
}
