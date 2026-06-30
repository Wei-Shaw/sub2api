package admin

import (
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
REDACTED

// NewPaymentHandler creates a new admin PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		configService:  configService,
REDACTED
REDACTED

// --- Dashboard ---

// GetDashboard returns payment dashboard statistics.
// GET /api/v1/admin/payment/dashboard
func (h *PaymentHandler) GetDashboard(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
	REDACTED
REDACTED
	stats, err := h.paymentService.GetDashboardStats(c.Request.Context(), days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, stats)
REDACTED

// --- Orders ---

// ListOrders returns a paginated list of all payment orders.
// GET /api/v1/admin/payment/orders
func (h *PaymentHandler) ListOrders(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = v
	REDACTED
REDACTED
	orders, total, err := h.paymentService.AdminListOrders(c.Request.Context(), userID, service.OrderListParams{
		Page:        page,
		PageSize:    pageSize,
		Status:      c.Query("status"),
		OrderType:   c.Query("order_type"),
		PaymentType: c.Query("payment_type"),
		Keyword:     c.Query("keyword"),
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Paginated(c, sanitizeAdminPaymentOrdersForResponse(orders), int64(total), page, pageSize)
REDACTED

// GetOrderDetail returns detailed information about a single order.
// GET /api/v1/admin/payment/orders/:id
func (h *PaymentHandler) GetOrderDetail(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	order, err := h.paymentService.GetOrderByID(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	auditLogs, _ := h.paymentService.GetOrderAuditLogs(c.Request.Context(), orderID)
	response.Success(c, gin.H{"order": sanitizeAdminPaymentOrderForResponse(order), "auditLogs": auditLogsREDACTED)
REDACTED

// CancelOrder cancels a pending order (admin).
// POST /api/v1/admin/payment/orders/:id/cancel
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	msg, err := h.paymentService.AdminCancelOrder(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"message": msgREDACTED)
REDACTED

// RetryFulfillment retries fulfillment for a paid order.
// POST /api/v1/admin/payment/orders/:id/retry
func (h *PaymentHandler) RetryFulfillment(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	if err := h.paymentService.RetryFulfillment(c.Request.Context(), orderID); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"message": "fulfillment retried"REDACTED)
REDACTED

type AdminPaymentOrderResult struct {
	ID                  int64      `json:"id"`
	UserID              int64      `json:"user_id"`
	UserEmail           string     `json:"user_email,omitempty"`
	UserName            string     `json:"user_name,omitempty"`
	UserNotes           *string    `json:"user_notes,omitempty"`
	Amount              float64    `json:"amount"`
	PayAmount           float64    `json:"pay_amount"`
	FeeRate             float64    `json:"fee_rate"`
	Currency            string     `json:"currency"`
	RechargeCode        string     `json:"recharge_code,omitempty"`
	OutTradeNo          string     `json:"out_trade_no"`
	PaymentType         string     `json:"payment_type"`
	PaymentTradeNo      string     `json:"payment_trade_no,omitempty"`
	PayURL              *string    `json:"pay_url,omitempty"`
	QRCode              *string    `json:"qr_code,omitempty"`
	QRCodeImg           *string    `json:"qr_code_img,omitempty"`
	OrderType           string     `json:"order_type"`
	PlanID              *int64     `json:"plan_id,omitempty"`
	SubscriptionGroupID *int64     `json:"subscription_group_id,omitempty"`
	SubscriptionDays    *int       `json:"subscription_days,omitempty"`
	ProviderInstanceID  *string    `json:"provider_instance_id,omitempty"`
	ProviderKey         *string    `json:"provider_key,omitempty"`
	Status              string     `json:"status"`
	RefundAmount        float64    `json:"refund_amount"`
	RefundReason        *string    `json:"refund_reason,omitempty"`
	RefundAt            *time.Time `json:"refund_at,omitempty"`
	ForceRefund         bool       `json:"force_refund,omitempty"`
	RefundRequestedAt   *time.Time `json:"refund_requested_at,omitempty"`
	RefundRequestReason *string    `json:"refund_request_reason,omitempty"`
	RefundRequestedBy   *string    `json:"refund_requested_by,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	FailedAt            *time.Time `json:"failed_at,omitempty"`
	FailedReason        *string    `json:"failed_reason,omitempty"`
	ClientIP            string     `json:"client_ip,omitempty"`
	SrcHost             string     `json:"src_host,omitempty"`
	SrcURL              *string    `json:"src_url,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
REDACTED

func sanitizeAdminPaymentOrdersForResponse(orders []*dbent.PaymentOrder) []*AdminPaymentOrderResult {
	out := make([]*AdminPaymentOrderResult, 0, len(orders))
	for _, order := range orders {
		if item := sanitizeAdminPaymentOrderForResponse(order); item != nil {
			out = append(out, item)
	REDACTED
REDACTED
	return out
REDACTED

func sanitizeAdminPaymentOrderForResponse(order *dbent.PaymentOrder) *AdminPaymentOrderResult {
	if order == nil {
		return nil
REDACTED
	return &AdminPaymentOrderResult{
		ID:                  order.ID,
		UserID:              order.UserID,
		UserEmail:           order.UserEmail,
		UserName:            order.UserName,
		UserNotes:           order.UserNotes,
		Amount:              order.Amount,
		PayAmount:           order.PayAmount,
		FeeRate:             order.FeeRate,
		Currency:            service.PaymentOrderCurrency(order),
		RechargeCode:        order.RechargeCode,
		OutTradeNo:          order.OutTradeNo,
		PaymentType:         order.PaymentType,
		PaymentTradeNo:      order.PaymentTradeNo,
		PayURL:              order.PayURL,
		QRCode:              order.QrCode,
		QRCodeImg:           order.QrCodeImg,
		OrderType:           order.OrderType,
		PlanID:              order.PlanID,
		SubscriptionGroupID: order.SubscriptionGroupID,
		SubscriptionDays:    order.SubscriptionDays,
		ProviderInstanceID:  order.ProviderInstanceID,
		ProviderKey:         order.ProviderKey,
		Status:              order.Status,
		RefundAmount:        order.RefundAmount,
		RefundReason:        order.RefundReason,
		RefundAt:            order.RefundAt,
		ForceRefund:         order.ForceRefund,
		RefundRequestedAt:   order.RefundRequestedAt,
		RefundRequestReason: order.RefundRequestReason,
		RefundRequestedBy:   order.RefundRequestedBy,
		ExpiresAt:           order.ExpiresAt,
		PaidAt:              order.PaidAt,
		CompletedAt:         order.CompletedAt,
		FailedAt:            order.FailedAt,
		FailedReason:        order.FailedReason,
		ClientIP:            order.ClientIP,
		SrcHost:             order.SrcHost,
		SrcURL:              order.SrcURL,
		CreatedAt:           order.CreatedAt,
		UpdatedAt:           order.UpdatedAt,
REDACTED
REDACTED

// AdminProcessRefundRequest is the request body for admin refund processing.
type AdminProcessRefundRequest struct {
	Amount        float64 `json:"amount"`
	Reason        string  `json:"reason"`
	Force         bool    `json:"force"`
	DeductBalance bool    `json:"deduct_balance"`
REDACTED

// ProcessRefund processes a refund for an order (admin).
// POST /api/v1/admin/payment/orders/:id/refund
func (h *PaymentHandler) ProcessRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED

	var req AdminProcessRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	plan, earlyResult, err := h.paymentService.PrepareRefund(c.Request.Context(), orderID, req.Amount, req.Reason, req.Force, req.DeductBalance)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if earlyResult != nil {
		response.Success(c, earlyResult)
		return
REDACTED

	result, err := h.paymentService.ExecuteRefund(c.Request.Context(), plan)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

// QueryAndFinalizeRefund queries the provider refund status and finalizes a pending refund.
// POST /api/v1/admin/payment/orders/:id/refund/query
func (h *PaymentHandler) QueryAndFinalizeRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED

	result, err := h.paymentService.QueryAndFinalizeRefund(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

// --- Subscription Plans ---

// ListPlans returns all subscription plans.
// GET /api/v1/admin/payment/plans
func (h *PaymentHandler) ListPlans(c *gin.Context) {
	plans, err := h.configService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, plans)
REDACTED

// CreatePlan creates a new subscription plan.
// POST /api/v1/admin/payment/plans
func (h *PaymentHandler) CreatePlan(c *gin.Context) {
	var req service.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	plan, err := h.configService.CreatePlan(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Created(c, plan)
REDACTED

// UpdatePlan updates an existing subscription plan.
// PUT /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) UpdatePlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	var req service.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	plan, err := h.configService.UpdatePlan(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, plan)
REDACTED

// DeletePlan deletes a subscription plan.
// DELETE /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) DeletePlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	if err := h.configService.DeletePlan(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"message": "deleted"REDACTED)
REDACTED

// --- Provider Instances ---

// ListProviders returns all payment provider instances.
// GET /api/v1/admin/payment/providers
func (h *PaymentHandler) ListProviders(c *gin.Context) {
	providers, err := h.configService.ListProviderInstancesWithConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, providers)
REDACTED

// CreateProvider creates a new payment provider instance.
// POST /api/v1/admin/payment/providers
func (h *PaymentHandler) CreateProvider(c *gin.Context) {
	var req service.CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	inst, err := h.configService.CreateProviderInstance(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Created(c, inst)
REDACTED

// UpdateProvider updates an existing payment provider instance.
// PUT /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) UpdateProvider(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	var req service.UpdateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	inst, err := h.configService.UpdateProviderInstance(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, inst)
REDACTED

// DeleteProvider deletes a payment provider instance.
// DELETE /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) DeleteProvider(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
REDACTED
	if err := h.configService.DeleteProviderInstance(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, gin.H{"message": "deleted"REDACTED)
REDACTED

// parseIDParam parses an int64 path parameter.
// Returns the parsed ID and true on success; on failure it writes a BadRequest response and returns false.
func parseIDParam(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid "+paramName)
		return 0, false
REDACTED
	return id, true
REDACTED

// --- Config ---

// GetConfig returns the payment configuration (admin view).
// GET /api/v1/admin/payment/config
func (h *PaymentHandler) GetConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, cfg)
REDACTED

// UpdateConfig updates the payment configuration.
// PUT /api/v1/admin/payment/config
func (h *PaymentHandler) UpdateConfig(c *gin.Context) {
	var req service.UpdatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED
	if err := h.configService.UpdatePaymentConfig(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"message": "updated"REDACTED)
REDACTED
