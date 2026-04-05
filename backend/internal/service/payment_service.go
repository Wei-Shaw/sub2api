package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OrderStatusPending           = "PENDING"
	OrderStatusPaid              = "PAID"
	OrderStatusRecharging        = "RECHARGING"
	OrderStatusCompleted         = "COMPLETED"
	OrderStatusExpired           = "EXPIRED"
	OrderStatusCancelled         = "CANCELLED"
	OrderStatusFailed            = "FAILED"
	OrderStatusRefundRequested   = "REFUND_REQUESTED"
	OrderStatusRefunding         = "REFUNDING"
	OrderStatusPartiallyRefunded = "PARTIALLY_REFUNDED"
	OrderStatusRefunded          = "REFUNDED"
	OrderStatusRefundFailed      = "REFUND_FAILED"
)

const (
	defaultMaxPendingOrders = 3
	paymentGraceMinutes     = 5
)

type CreateOrderRequest struct {
	UserID      int64
	Amount      float64
	PaymentType string
	ClientIP    string
	IsMobile    bool
	SrcHost     string
	SrcURL      string
	OrderType   string
	PlanID      int64
}

type CreateOrderResponse struct {
	OrderID      int64     `json:"orderId"`
	Amount       float64   `json:"amount"`
	PayAmount    float64   `json:"payAmount"`
	FeeRate      float64   `json:"feeRate"`
	Status       string    `json:"status"`
	PaymentType  string    `json:"paymentType"`
	PayURL       string    `json:"payUrl,omitempty"`
	QRCode       string    `json:"qrCode,omitempty"`
	ClientSecret string    `json:"clientSecret,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
}

type RefundPlan struct {
	OrderID         int64
	Order           *dbent.PaymentOrder
	RefundAmount    float64
	GatewayAmount   float64
	Reason          string
	Force           bool
	DeductBalance   bool
	DeductionType   string
	BalanceToDeduct float64
	SubDaysToDeduct int
}

type RefundResult struct {
	Success         bool    `json:"success"`
	Warning         string  `json:"warning,omitempty"`
	RequireForce    bool    `json:"requireForce,omitempty"`
	BalanceDeducted float64 `json:"balanceDeducted,omitempty"`
	SubDaysDeducted int     `json:"subscriptionDaysDeducted,omitempty"`
}

type DashboardStats struct {
	TotalOrders     int     `json:"totalOrders"`
	TotalRevenue    float64 `json:"totalRevenue"`
	TotalRefunded   float64 `json:"totalRefunded"`
	PendingOrders   int     `json:"pendingOrders"`
	CompletedOrders int     `json:"completedOrders"`
	FailedOrders    int     `json:"failedOrders"`
}

type PaymentService struct {
	entClient       *dbent.Client
	registry        *payment.Registry
	loadBalancer    payment.LoadBalancer
	redeemService   *RedeemService
	subscriptionSvc *SubscriptionService
	configService   *PaymentConfigService
	userRepo        UserRepository
	groupRepo       GroupRepository
}

func NewPaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository) *PaymentService {
	return &PaymentService{entClient: entClient, registry: registry, loadBalancer: loadBalancer, redeemService: redeemService, subscriptionSvc: subscriptionSvc, configService: configService, userRepo: userRepo, groupRepo: groupRepo}
}

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = "balance"
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != "active" {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}
	amount := req.Amount
	if plan != nil {
		amount = plan.Price
	}
	feeRate := s.getFeeRate(req.PaymentType)
	payAmountStr := payment.CalculatePayAmount(amount, feeRate)
	payAmount, _ := strconv.ParseFloat(payAmountStr, 64)
	order, err := s.createOrderInTx(ctx, req, user, plan, cfg, amount, feeRate, payAmount)
	if err != nil {
		return nil, err
	}
	resp, err := s.invokeProvider(ctx, order, req, cfg, payAmountStr, payAmount, plan)
	if err != nil {
		_ = s.entClient.PaymentOrder.DeleteOneID(order.ID).Exec(ctx)
		return nil, err
	}
	return resp, nil
}

func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*dbent.SubscriptionPlan, error) {
	if req.OrderType == "balance" && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
	}
	if req.OrderType == "subscription" {
		return s.validateSubOrder(ctx, req)
	}
	if req.Amount < cfg.MinAmount || req.Amount > cfg.MaxAmount {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", fmt.Sprintf("amount must be between %.2f and %.2f", cfg.MinAmount, cfg.MaxAmount))
	}
	return nil, nil
}

func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*dbent.SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
	}
	plan, err := s.configService.GetPlan(ctx, req.PlanID)
	if err != nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
	}
	group, err := s.groupRepo.GetByID(ctx, plan.GroupID)
	if err != nil || group.Status != "active" {
		return nil, infraerrors.NotFound("GROUP_NOT_FOUND", "subscription group is no longer available")
	}
	if !group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("GROUP_TYPE_MISMATCH", "group is not a subscription type")
	}
	return plan, nil
}

func (s *PaymentService) createOrderInTx(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, amount, feeRate, payAmount float64) (*dbent.PaymentOrder, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.checkPendingLimit(ctx, tx, req.UserID, cfg.MaxPendingOrders); err != nil {
		return nil, err
	}
	if err := s.checkDailyLimit(ctx, tx, req.UserID, amount, cfg.DailyLimit); err != nil {
		return nil, err
	}
	tm := cfg.OrderTimeoutMin
	if tm <= 0 {
		tm = 30
	}
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	b := tx.PaymentOrder.Create().SetUserID(req.UserID).SetUserEmail(user.Email).SetUserName(user.Username).SetNillableUserNotes(psNilIfEmpty(user.Notes)).SetAmount(amount).SetPayAmount(payAmount).SetFeeRate(feeRate).SetRechargeCode("").SetPaymentType(req.PaymentType).SetPaymentTradeNo("").SetOrderType(req.OrderType).SetStatus(OrderStatusPending).SetExpiresAt(exp).SetClientIP(req.ClientIP).SetSrcHost(req.SrcHost)
	if req.SrcURL != "" {
		b.SetSrcURL(req.SrcURL)
	}
	if plan != nil {
		b.SetPlanID(plan.ID).SetSubscriptionGroupID(plan.GroupID).SetSubscriptionDays(psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit))
	}
	order, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	code := fmt.Sprintf("PAY-%d-%d", order.ID, time.Now().UnixNano()%100000)
	order, err = tx.PaymentOrder.UpdateOneID(order.ID).SetRechargeCode(code).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("set recharge code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit order transaction: %w", err)
	}
	return order, nil
}

func (s *PaymentService) checkPendingLimit(ctx context.Context, tx *dbent.Tx, userID int64, max int) error {
	if max <= 0 {
		max = defaultMaxPendingOrders
	}
	c, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusEQ(OrderStatusPending)).Count(ctx)
	if err != nil {
		return fmt.Errorf("count pending orders: %w", err)
	}
	if c >= max {
		return infraerrors.TooManyRequests("TOO_MANY_PENDING", fmt.Sprintf("too many pending orders (max %d)", max))
	}
	return nil
}

func (s *PaymentService) checkDailyLimit(ctx context.Context, tx *dbent.Tx, userID int64, amount, limit float64) error {
	if limit <= 0 {
		return nil
	}
	ts := psStartOfDayUTC(time.Now())
	orders, err := tx.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID), paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted), paymentorder.PaidAtGTE(ts)).All(ctx)
	if err != nil {
		return fmt.Errorf("query daily usage: %w", err)
	}
	var used float64
	for _, o := range orders {
		used += o.Amount
	}
	if used+amount > limit {
		return infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", fmt.Sprintf("daily recharge limit reached, remaining: %.2f", math.Max(0, limit-used)))
	}
	return nil
}

func (s *PaymentService) invokeProvider(ctx context.Context, order *dbent.PaymentOrder, req CreateOrderRequest, cfg *PaymentConfig, payAmountStr string, payAmount float64, plan *dbent.SubscriptionPlan) (*CreateOrderResponse, error) {
	provider, err := s.registry.GetProvider(req.PaymentType)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment method (%s) is not configured", req.PaymentType))
	}
	sel, err := s.loadBalancer.SelectInstance(ctx, provider.ProviderKey(), req.PaymentType)
	if err != nil {
		return nil, fmt.Errorf("select provider instance: %w", err)
	}
	if sel == nil {
		return nil, infraerrors.TooManyRequests("NO_AVAILABLE_INSTANCE", "no available payment instance")
	}
	subject := s.buildPaymentSubject(plan, payAmountStr, cfg)
	pr, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: strconv.FormatInt(order.ID, 10), Amount: payAmountStr, PaymentType: req.PaymentType, Subject: subject, ClientIP: req.ClientIP, IsMobile: req.IsMobile})
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment method is temporarily unavailable")
	}
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetPaymentTradeNo(pr.TradeNo).SetNillablePayURL(psNilIfEmpty(pr.PayURL)).SetNillableQrCode(psNilIfEmpty(pr.QRCode)).SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update order with payment details: %w", err)
	}
	s.writeAuditLog(ctx, order.ID, "ORDER_CREATED", fmt.Sprintf("user:%d", req.UserID), map[string]any{"amount": req.Amount, "paymentType": req.PaymentType, "orderType": req.OrderType})
	return &CreateOrderResponse{OrderID: order.ID, Amount: order.Amount, PayAmount: payAmount, FeeRate: order.FeeRate, Status: OrderStatusPending, PaymentType: req.PaymentType, PayURL: pr.PayURL, QRCode: pr.QRCode, ClientSecret: pr.ClientSecret, ExpiresAt: order.ExpiresAt}, nil
}

func (s *PaymentService) buildPaymentSubject(plan *dbent.SubscriptionPlan, payAmountStr string, cfg *PaymentConfig) string {
	if plan != nil {
		if plan.ProductName != "" {
			return plan.ProductName
		}
		return "Sub2API Subscription " + plan.Name
	}
	pf := strings.TrimSpace(cfg.ProductNamePrefix)
	sf := strings.TrimSpace(cfg.ProductNameSuffix)
	if pf != "" || sf != "" {
		return strings.TrimSpace(pf + " " + payAmountStr + " " + sf)
	}
	return "Sub2API " + payAmountStr + " CNY"
}

func (s *PaymentService) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

func (s *PaymentService) GetOrderByID(ctx context.Context, orderID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	return o, nil
}

func (s *PaymentService) GetUserOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID))
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
	}
	ps := p.PageSize
	if ps <= 0 {
		ps = 20
	}
	if ps > 100 {
		ps = 100
	}
	pg := p.Page
	if pg < 1 {
		pg = 1
	}
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query user orders: %w", err)
	}
	return orders, total, nil
}

func (s *PaymentService) CancelOrder(ctx context.Context, orderID, userID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return "", infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, fmt.Sprintf("user:%d", userID), "user cancelled order")
}

func (s *PaymentService) AdminCancelOrder(ctx context.Context, orderID int64) (string, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, "admin", "admin cancelled order")
}

func (s *PaymentService) cancelCore(ctx context.Context, o *dbent.PaymentOrder, fs, op, ad string) (string, error) {
	if o.PaymentTradeNo != "" && o.PaymentType != "" {
		if s.checkPaid(ctx, o) == "already_paid" {
			return "already_paid", nil
		}
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).SetStatus(fs).Save(ctx)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
	}
	if c > 0 {
		s.writeAuditLog(ctx, o.ID, "ORDER_CANCELLED", op, map[string]any{"detail": ad})
	}
	return "cancelled", nil
}

func (s *PaymentService) checkPaid(ctx context.Context, o *dbent.PaymentOrder) string {
	prov, err := s.registry.GetProvider(o.PaymentType)
	if err != nil {
		return ""
	}
	resp, err := prov.QueryOrder(ctx, o.PaymentTradeNo)
	if err != nil {
		slog.Warn("query upstream failed", "orderID", o.ID, "error", err)
		return ""
	}
	if resp.Status == "paid" {
		_ = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{TradeNo: o.PaymentTradeNo, OrderID: strconv.FormatInt(o.ID, 10), Amount: resp.Amount, Status: "success"}, prov.ProviderKey())
		return "already_paid"
	}
	if cp, ok := prov.(payment.CancelableProvider); ok {
		_ = cp.CancelPayment(ctx, o.PaymentTradeNo)
	}
	return ""
}

func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	if n.Status != "success" {
		return nil
	}
	oid, err := strconv.ParseInt(n.OrderID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid order ID: %s", n.OrderID)
	}
	return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk)
}

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		slog.Error("order not found", "orderID", oid)
		return nil
	}
	if math.Abs(paid-o.PayAmount) > 0.01 {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
		return fmt.Errorf("amount mismatch: expected %.2f, got %.2f", o.PayAmount, paid)
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, pk string) error {
	now := time.Now()
	grace := now.Add(-paymentGraceMinutes * time.Minute)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.Or(paymentorder.StatusEQ(OrderStatusPending), paymentorder.And(paymentorder.StatusEQ(OrderStatusExpired), paymentorder.UpdatedAtGTE(grace)))).SetStatus(OrderStatusPaid).SetPayAmount(paid).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o)
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid})
	return s.executeFulfillment(ctx, o.ID)
}

func (s *PaymentService) alreadyProcessed(ctx context.Context, o *dbent.PaymentOrder) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded:
		return nil
	case OrderStatusFailed:
		return s.executeFulfillment(ctx, o.ID)
	case OrderStatusPaid, OrderStatusRecharging:
		return fmt.Errorf("order %d is being processed", o.ID)
	default:
		return nil
	}
}

func (s *PaymentService) executeFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if o.OrderType == "subscription" {
		return s.ExecuteSubscriptionFulfillment(ctx, oid)
	}
	return s.ExecuteBalanceFulfillment(ctx, oid)
}

func (s *PaymentService) ExecuteBalanceFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doBalance(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder) error {
	rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
	if err := s.redeemService.CreateCode(ctx, rc); err != nil {
		return fmt.Errorf("create redeem code: %w", err)
	}
	if _, err := s.redeemService.Redeem(ctx, o.UserID, o.RechargeCode); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	now := time.Now()
	_, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, "RECHARGE_SUCCESS", "system", map[string]any{"rechargeCode": o.RechargeCode, "amount": o.Amount})
	return nil
}

func (s *PaymentService) ExecuteSubscriptionFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		return nil
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).SetStatus(OrderStatusRecharging).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil
	}
	if err := s.doSub(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

func (s *PaymentService) doSub(ctx context.Context, o *dbent.PaymentOrder) error {
	gid := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	g, err := s.groupRepo.GetByID(ctx, gid)
	if err != nil || g.Status != "active" {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	_, _, err = s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: o.UserID, GroupID: gid, ValidityDays: days, AssignedBy: 0, Notes: fmt.Sprintf("payment order %d", o.ID)})
	if err != nil {
		return fmt.Errorf("assign subscription: %w", err)
	}
	now := time.Now()
	_, err = s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, "SUBSCRIPTION_SUCCESS", "system", map[string]any{"groupId": gid, "days": days, "amount": o.Amount})
	return nil
}

func (s *PaymentService) markFailed(ctx context.Context, oid int64, cause error) {
	now := time.Now()
	r := psErrMsg(cause)
	_, e := s.entClient.PaymentOrder.UpdateOneID(oid).SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason(r).Save(ctx)
	if e != nil {
		slog.Error("mark FAILED", "orderID", oid, "error", e)
	}
	s.writeAuditLog(ctx, oid, "FULFILLMENT_FAILED", "system", map[string]any{"reason": r})
}

func (s *PaymentService) RetryFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.PaidAt == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "order is not paid")
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot retry")
	}
	if o.Status == OrderStatusRecharging {
		return infraerrors.Conflict("CONFLICT", "order is being processed")
	}
	if o.Status == OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "order already completed")
	}
	if o.Status != OrderStatusFailed && o.Status != OrderStatusPaid {
		return infraerrors.BadRequest("INVALID_STATUS", "only paid and failed orders can retry")
	}
	_, err = s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.StatusIn(OrderStatusFailed, OrderStatusPaid)).SetStatus(OrderStatusPaid).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("reset for retry: %w", err)
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}

func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != "balance" {
		return infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u.Balance < o.Amount {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	nr := strings.TrimSpace(reason)
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ("balance")).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(by).SetRefundAmount(o.Amount).Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr})
	return nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	ok := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	if amt <= 0 {
		amt = o.Amount
	}
	if amt > o.Amount {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	ga := amt
	if amt == o.Amount {
		ga = o.PayAmount
	}
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	p := &RefundPlan{OrderID: oid, Order: o, RefundAmount: amt, GatewayAmount: ga, Reason: rr, Force: force, DeductBalance: deduct, DeductionType: "none"}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == "subscription" {
		p.DeductionType = "subscription"
		return nil
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = "balance"
	p.BalanceToDeduct = math.Min(p.RefundAmount, u.Balance)
	return nil
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusIn(OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed)).SetStatus(OrderStatusRefunding).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if p.DeductionType == "balance" && p.BalanceToDeduct > 0 {
		if err := s.userRepo.DeductBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			s.restoreStatus(ctx, p)
			return nil, fmt.Errorf("deduction: %w", err)
		}
	}
	if err := s.gwRefund(ctx, p); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.markRefundOk(ctx, p)
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) error {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_NO_TRADE_NO", "admin", map[string]any{"detail": "skipped"})
		return nil
	}
	prov, err := s.registry.GetProvider(p.Order.PaymentType)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	_, err = prov.Refund(ctx, payment.RefundRequest{TradeNo: p.Order.PaymentTradeNo, OrderID: strconv.FormatInt(p.Order.ID, 10), Amount: strconv.FormatFloat(p.GatewayAmount, 'f', 2, 64), Reason: p.Reason})
	return err
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s.RollbackRefund(ctx, p, gErr) {
		s.restoreStatus(ctx, p)
		s.writeAuditLog(ctx, p.OrderID, "REFUND_GATEWAY_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
		return &RefundResult{Success: false, Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back"}, nil
	}
	now := time.Now()
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundFailed).SetFailedAt(now).SetFailedReason(psErrMsg(gErr)).Save(ctx)
	s.writeAuditLog(ctx, p.OrderID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	fs := OrderStatusRefunded
	if p.RefundAmount < p.Order.Amount {
		fs = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	_, err := s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(fs).SetRefundAmount(p.RefundAmount).SetRefundReason(p.Reason).SetRefundAt(now).SetForceRefund(p.Force).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	s.writeAuditLog(ctx, p.OrderID, "REFUND_SUCCESS", "admin", map[string]any{"refundAmount": p.RefundAmount, "reason": p.Reason, "balanceDeducted": p.BalanceToDeduct, "force": p.Force})
	return &RefundResult{Success: true, BalanceDeducted: p.BalanceToDeduct, SubDaysDeducted: p.SubDaysToDeduct}, nil
}

func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == "balance" && p.BalanceToDeduct > 0 {
		if err := s.userRepo.UpdateBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "balanceDeducted": p.BalanceToDeduct})
			return false
		}
	}
	return true
}

func (s *PaymentService) restoreStatus(ctx context.Context, p *RefundPlan) {
	rs := OrderStatusCompleted
	if p.Order.Status == OrderStatusRefundRequested {
		rs = OrderStatusRefundRequested
	}
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(rs).Save(ctx)
}

func (s *PaymentService) ExpireTimedOutOrders(ctx context.Context) (int, error) {
	now := time.Now()
	orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.ExpiresAtLTE(now)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query expired: %w", err)
	}
	n := 0
	for _, o := range orders {
		c, e := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).SetStatus(OrderStatusExpired).Save(ctx)
		if e != nil {
			slog.Warn("expire failed", "orderID", o.ID, "error", e)
			continue
		}
		if c > 0 {
			s.writeAuditLog(ctx, o.ID, "ORDER_EXPIRED", "system", map[string]any{"expiresAt": o.ExpiresAt.Format(time.RFC3339)})
			n++
		}
	}
	return n, nil
}

func (s *PaymentService) GetDashboardStats(ctx context.Context, days int) (*DashboardStats, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	st := &DashboardStats{}
	var err error
	st.TotalOrders, err = s.entClient.PaymentOrder.Query().Where(paymentorder.CreatedAtGTE(since)).Count(ctx)
	if err != nil {
		return nil, err
	}
	st.PendingOrders, err = s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusPending), paymentorder.CreatedAtGTE(since)).Count(ctx)
	if err != nil {
		return nil, err
	}
	st.CompletedOrders, err = s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.CreatedAtGTE(since)).Count(ctx)
	if err != nil {
		return nil, err
	}
	st.FailedOrders, err = s.entClient.PaymentOrder.Query().Where(paymentorder.StatusEQ(OrderStatusFailed), paymentorder.CreatedAtGTE(since)).Count(ctx)
	if err != nil {
		return nil, err
	}
	st.TotalRevenue, err = s.sumAmt(ctx, []string{OrderStatusCompleted}, since, true)
	if err != nil {
		return nil, err
	}
	st.TotalRefunded, err = s.sumAmt(ctx, []string{OrderStatusRefunded, OrderStatusPartiallyRefunded}, since, false)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (s *PaymentService) sumAmt(ctx context.Context, statuses []string, since time.Time, usePaid bool) (float64, error) {
	q := s.entClient.PaymentOrder.Query().Where(paymentorder.StatusIn(statuses...))
	if usePaid {
		q = q.Where(paymentorder.PaidAtGTE(since))
	} else {
		q = q.Where(paymentorder.CreatedAtGTE(since))
	}
	os, err := q.All(ctx)
	if err != nil {
		return 0, err
	}
	var t float64
	for _, o := range os {
		if usePaid {
			t += o.Amount
		} else {
			t += o.RefundAmount
		}
	}
	return t, nil
}

func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(oid, 10)).SetAction(action).SetDetail(string(dj)).SetOperator(op).Save(ctx)
	if err != nil {
		slog.Error("audit log failed", "orderID", oid, "action", action, "error", err)
	}
}

func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*dbent.PaymentAuditLog, error) {
	return s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Order(paymentauditlog.ByCreatedAt()).All(ctx)
}

func psIsRefundStatus(s string) bool {
	switch s {
	case OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return true
	}
	return false
}

func psErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

func psComputeValidityDays(days int, unit string) int {
	switch unit {
	case "week":
		return days * 7
	case "month":
		return days * 30
	default:
		return days
	}
}

func (s *PaymentService) getFeeRate(_ string) float64 { return 0 }

func psStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// AdminListOrders returns a paginated list of orders. If userID > 0, filters by user.
func (s *PaymentService) AdminListOrders(ctx context.Context, userID int64, p OrderListParams) ([]*dbent.PaymentOrder, int, error) {
	q := s.entClient.PaymentOrder.Query()
	if userID > 0 {
		q = q.Where(paymentorder.UserIDEQ(userID))
	}
	if p.Status != "" {
		q = q.Where(paymentorder.StatusEQ(p.Status))
	}
	if p.OrderType != "" {
		q = q.Where(paymentorder.OrderTypeEQ(p.OrderType))
	}
	if p.PaymentType != "" {
		q = q.Where(paymentorder.PaymentTypeEQ(p.PaymentType))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}
	ps := p.PageSize
	if ps <= 0 {
		ps = 20
	}
	if ps > 100 {
		ps = 100
	}
	pg := p.Page
	if pg < 1 {
		pg = 1
	}
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	return orders, total, nil
}
