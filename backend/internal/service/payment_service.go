package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OrderStatusPending           = payment.OrderStatusPending
	OrderStatusPaid              = payment.OrderStatusPaid
	OrderStatusRecharging        = payment.OrderStatusRecharging
	OrderStatusCompleted         = payment.OrderStatusCompleted
	OrderStatusExpired           = payment.OrderStatusExpired
	OrderStatusCancelled         = payment.OrderStatusCancelled
	OrderStatusFailed            = payment.OrderStatusFailed
	OrderStatusRefundRequested   = payment.OrderStatusRefundRequested
	OrderStatusRefunding         = payment.OrderStatusRefunding
	OrderStatusPartiallyRefunded = payment.OrderStatusPartiallyRefunded
	OrderStatusRefunded          = payment.OrderStatusRefunded
	OrderStatusRefundFailed      = payment.OrderStatusRefundFailed
)

const (
	// defaultMaxPendingOrders and defaultOrderTimeoutMin are defined in
	// payment_config_service.go alongside other payment configuration defaults.
	paymentGraceMinutes = 5

	defaultPageSize    = 20
	maxPageSize        = 100
	topUsersLimit      = 10
	amountToleranceCNY = 0.01

	orderIDPrefix = "sub2_"
)

// formatOrderID converts an internal DB order ID to the external order ID sent to payment providers.
func formatOrderID(id int64) string {
	return orderIDPrefix + strconv.FormatInt(id, 10)
}

// parseOrderID extracts the internal DB order ID from an external order ID returned by payment providers.
func parseOrderID(externalID string) (int64, error) {
	trimmed := strings.TrimPrefix(externalID, orderIDPrefix)
	return strconv.ParseInt(trimmed, 10, 64)
}

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
	TodayAmount   float64 `json:"today_amount"`
	TotalAmount   float64 `json:"total_amount"`
	TodayCount    int     `json:"today_count"`
	TotalCount    int     `json:"total_count"`
	AvgAmount     float64 `json:"avg_amount"`
	PendingOrders int     `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       []TopUserStat       `json:"top_users"`
}

type DailyStats struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

type PaymentMethodStat struct {
	Type   string  `json:"type"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

type TopUserStat struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Amount float64 `json:"amount"`
}

type PaymentService struct {
	providerMu      sync.Mutex
	providerOnce    sync.Once
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
		tm = defaultOrderTimeoutMin
	}
	exp := time.Now().Add(time.Duration(tm) * time.Minute)
	b := tx.PaymentOrder.Create().
		SetUserID(req.UserID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(amount).
		SetPayAmount(payAmount).
		SetFeeRate(feeRate).
		SetRechargeCode("").
		SetPaymentType(req.PaymentType).
		SetPaymentTradeNo("").
		SetOrderType(req.OrderType).
		SetStatus(OrderStatusPending).
		SetExpiresAt(exp).
		SetClientIP(req.ClientIP).
		SetSrcHost(req.SrcHost)
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
	s.EnsureProviders(ctx)
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
	pr, err := provider.CreatePayment(ctx, payment.CreatePaymentRequest{OrderID: formatOrderID(order.ID), Amount: payAmountStr, PaymentType: req.PaymentType, Subject: subject, ClientIP: req.ClientIP, IsMobile: req.IsMobile})
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment method is temporarily unavailable")
	}
	_, err = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetNillablePaymentTradeNo(psNilIfEmpty(pr.TradeNo)).SetNillablePayURL(psNilIfEmpty(pr.PayURL)).SetNillableQrCode(psNilIfEmpty(pr.QRCode)).SetNillableProviderInstanceID(psNilIfEmpty(sel.InstanceID)).Save(ctx)
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
	ps, pg := applyPagination(p.PageSize, p.Page)
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
	s.EnsureProviders(ctx)
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
		_ = s.HandlePaymentNotification(ctx, &payment.PaymentNotification{TradeNo: o.PaymentTradeNo, OrderID: formatOrderID(o.ID), Amount: resp.Amount, Status: "success"}, prov.ProviderKey())
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
	oid, err := parseOrderID(n.OrderID)
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
	if math.Abs(paid-o.PayAmount) > amountToleranceCNY {
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
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
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

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != "balance" {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	return o, nil
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
	s.EnsureProviders(ctx)
	prov, err := s.registry.GetProvider(p.Order.PaymentType)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	_, err = prov.Refund(ctx, payment.RefundRequest{TradeNo: p.Order.PaymentTradeNo, OrderID: formatOrderID(p.Order.ID), Amount: strconv.FormatFloat(p.GatewayAmount, 'f', 2, 64), Reason: p.Reason})
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
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	paidStatuses := []string{OrderStatusCompleted, OrderStatusPaid, OrderStatusRecharging}

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(since),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	st := &DashboardStats{}
	computeBasicStats(st, orders, todayStart)

	st.PendingOrders, err = s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	st.DailySeries = buildDailySeries(orders, since, days)
	st.PaymentMethods = buildMethodDistribution(orders)
	st.TopUsers = buildTopUsers(orders)

	return st, nil
}

func computeBasicStats(st *DashboardStats, orders []*dbent.PaymentOrder, todayStart time.Time) {
	var totalAmount, todayAmount float64
	var todayCount int
	for _, o := range orders {
		totalAmount += o.PayAmount
		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			todayAmount += o.PayAmount
			todayCount++
		}
	}
	st.TotalAmount = math.Round(totalAmount*100) / 100
	st.TodayAmount = math.Round(todayAmount*100) / 100
	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	if st.TotalCount > 0 {
		st.AvgAmount = math.Round(totalAmount/float64(st.TotalCount)*100) / 100
	}
}

func buildDailySeries(orders []*dbent.PaymentOrder, since time.Time, days int) []DailyStats {
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		date := o.PaidAt.Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date}
			dailyMap[date] = ds
		}
		ds.Amount += o.PayAmount
		ds.Count++
	}
	series := make([]DailyStats, 0, days)
	for i := 0; i < days; i++ {
		date := since.AddDate(0, 0, i+1).Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			ds.Amount = math.Round(ds.Amount*100) / 100
			series = append(series, *ds)
		} else {
			series = append(series, DailyStats{Date: date})
		}
	}
	return series
}

func buildMethodDistribution(orders []*dbent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		ms, ok := methodMap[o.PaymentType]
		if !ok {
			ms = &PaymentMethodStat{Type: o.PaymentType}
			methodMap[o.PaymentType] = ms
		}
		ms.Amount += o.PayAmount
		ms.Count++
	}
	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, ms := range methodMap {
		ms.Amount = math.Round(ms.Amount*100) / 100
		methods = append(methods, *ms)
	}
	return methods
}

func buildTopUsers(orders []*dbent.PaymentOrder) []TopUserStat {
	userMap := make(map[int64]*TopUserStat)
	for _, o := range orders {
		us, ok := userMap[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmail}
			userMap[o.UserID] = us
		}
		us.Amount += o.PayAmount
	}
	userList := make([]*TopUserStat, 0, len(userMap))
	for _, us := range userMap {
		us.Amount = math.Round(us.Amount*100) / 100
		userList = append(userList, us)
	}
	sort.Slice(userList, func(i, j int) bool {
		return userList[i].Amount > userList[j].Amount
	})
	limit := topUsersLimit
	if len(userList) < limit {
		limit = len(userList)
	}
	result := make([]TopUserStat, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, *userList[i])
	}
	return result
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

func applyPagination(pageSize, page int) (size, pg int) {
	size = pageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	pg = page
	if pg < 1 {
		pg = 1
	}
	return size, pg
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
	ps, pg := applyPagination(p.PageSize, p.Page)
	orders, err := q.Order(dbent.Desc(paymentorder.FieldCreatedAt)).Limit(ps).Offset((pg - 1) * ps).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	return orders, total, nil
}

// EnsureProviders lazily initializes the provider registry on first call.
// It queries all enabled PaymentProviderInstance records, decrypts their config,
// creates providers via provider.CreateProvider, and registers them.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	s.providerOnce.Do(func() {
		s.loadProviders(ctx)
	})
}

// RefreshProviders clears and re-registers all providers from the database.
// Call this when provider instances are created, updated, or deleted.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.registry.Clear()
	s.loadProviders(ctx)
	s.providerOnce = sync.Once{} // reset so next EnsureProviders is a no-op until next Refresh
	s.providerOnce.Do(func() {}) // mark as done since we just loaded
}

func (s *PaymentService) loadProviders(ctx context.Context) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		slog.Error("[PaymentService] failed to query provider instances", "error", err)
		return
	}
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			slog.Warn("[PaymentService] failed to decrypt config for instance", "instanceID", inst.ID, "error", err)
			continue
		}
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			slog.Warn("[PaymentService] failed to create provider for instance", "instanceID", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
		}
		s.registry.Register(p)
	}
	slog.Info("[PaymentService] registered payment types", "types", len(s.registry.SupportedTypes()), "instances", len(instances))
}
