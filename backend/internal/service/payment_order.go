package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Order Creation ---

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
REDACTED
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
REDACTED
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
REDACTED
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
REDACTED
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
REDACTED
	if err := s.checkCancelRateLimit(ctx, req.UserID, cfg); err != nil {
		return nil, err
REDACTED
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
REDACTED
	orderAmount := req.Amount
	limitAmount := req.Amount
	if plan != nil {
		orderAmount = plan.Price
		limitAmount = plan.Price
REDACTED else if req.OrderType == payment.OrderTypeBalance {
		orderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier)
REDACTED
	feeRate := cfg.RechargeFeeRate
	payAmountStr := payment.CalculatePayAmount(limitAmount, feeRate)
	payAmount, _ := strconv.ParseFloat(payAmountStr, 64)
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, orderAmount, limitAmount, feeRate, payAmount)
	if err != nil {
		return nil, err
REDACTED
	resp, err := s.invokeProvider(ctx, order, req, cfg, limitAmount, payAmountStr, payAmount, plan)
	if err != nil {
		_, _ = s.entClient.PaymentOrder.UpdateOneID(order.ID).
			SetStatus(OrderStatusFailed).
			Save(ctx)
		return nil, err
REDACTED
	return resp, nil
REDACTED

func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*dbent.SubscriptionPlan, error) {
	if req.OrderType == payment.OrderTypeBalance && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
REDACTED
	if req.OrderType == payment.OrderTypeSubscription {
		return s.validateSubOrder(ctx, req)
REDACTED
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
REDACTED
	if (cfg.MinAmount > 0 && req.Amount < cfg.MinAmount) || (cfg.MaxAmount > 0 && req.Amount > cfg.MaxAmount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{"min": fmt.Sprintf("%.2f", cfg.MinAmount), "max": fmt.Sprintf("%.2f", cfg.MaxAmount)REDACTED)
REDACTED
	return nil, nil
REDACTED

func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*dbent.SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
REDACTED
	plan, err := s.configService.GetPlan(ctx, req.PlanID)
	if err != nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
REDACTED
	group, err := s.groupRepo.GetByID(ctx, plan.GroupID)
	if err != nil || group.Status != payment.EntityStatusActive {
		return nil, infraerrors.NotFound("GROUP_NOT_FOUND", "subscription group is no longer available")
REDACTED
	if !group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_TYPE_MISMATCH", "group is not a subscription type")
REDACTED
	return plan, nil
REDACTED

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, orderAmount, limitAmount, feeRate, payAmount float64) (*dbent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
REDACTED
	if err := s.checkDailyLimit(ctx, tx, req.UserID, limitAmount, cfg.DailyLimit); err != nil {
		return nil, err
REDACTED
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = defaultOrderTimeoutMin
REDACTED
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(orderAmount).
		SetPayAmount(payAmount).
		SetFeeRate(feeRate).
		SetRechargeCode("").
		SetOutTradeNo(generateOutTradeNo()).
		SetPaymentType(req.PaymentType).
		SetPaymentTradeNo("").
		SetOrderType(req.OrderType).
		SetStatus(OrderStatusPending).
		SetExpiresAt(exp).
		SetClientIP(req.ClientIP).
		SetSrcHost(req.SrcHost)
	if req.SrcURL != "" {
		b.SetSrcURL(req.SrcURL)
REDACTED
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
REDACTED
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
REDACTED
	code := fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)
	order, err = tx.PaymentOrder.UpdateOneID(order.ID).SetRechargeCode(code).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set recharge code: %w", err)
REDACTED
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
REDACTED
	return order, nil
REDACTED

func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *dbent.Tx, userID int64, max int) error {
	if max <= 0 {
		max = defaultMaxPendingOrders
REDACTED
	c, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
REDACTED
	if c >= max {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", fmt.Sprintf("too many pending orders (max %d)", max)).
			WithMetadata(map[string]string{"max": strconv.Itoa(max)REDACTED)
REDACTED
	return nil
REDACTED

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
REDACTED
	ts := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted), paymentorder.PaidAtGTE(ts)).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
REDACTED
	var used float64
	for _, o := range orders {
		if o.OrderType == payment.OrderTypeBalance {
			used += o.PayAmount
			continue
	REDACTED
		used += o.Amount
REDACTED
	if used+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", fmt.Sprintf("daily recharge limit reached, remaining: %.2f", math.Max(0, limit-used)))
REDACTED
	return nil
REDACTED

func (s *PaymentService) invokeProvider(ctx context.Context, order *dbent.PaymentOrder, req CreateOrderRequest, cfg *PaymentConfig, limitAmount float64, payAmountStr string, payAmount float64, plan *dbent.SubscriptionPlan) (*CreateOrderResponse, error) {
	// Select an instance across all providers that support the requested payment type.
	// This enables cross-provider load balancing (e.g. EasyPay + Alipay direct for "alipay").
	sel, err := s.loadBalancer.SelectInstance(ctx, "", req.PaymentType, payment.Strategy(cfg.LoadBalanceStrategy), payAmount)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment method (%s) is not configured", req.PaymentType))
REDACTED
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no available payment instance")
REDACTED
	prov, err := provider.CreateProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment method is temporarily unavailable")
REDACTED
	subject := s.buildPaymentSubject(plan, limitAmount, cfg)
	outTradeNo := order.OutTradeNo
	canonicalReturnURL, err := CanonicalizeReturnURL(req.ReturnURL)
	if err != nil {
		return nil, err
REDACTED
	resumeToken := ""
	if resume := s.paymentResume(); resume != nil {
		resumeToken, err = resume.CreateToken(ResumeTokenClaims{
			OrderID:            order.ID,
			UserID:             order.UserID,
			ProviderInstanceID: sel.InstanceID,
			ProviderKey:        sel.ProviderKey,
			PaymentType:        req.PaymentType,
			CanonicalReturnURL: canonicalReturnURL,
	REDACTED)
		if err != nil {
			return nil, fmt.Errorf("create payment resume token: %w", err)
	REDACTED
REDACTED
	providerReturnURL, err := buildPaymentReturnURL(canonicalReturnURL, order.ID, resumeToken)
	if err != nil {
		return nil, err
REDACTED
	pr, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID:            outTradeNo,
		Amount:             payAmountStr,
		PaymentType:        req.PaymentType,
		Subject:            subject,
		ReturnURL:          providerReturnURL,
		ClientIP:           req.ClientIP,
		IsMobile:           req.IsMobile,
		InstanceSubMethods: sel.SupportedTypes,
REDACTED)
	if err != nil {
		slog.Error("[PaymentService] CreatePayment failed", "provider", sel.ProviderKey, "instance", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
REDACTED
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).SetNillablePayURL(psNilIfEmpty(pr.PayURL)).SetNillableQrCode(psNilIfEmpty(pr.QRCode)).SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update order with payment details: %w", err)
REDACTED
	s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{
		"paymentAmount":  req.Amount,
		"creditedAmount": order.Amount,
		"payAmount":      order.PayAmount,
		"paymentType":    req.PaymentType,
		"orderType":      req.OrderType,
		"paymentSource":  NormalizePaymentSource(req.PaymentSource),
REDACTED)
	return &CreateOrderResponse{
		OrderID:      order.ID,
		Amount:       order.Amount,
		PayAmount:    payAmount,
		FeeRate:      order.FeeRate,
		Status:       OrderStatusPending,
		PaymentType:  req.PaymentType,
		PayURL:       pr.PayURL,
		QRCode:       pr.QRCode,
		ClientSecret: pr.ClientSecret,
		ExpiresAt:    order.ExpiresAt,
		PaymentMode:  sel.PaymentMode,
		ResumeToken:  resumeToken,
REDACTED, nil
REDACTED

func (s *PaymentService) buildPaymentSubject(plan *dbent.SubscriptionPlan, limitAmount float64, cfg *PaymentConfig) string {
	if plan != nil {
		if plan.ProductName != "" {
			return plan.ProductName
	REDACTED
		return "Sub2API Subscription " + plan.Name
REDACTED
	amountStr := strconv.FormatFloat(limitAmount, 'f', 2, 64)
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	if pf != "" || sf != "" {
		return strings.TrimSpace(pf + " " + amountStr + " " + sf)
REDACTED
	return "Sub2API " + amountStr + " CNY"
REDACTED

// --- Order Queries ---

func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
REDACTED
	return o, nil
REDACTED

func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
REDACTED
	return o, nil
REDACTED

func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
REDACTED
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
REDACTED
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
REDACTED
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
REDACTED
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
REDACTED
	return orders, total, nil
REDACTED

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
REDACTED
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
REDACTED
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
REDACTED
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
REDACTED
	if p.Keyword != "" {
		q = q.Where(paymentorder.Or(
			paymentorder.OutTradeNoContainsFold(p.Keyword),
			paymentorder.UserEmailContainsFold(p.Keyword),
			paymentorder.UserNameContainsFold(p.Keyword),
		))
REDACTED
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
REDACTED
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
REDACTED
	return orders, total, nil
REDACTED
