package admin

import (
	"math"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentHandler handles admin payment management.
type PaymentHandler struct {
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
}

// NewPaymentHandler creates a new admin PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		configService:  configService,
	}
}

// --- Dashboard ---

// GetDashboard returns payment dashboard statistics.
// GET /api/v1/admin/payment/dashboard
func (h *PaymentHandler) GetDashboard(c *gin.Context) {
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

// GetDashboardLegacy returns the old sub2apipay dashboard payload shape.
// GET /api/v1/admin/sub2api/dashboard
func (h *PaymentHandler) GetDashboardLegacy(c *gin.Context) {
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
	response.Success(c, legacyDashboardStatsFromService(stats, days, time.Now()))
}

// --- Orders ---

// ListOrders returns a paginated list of all payment orders.
// GET /api/v1/admin/payment/orders
func (h *PaymentHandler) ListOrders(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
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
// GET /api/v1/admin/payment/orders/:id
func (h *PaymentHandler) GetOrderDetail(c *gin.Context) {
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
// POST /api/v1/admin/payment/orders/:id/cancel
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
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
// POST /api/v1/admin/payment/orders/:id/retry
func (h *PaymentHandler) RetryFulfillment(c *gin.Context) {
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

// ListOrdersLegacy returns the old sub2apipay admin order list shape.
// GET /api/v1/admin/sub2api/orders
func (h *PaymentHandler) ListOrdersLegacy(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
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
	out := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		out = append(out, legacyAdminOrderFromEnt(order))
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	response.Success(c, gin.H{"orders": out, "total": total, "total_pages": totalPages})
}

// GetOrderDetailLegacy returns the old sub2apipay admin order detail shape.
// GET /api/v1/admin/sub2api/orders/:id
func (h *PaymentHandler) GetOrderDetailLegacy(c *gin.Context) {
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
	response.Success(c, legacyAdminOrderDetailFromEnt(order, auditLogs))
}

// CancelOrderLegacy cancels an order and responds using the old sub2apipay shape.
// POST /api/v1/admin/sub2api/orders/:id/cancel
func (h *PaymentHandler) CancelOrderLegacy(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	msg, err := h.paymentService.AdminCancelOrder(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true, "message": msg})
}

// RetryFulfillmentLegacy retries fulfillment using the old sub2apipay route shape.
// POST /api/v1/admin/sub2api/orders/:id/retry
func (h *PaymentHandler) RetryFulfillmentLegacy(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.paymentService.RetryFulfillment(c.Request.Context(), orderID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true, "message": "fulfillment retried"})
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

func legacyAdminOrderFromEnt(order *dbent.PaymentOrder) gin.H {
	if order == nil {
		return gin.H{}
	}
	return gin.H{
		"id":              strconv.FormatInt(order.ID, 10),
		"userId":          order.UserID,
		"userName":        order.UserName,
		"userEmail":       order.UserEmail,
		"userNotes":       stringPtrValue(order.UserNotes),
		"amount":          order.Amount,
		"status":          order.Status,
		"paymentType":     order.PaymentType,
		"createdAt":       formatLegacyTime(order.CreatedAt),
		"paidAt":          formatLegacyTimePtr(order.PaidAt),
		"completedAt":     formatLegacyTimePtr(order.CompletedAt),
		"failedReason":    stringPtrValue(order.FailedReason),
		"expiresAt":       formatLegacyTime(order.ExpiresAt),
		"srcHost":         order.SrcHost,
		"rechargeCode":    order.RechargeCode,
		"paymentTradeNo":  order.PaymentTradeNo,
		"refundAmount":    order.RefundAmount,
		"refundReason":    stringPtrValue(order.RefundReason),
		"refundAt":        formatLegacyTimePtr(order.RefundAt),
		"forceRefund":     order.ForceRefund,
		"failedAt":        formatLegacyTimePtr(order.FailedAt),
		"updatedAt":       formatLegacyTime(order.UpdatedAt),
		"clientIp":        order.ClientIP,
		"srcUrl":          stringPtrValue(order.SrcURL),
		"paymentSuccess":  legacyPaymentSuccess(order),
		"rechargeSuccess": legacyRechargeSuccess(order),
		"rechargeStatus":  legacyRechargeStatus(order),
	}
}

func legacyAdminOrderDetailFromEnt(order *dbent.PaymentOrder, logs []*dbent.PaymentAuditLog) gin.H {
	out := legacyAdminOrderFromEnt(order)
	auditLogs := make([]gin.H, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		auditLogs = append(auditLogs, gin.H{
			"id":        log.ID,
			"action":    log.Action,
			"detail":    log.Detail,
			"operator":  log.Operator,
			"createdAt": formatLegacyTime(log.CreatedAt),
		})
	}
	out["auditLogs"] = auditLogs
	return out
}

func legacyPaymentSuccess(order *dbent.PaymentOrder) bool {
	if order == nil {
		return false
	}
	return order.PaidAt != nil || legacyRechargeSuccess(order)
}

func legacyRechargeSuccess(order *dbent.PaymentOrder) bool {
	return order != nil && (order.CompletedAt != nil || order.Status == "COMPLETED")
}

func legacyRechargeStatus(order *dbent.PaymentOrder) string {
	if order == nil {
		return "not_paid"
	}
	switch order.Status {
	case "COMPLETED":
		return "success"
	case "RECHARGING":
		return "recharging"
	case "FAILED":
		return "failed"
	case "EXPIRED", "CANCELLED", "REFUNDING", "REFUNDED", "REFUND_FAILED":
		return "closed"
	}
	if order.CompletedAt != nil {
		return "success"
	}
	if order.PaidAt != nil {
		return "paid_pending"
	}
	return "not_paid"
}

func formatLegacyTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func formatLegacyTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339)
}

func stringPtrValue(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func legacyDashboardStatsFromService(stats *service.DashboardStats, days int, generatedAt time.Time) gin.H {
	if stats == nil {
		stats = &service.DashboardStats{}
	}
	dailySeries := make([]gin.H, 0, len(stats.DailySeries))
	for _, item := range stats.DailySeries {
		dailySeries = append(dailySeries, gin.H{
			"date":   item.Date,
			"amount": item.Amount,
			"count":  item.Count,
		})
	}

	leaderboard := make([]gin.H, 0, len(stats.TopUsers))
	for _, user := range stats.TopUsers {
		leaderboard = append(leaderboard, gin.H{
			"userId":      user.UserID,
			"userName":    nil,
			"userEmail":   user.Email,
			"totalAmount": user.Amount,
			"orderCount":  0,
		})
	}

	paymentMethods := make([]gin.H, 0, len(stats.PaymentMethods))
	for _, method := range stats.PaymentMethods {
		percentage := 0.0
		if stats.TotalAmount > 0 {
			percentage = math.Round(method.Amount/stats.TotalAmount*10000) / 100
		}
		paymentMethods = append(paymentMethods, gin.H{
			"paymentType": method.Type,
			"amount":      method.Amount,
			"count":       method.Count,
			"percentage":  percentage,
		})
	}

	successRate := 0.0
	if stats.TotalCount > 0 {
		successRate = 100
	}
	return gin.H{
		"summary": gin.H{
			"today": gin.H{
				"amount":     stats.TodayAmount,
				"orderCount": stats.TodayCount,
				"paidCount":  stats.TodayCount,
			},
			"total": gin.H{
				"amount":     stats.TotalAmount,
				"orderCount": stats.TotalCount,
				"paidCount":  stats.TotalCount,
			},
			"successRate": successRate,
			"avgAmount":   stats.AvgAmount,
		},
		"dailySeries":    dailySeries,
		"leaderboard":    leaderboard,
		"paymentMethods": paymentMethods,
		"meta": gin.H{
			"days":        days,
			"generatedAt": formatLegacyTime(generatedAt),
		},
	}
}

// AdminProcessRefundRequest is the request body for admin refund processing.
type AdminProcessRefundRequest struct {
	Amount        float64 `json:"amount"`
	Reason        string  `json:"reason"`
	Force         bool    `json:"force"`
	DeductBalance bool    `json:"deduct_balance"`
}

// ProcessRefund processes a refund for an order (admin).
// POST /api/v1/admin/payment/orders/:id/refund
func (h *PaymentHandler) ProcessRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req AdminProcessRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, earlyResult, err := h.paymentService.PrepareRefund(c.Request.Context(), orderID, req.Amount, req.Reason, req.Force, req.DeductBalance)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if earlyResult != nil {
		response.Success(c, earlyResult)
		return
	}

	result, err := h.paymentService.ExecuteRefund(c.Request.Context(), plan)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// --- Subscription Plans ---

// ListPlans returns all subscription plans.
// GET /api/v1/admin/payment/plans
func (h *PaymentHandler) ListPlans(c *gin.Context) {
	plans, err := h.configService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plans)
}

// ListPlansLegacy returns all subscription plans using the old sub2apipay
// management API shape.
// GET /api/v1/admin/subscription-plans
func (h *PaymentHandler) ListPlansLegacy(c *gin.Context) {
	plans, err := h.configService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groupInfo := h.configService.GetGroupInfoMap(c.Request.Context(), plans)
	response.Success(c, gin.H{"plans": legacySubscriptionPlans(plans, groupInfo, true)})
}

// CreatePlanLegacy accepts the old sub2apipay plan payload and stores it in the
// native sub2api subscription plan model.
// POST /api/v1/admin/subscription-plans
func (h *PaymentHandler) CreatePlanLegacy(c *gin.Context) {
	var req legacyPlanUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	createReq := service.CreatePlanRequest{}
	if req.GroupID != nil {
		createReq.GroupID = *req.GroupID
	}
	if req.Name != nil {
		createReq.Name = *req.Name
	}
	if req.Description != nil {
		createReq.Description = *req.Description
	}
	if req.Price != nil {
		createReq.Price = *req.Price
	}
	createReq.OriginalPrice = req.OriginalPrice
	if req.ValidityDays != nil {
		createReq.ValidityDays = *req.ValidityDays
	}
	if req.ValidityUnit != nil {
		createReq.ValidityUnit = *req.ValidityUnit
	}
	if req.Features != nil {
		createReq.Features = flattenLegacyFeatures(*req.Features)
	}
	if req.ProductName != nil {
		createReq.ProductName = *req.ProductName
	}
	if req.ForSale != nil {
		createReq.ForSale = *req.ForSale
	}
	if req.SortOrder != nil {
		createReq.SortOrder = *req.SortOrder
	}
	plan, err := h.configService.CreatePlan(c.Request.Context(), createReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, plan)
}

// UpdatePlanLegacy accepts patch-style old sub2apipay plan updates.
// PUT /api/v1/admin/subscription-plans/:id
func (h *PaymentHandler) UpdatePlanLegacy(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req legacyPlanUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updateReq := service.UpdatePlanRequest{
		GroupID:       req.GroupID,
		Name:          req.Name,
		Description:   req.Description,
		Price:         req.Price,
		OriginalPrice: req.OriginalPrice,
		ValidityDays:  req.ValidityDays,
		ValidityUnit:  req.ValidityUnit,
		ProductName:   req.ProductName,
		ForSale:       req.ForSale,
		SortOrder:     req.SortOrder,
	}
	if req.Features != nil {
		features := flattenLegacyFeatures(*req.Features)
		updateReq.Features = &features
	}
	plan, err := h.configService.UpdatePlan(c.Request.Context(), id, updateReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// DeletePlanLegacy deletes a plan through the old sub2apipay route.
// DELETE /api/v1/admin/subscription-plans/:id
func (h *PaymentHandler) DeletePlanLegacy(c *gin.Context) {
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

// CreatePlan creates a new subscription plan.
// POST /api/v1/admin/payment/plans
func (h *PaymentHandler) CreatePlan(c *gin.Context) {
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
// PUT /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) UpdatePlan(c *gin.Context) {
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
// DELETE /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) DeletePlan(c *gin.Context) {
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

// --- Provider Instances ---

// ListProviders returns all payment provider instances.
// GET /api/v1/admin/payment/providers
func (h *PaymentHandler) ListProviders(c *gin.Context) {
	providers, err := h.configService.ListProviderInstancesWithConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, providers)
}

// CreateProvider creates a new payment provider instance.
// POST /api/v1/admin/payment/providers
func (h *PaymentHandler) CreateProvider(c *gin.Context) {
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
// PUT /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) UpdateProvider(c *gin.Context) {
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
// DELETE /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) DeleteProvider(c *gin.Context) {
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
// Returns the parsed ID and true on success; on failure it writes a BadRequest response and returns false.
func parseIDParam(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid "+paramName)
		return 0, false
	}
	return id, true
}

// --- Product catalog ---

// GetCatalogProducts returns all homepage/shop products for admin editing.
// GET /api/v1/admin/payment/products
func (h *PaymentHandler) GetCatalogProducts(c *gin.Context) {
	products, err := h.configService.GetCatalogProducts(c.Request.Context(), true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"products": products})
}

// UpdateCatalogProducts replaces the homepage/shop product list.
// PUT /api/v1/admin/payment/products
func (h *PaymentHandler) UpdateCatalogProducts(c *gin.Context) {
	var req struct {
		Products []service.CatalogProduct `json:"products"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.configService.SetCatalogProducts(c.Request.Context(), req.Products); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	products, err := h.configService.GetCatalogProducts(c.Request.Context(), true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"products": products})
}

// --- Config ---

// GetConfig returns the payment configuration (admin view).
// GET /api/v1/admin/payment/config
func (h *PaymentHandler) GetConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig updates the payment configuration.
// PUT /api/v1/admin/payment/config
func (h *PaymentHandler) UpdateConfig(c *gin.Context) {
	var req service.UpdatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.configService.UpdatePaymentConfig(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "updated"})
}
