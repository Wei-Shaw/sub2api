package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	"github.com/shopspring/decimal"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrOrderNotFound is returned by HandlePaymentNotification when the webhook
// references an out_trade_no that does not exist in our DB. Callers (webhook
// handlers) should treat this as a terminal, non-retryable condition and still
// respond with a 2xx success to the provider — otherwise the provider will keep
// retrying forever (e.g. when a foreign environment's webhook endpoint is
// misconfigured to point at us, or when our orders table has been wiped).
var ErrOrderNotFound = errors.New("payment order not found")

const paymentFulfillmentLeaseDuration = 5 * time.Minute

type paymentFulfillmentLease struct {
	version time.Time
}

// --- Payment Notification & Fulfillment ---

func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	if n.Status != payment.NotificationStatusSuccess {
		return nil
	}
	// Look up order by out_trade_no (the external order ID we sent to the provider)
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		// Fallback only for true legacy "sub2_N" DB-ID payloads when the
		// current out_trade_no lookup genuinely did not find an order.
		if oid, ok := parseLegacyPaymentOrderID(n.OrderID, err); ok {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk, n.Metadata)
		}
		if dbent.IsNotFound(err) {
			return fmt.Errorf("%w: out_trade_no=%s", ErrOrderNotFound, n.OrderID)
		}
		return fmt.Errorf("lookup order failed for out_trade_no %s: %w", n.OrderID, err)
	}
	return s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, pk, n.Metadata)
}

func parseLegacyPaymentOrderID(orderID string, lookupErr error) (int64, bool) {
	if !dbent.IsNotFound(lookupErr) {
		return 0, false
	}
	orderID = strings.TrimSpace(orderID)
	if !strings.HasPrefix(orderID, orderIDPrefix) {
		return 0, false
	}
	trimmed := strings.TrimPrefix(orderID, orderIDPrefix)
	if trimmed == "" || trimmed == orderID {
		return 0, false
	}
	oid, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || oid <= 0 {
		return 0, false
	}
	return oid, true
}

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid decimal.Decimal, pk string, metadata map[string]string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		slog.Error("order not found", "orderID", oid)
		return nil
	}
	instanceProviderKey := ""
	if inst, instErr := s.getOrderProviderInstance(ctx, o); instErr == nil && inst != nil {
		instanceProviderKey = inst.ProviderKey
	}
	expectedProviderKey := expectedNotificationProviderKeyForOrder(s.registry, o, instanceProviderKey)
	if expectedProviderKey != "" && strings.TrimSpace(pk) != "" && !strings.EqualFold(expectedProviderKey, strings.TrimSpace(pk)) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_MISMATCH", pk, map[string]any{
			"expectedProvider": expectedProviderKey,
			"actualProvider":   pk,
			"tradeNo":          tradeNo,
		})
		return fmt.Errorf("provider mismatch: expected %s, got %s", expectedProviderKey, pk)
	}
	if err := validateProviderNotificationMetadata(o, pk, metadata); err != nil {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_METADATA_MISMATCH", pk, map[string]any{
			"detail":  err.Error(),
			"tradeNo": tradeNo,
		})
		return err
	}
	if !isValidProviderAmount(paid) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_INVALID_AMOUNT", pk, map[string]any{
			"expected": o.PayAmount,
			"paid":     paid.String(),
			"tradeNo":  tradeNo,
		})
		return fmt.Errorf("invalid paid amount from provider: %s", paid.String())
	}
	expected := decimal.NewFromFloat(o.PayAmount)
	if shortfall := expected.Sub(paid); shortfall.IsPositive() {
		// 少付一律不予履约：金额全程是十进制精确值，没有需要吸收的表示误差，
		// 旧的 0.01 容差纯粹是 float64 时代的产物，副作用是 100.00 的订单
		// 收到 99.99 也照样足额到账。
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{
			"expected": o.PayAmount, "paid": paid.String(), "shortfall": shortfall.String(), "tradeNo": tradeNo,
		})
		return fmt.Errorf("amount mismatch: expected %s, got %s", expected.String(), paid.String())
	}
	if overpay := paid.Sub(expected); overpay.GreaterThan(paymentOverpayToleranceForCurrency(PaymentOrderCurrency(o))) {
		// 多付超过容差同样拒绝（与既有行为一致）：多半是币种或实例配错了，
		// 静默按订单金额入账会掩盖配置问题。容差仍按币种保留，是留给那些
		// 确实会做进位的服务商的口子。
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{
			"expected": o.PayAmount, "paid": paid.String(), "overpay": overpay.String(), "tradeNo": tradeNo,
		})
		return fmt.Errorf("amount mismatch: expected %s, got %s", expected.String(), paid.String())
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

// paymentAmountToleranceForCurrency 退款侧仍在用的浮点容差（见 payment_amounts.go /
// payment_refund.go）。收款侧改用 paymentOverpayToleranceForCurrency。
func paymentAmountToleranceForCurrency(currency string) float64 {
	minorUnit := payment.CurrencyMinorUnit(currency)
	if minorUnit <= 2 {
		return amountToleranceCNY
	}
	return math.Pow10(-minorUnit) / 2
}

// paymentOverpayToleranceForCurrency 只作用于「多付」方向：少付零容差。
func paymentOverpayToleranceForCurrency(currency string) decimal.Decimal {
	minorUnit := payment.CurrencyMinorUnit(currency)
	if minorUnit <= 2 {
		return decimalAmountToleranceCNY
	}
	// 半个最小货币单位：给按最小单位进位的服务商留出余地。
	return decimal.New(5, int32(-minorUnit-1))
}

var decimalAmountToleranceCNY = decimal.NewFromFloat(amountToleranceCNY)

// isValidProviderAmount 金额必须为正。decimal 不存在 NaN/Inf，无需额外判定。
func isValidProviderAmount(amount decimal.Decimal) bool {
	return amount.IsPositive()
}

func validateProviderNotificationMetadata(order *dbent.PaymentOrder, providerKey string, metadata map[string]string) error {
	return validateProviderSnapshotMetadata(order, providerKey, metadata)
}

func expectedNotificationProviderKey(registry *payment.Registry, orderPaymentType string, orderProviderKey string, instanceProviderKey string) string {
	if key := strings.TrimSpace(instanceProviderKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(orderProviderKey); key != "" {
		return key
	}
	if registry != nil {
		if key := strings.TrimSpace(registry.GetProviderKey(payment.PaymentType(orderPaymentType))); key != "" {
			return key
		}
	}
	return strings.TrimSpace(orderPaymentType)
}

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid decimal.Decimal, pk string) error {
	previousStatus := o.Status
	now := time.Now()
	grace := now.Add(-paymentGraceMinutes * time.Minute)
	c, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(o.ID),
		paymentorder.Or(
			paymentorder.StatusEQ(OrderStatusPending),
			paymentorder.StatusEQ(OrderStatusCancelled),
			paymentorder.And(
				paymentorder.StatusEQ(OrderStatusExpired),
				paymentorder.UpdatedAtGTE(grace),
			),
		),
	).SetStatus(OrderStatusPaid).SetPayAmount(paid.InexactFloat64()).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o)
	}
	if previousStatus == OrderStatusCancelled || previousStatus == OrderStatusExpired {
		slog.Info("order recovered from webhook payment success",
			"orderID", o.ID,
			"previousStatus", previousStatus,
			"tradeNo", tradeNo,
			"provider", pk,
		)
		s.writeAuditLog(ctx, o.ID, "ORDER_RECOVERED", pk, map[string]any{
			"previous_status": previousStatus,
			"tradeNo":         tradeNo,
			"paidAmount":      paid.String(),
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid.String()})
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
	case OrderStatusFailed, OrderStatusPaid, OrderStatusRecharging:
		return s.executeFulfillment(ctx, o.ID)
	case OrderStatusExpired:
		slog.Warn("webhook payment success for expired order beyond grace period",
			"orderID", o.ID,
			"status", cur.Status,
			"updatedAt", cur.UpdatedAt,
		)
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AFTER_EXPIRY", "system", map[string]any{
			"status":    cur.Status,
			"updatedAt": cur.UpdatedAt,
			"reason":    "payment arrived after expiry grace period",
		})
		return nil
	default:
		return nil
	}
}

func (s *PaymentService) executeFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if o.OrderType == payment.OrderTypeSubscription {
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
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	lease, err := s.acquirePaymentFulfillmentLease(ctx, o)
	if err != nil {
		return err
	}
	if lease == nil {
		return nil
	}
	if err := s.doBalance(ctx, o, lease); err != nil {
		s.markFailed(ctx, oid, lease, err)
		return err
	}
	return nil
}

func (s *PaymentService) acquirePaymentFulfillmentLease(ctx context.Context, o *dbent.PaymentOrder) (*paymentFulfillmentLease, error) {
	if o == nil {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "nil payment order")
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-paymentFulfillmentLeaseDuration)
	updated, err := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(o.ID),
			paymentorder.Or(
				paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed),
				paymentorder.And(
					paymentorder.StatusEQ(OrderStatusRecharging),
					paymentorder.UpdatedAtLTE(staleBefore),
				),
			),
		).
		SetStatus(OrderStatusRecharging).
		SetUpdatedAt(now).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire fulfillment lease: %w", err)
	}
	if updated == 0 {
		current, getErr := s.entClient.PaymentOrder.Get(ctx, o.ID)
		if getErr != nil {
			return nil, fmt.Errorf("reload fulfillment lease: %w", getErr)
		}
		if current.Status == OrderStatusCompleted {
			return nil, nil
		}
		if current.Status == OrderStatusRecharging {
			return nil, infraerrors.Conflict("CONFLICT", "order is being processed")
		}
		return nil, infraerrors.Conflict("CONFLICT", "order status changed while acquiring fulfillment lease")
	}

	// Reload the persisted timestamp instead of trusting application clock precision.
	claimed, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("reload acquired fulfillment lease: %w", err)
	}
	if claimed.Status != OrderStatusRecharging {
		return nil, infraerrors.Conflict("CONFLICT", "fulfillment lease was lost")
	}
	return &paymentFulfillmentLease{version: claimed.UpdatedAt}, nil
}

// redeemAction represents the idempotency decision for balance fulfillment.
type redeemAction int

const (
	// redeemActionCreate: code does not exist — create it, then redeem.
	redeemActionCreate redeemAction = iota
	// redeemActionRedeem: code exists but is unused — skip creation, redeem only.
	redeemActionRedeem
	// redeemActionSkipCompleted: code exists and is already used — skip to mark completed.
	redeemActionSkipCompleted
)

// resolveRedeemAction decides the idempotency action based on an existing redeem code lookup.
// existing is the result of GetByCode; lookupErr is the error from that call.
func resolveRedeemAction(existing *RedeemCode, lookupErr error) redeemAction {
	if existing == nil || lookupErr != nil {
		return redeemActionCreate
	}
	if existing.IsUsed() {
		return redeemActionSkipCompleted
	}
	return redeemActionRedeem
}

// doBalance 用订单上预生成的兑换码给用户加余额。
//
// 创建兑换码与兑换在同一个事务内完成（见 createAndRedeemBalanceCode）：
// 两者分属两个事务时，中间崩溃会留下一条 unused 的孤儿充值码。虽然
// o.RechargeCode 是 130 bit crypto/rand（见 generateRechargeCode）、
// 用户侧接口不返回，枚举不可行，但那仍是一条能被兑换却没有对应入账的活码。
//
// 幂等性判定（GetByCode）刻意留在事务外：它是一次只读探测，
// redeemActionSkipCompleted 分支不需要开事务；若与并发履约撞车，
// Create 会撞唯一约束、整笔事务回滚，下一次重试按「存在且未使用」自愈。
func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	// Idempotency: check if redeem code already exists (from a previous partial run)
	existing, lookupErr := s.redeemService.GetByCode(ctx, o.RechargeCode)
	action := resolveRedeemAction(existing, lookupErr)

	if action == redeemActionSkipCompleted {
		if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
			return err
		}
		// Code already created and redeemed — just mark completed
		return s.markCompleted(ctx, o, lease, "RECHARGE_SUCCESS")
	}

	if err := s.createAndRedeemBalanceCode(ctx, o, action == redeemActionCreate); err != nil {
		return err
	}
	if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, lease, "RECHARGE_SUCCESS")
}

// createAndRedeemBalanceCode 在一个事务内创建（可选）并兑换充值码。
//
// RedeemService.Redeem 会复用 context 里的事务，所以标记兑换码已用与给用户加钱
// 和这里的 Create 同属一个事务；崩溃时整笔回滚，不留下 unused 的孤儿码。
// 缓存失效与邀请返利由 ContextWithRedeemPostCommit 收集，Commit 成功后才执行。
func (s *PaymentService) createAndRedeemBalanceCode(ctx context.Context, o *dbent.PaymentOrder, needCreate bool) error {
	if s.entClient == nil {
		return errors.New("payment service is not configured with a database client")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin balance fulfillment transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txCtx, runPostCommit := ContextWithRedeemPostCommit(dbent.NewTxContext(ctx, tx))
	if needCreate {
		rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(txCtx, rc); err != nil {
			return fmt.Errorf("create redeem code: %w", err)
		}
	}
	if _, err := s.redeemService.Redeem(ContextSkipRedeemAffiliate(txCtx), o.UserID, o.RechargeCode); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit balance fulfillment transaction: %w", err)
	}
	committed = true

	runPostCommit(ctx)
	return nil
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease, auditAction string) error {
	if lease == nil {
		return errors.New("missing payment fulfillment lease")
	}
	now := time.Now()
	updated, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(o.ID),
		paymentorder.StatusEQ(OrderStatusRecharging),
		paymentorder.UpdatedAtEQ(lease.version),
	).SetStatus(OrderStatusCompleted).SetCompletedAt(now).Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	if updated == 0 {
		current, getErr := s.entClient.PaymentOrder.Get(ctx, o.ID)
		if getErr == nil && current.Status == OrderStatusCompleted {
			return nil
		}
		return infraerrors.Conflict("CONFLICT", "fulfillment lease was lost before completion")
	}
	if !s.hasAuditLog(ctx, o.ID, auditAction) {
		s.writeAuditLog(ctx, o.ID, auditAction, "system", map[string]any{
			"rechargeCode":   o.RechargeCode,
			"creditedAmount": o.Amount,
			"payAmount":      o.PayAmount,
		})
		s.dispatchPaymentFulfillmentNotification(o, auditAction)
	}
	return nil
}

func (s *PaymentService) dispatchPaymentFulfillmentNotification(o *dbent.PaymentOrder, auditAction string) {
	if s == nil || s.notificationEmailService == nil || o == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()
		var err error
		switch auditAction {
		case "RECHARGE_SUCCESS":
			err = s.sendBalanceRechargeSuccessNotification(ctx, o)
		case "SUBSCRIPTION_SUCCESS":
			err = s.sendSubscriptionPurchaseSuccessNotification(ctx, o)
		default:
			return
		}
		if err != nil {
			slog.Warn("payment fulfillment notification email failed", "order_id", o.ID, "action", auditAction, "err", err.Error())
		}
	}()
}

func (s *PaymentService) sendBalanceRechargeSuccessNotification(ctx context.Context, o *dbent.PaymentOrder) error {
	currentBalance := ""
	if s.userRepo != nil {
		if user, err := s.userRepo.GetByID(ctx, o.UserID); err == nil && user != nil {
			currentBalance = fmt.Sprintf("%.2f", user.Balance)
		}
	}
	return s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceRechargeSuccess,
		RecipientEmail: o.UserEmail,
		RecipientName:  firstNonEmpty(o.UserName, o.UserEmail),
		UserID:         o.UserID,
		SourceType:     "payment_order",
		SourceID:       strconv.FormatInt(o.ID, 10),
		Variables: map[string]string{
			"recharge_amount": fmt.Sprintf("%.2f", o.Amount),
			"current_balance": currentBalance,
			"order_id":        strconv.FormatInt(o.ID, 10),
		},
	})
}

func (s *PaymentService) sendSubscriptionPurchaseSuccessNotification(ctx context.Context, o *dbent.PaymentOrder) error {
	variables := map[string]string{
		"subscription_group": "Subscription",
		"subscription_days":  "",
		"expiry_time":        "",
		"order_id":           strconv.FormatInt(o.ID, 10),
	}
	if o.SubscriptionDays != nil {
		variables["subscription_days"] = strconv.Itoa(*o.SubscriptionDays)
	}
	if o.SubscriptionGroupID != nil {
		if s.groupRepo != nil {
			if group, err := s.groupRepo.GetByID(ctx, *o.SubscriptionGroupID); err == nil && group != nil && strings.TrimSpace(group.Name) != "" {
				variables["subscription_group"] = group.Name
			}
		}
		if s.subscriptionSvc != nil {
			if sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID); err == nil && sub != nil {
				variables["expiry_time"] = sub.ExpiresAt.Format("2006-01-02 15:04")
			}
		}
	}
	return s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionPurchaseSuccess,
		RecipientEmail: o.UserEmail,
		RecipientName:  firstNonEmpty(o.UserName, o.UserEmail),
		UserID:         o.UserID,
		SourceType:     "payment_order",
		SourceID:       strconv.FormatInt(o.ID, 10),
		Variables:      variables,
	})
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
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	lease, err := s.acquirePaymentFulfillmentLease(ctx, o)
	if err != nil {
		return err
	}
	if lease == nil {
		return nil
	}
	if err := s.doSub(ctx, o, lease); err != nil {
		s.markFailed(ctx, oid, lease, err)
		return err
	}
	return nil
}

func (s *PaymentService) doSub(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	gid := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	g, err := s.groupRepo.GetByID(ctx, gid)
	if err != nil || g.Status != payment.EntityStatusActive {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	if err := s.ensurePaymentSubscriptionAssigned(ctx, o, gid, days); err != nil {
		return err
	}
	if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, lease, "SUBSCRIPTION_SUCCESS")
}

func (s *PaymentService) ensurePaymentSubscriptionAssigned(ctx context.Context, o *dbent.PaymentOrder, groupID int64, days int) error {
	if s.subscriptionSvc == nil {
		return errors.New("subscription service is unavailable")
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription fulfillment tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	txClient := tx.Client()
	alreadyAssigned, err := hasPaymentSubscriptionAssignmentAudit(txCtx, txClient, o.ID)
	if err != nil {
		return fmt.Errorf("check subscription assignment audit: %w", err)
	}

	recoveredFromNote := false
	if !alreadyAssigned {
		orderNote := paymentSubscriptionOrderNote(o.ID)
		existing, lookupErr := s.subscriptionSvc.userSubRepo.GetByUserIDAndGroupID(txCtx, o.UserID, groupID)
		switch {
		case lookupErr == nil && existing != nil && hasPaymentSubscriptionOrderNote(existing.Notes, orderNote):
			recoveredFromNote = true
		case lookupErr != nil && !errors.Is(lookupErr, ErrSubscriptionNotFound):
			return fmt.Errorf("check existing subscription assignment: %w", lookupErr)
		default:
			if _, _, err := s.subscriptionSvc.assignOrExtendSubscription(txCtx, &AssignSubscriptionInput{
				UserID:       o.UserID,
				GroupID:      groupID,
				ValidityDays: days,
				AssignedBy:   0,
				Notes:        orderNote,
			}, true); err != nil {
				return fmt.Errorf("assign subscription: %w", err)
			}
		}

		detail, _ := json.Marshal(map[string]any{
			"groupID":           groupID,
			"validityDays":      days,
			"recoveredFromNote": recoveredFromNote,
		})
		if _, err := txClient.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(o.ID, 10)).
			SetAction("SUBSCRIPTION_ASSIGNED").
			SetDetail(string(detail)).
			SetOperator("system").
			Save(txCtx); err != nil {
			if dbent.IsConstraintError(err) {
				_ = tx.Rollback()
				claimed, checkErr := hasPaymentSubscriptionAssignmentAudit(ctx, s.entClient, o.ID)
				if checkErr == nil && claimed {
					return s.subscriptionSvc.invalidateSubscriptionCaches(o.UserID, groupID)
				}
			}
			return fmt.Errorf("record subscription assignment audit: %w", err)
		}
	} else {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", groupID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscription fulfillment tx: %w", err)
	}
	// Assignment cache invalidation is deferred while this transaction is open,
	// then performed synchronously against the committed subscription.
	if err := s.subscriptionSvc.invalidateSubscriptionCaches(o.UserID, groupID); err != nil {
		return fmt.Errorf("invalidate subscription cache after fulfillment: %w", err)
	}
	return nil
}

func hasPaymentSubscriptionAssignmentAudit(ctx context.Context, client *dbent.Client, orderID int64) (bool, error) {
	count, err := client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionIn("SUBSCRIPTION_ASSIGNED", "SUBSCRIPTION_SUCCESS"),
		).
		Limit(1).
		Count(ctx)
	return count > 0, err
}

func paymentSubscriptionOrderNote(orderID int64) string {
	return fmt.Sprintf("payment order %d", orderID)
}

func hasPaymentSubscriptionOrderNote(notes string, orderNote string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == orderNote {
			return true
		}
	}
	return false
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	oid := strconv.FormatInt(orderID, 10)
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0
}

func (s *PaymentService) applyAffiliateRebateForOrder(ctx context.Context, o *dbent.PaymentOrder) error {
	baseAmount := affiliateRebateBaseAmount(o)
	if o == nil || baseAmount <= 0 {
		return nil
	}
	if s.affiliateService == nil {
		return nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": fmt.Sprintf("begin affiliate rebate tx: %v", err),
		})
		return fmt.Errorf("begin affiliate rebate tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	claimed, err := s.tryClaimAffiliateRebateAudit(txCtx, tx.Client(), o.ID, baseAmount)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("claim affiliate rebate audit: %w", err)
	}
	if !claimed {
		return nil
	}

	sourceOrderID := o.ID
	rebateAmount, err := s.affiliateService.AccrueInviteRebateForOrder(txCtx, o.UserID, baseAmount, &sourceOrderID)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("accrue affiliate rebate: %w", err)
	}

	if rebateAmount <= 0 {
		if err := s.updateClaimedAffiliateRebateAudit(txCtx, tx.Client(), o.ID, "AFFILIATE_REBATE_SKIPPED", map[string]any{
			"baseAmount": baseAmount,
			"reason":     "no inviter bound or rebate amount <= 0",
		}); err != nil {
			s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("update affiliate rebate skipped audit: %w", err)
		}
		if err := tx.Commit(); err != nil {
			s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
				"error": fmt.Sprintf("commit affiliate rebate tx: %v", err),
			})
			return fmt.Errorf("commit affiliate rebate tx: %w", err)
		}
		return nil
	}

	if err := s.updateClaimedAffiliateRebateAudit(txCtx, tx.Client(), o.ID, "AFFILIATE_REBATE_APPLIED", map[string]any{
		"baseAmount":   baseAmount,
		"rebateAmount": rebateAmount,
	}); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("update affiliate rebate applied audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_FAILED", "system", map[string]any{
			"error": fmt.Sprintf("commit affiliate rebate tx: %v", err),
		})
		return fmt.Errorf("commit affiliate rebate tx: %w", err)
	}
	return nil
}

func affiliateRebateBaseAmount(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	switch o.OrderType {
	case payment.OrderTypeBalance, payment.OrderTypeSubscription:
		return o.Amount
	default:
		return 0
	}
}

func (s *PaymentService) tryClaimAffiliateRebateAudit(ctx context.Context, client *dbent.Client, orderID int64, baseAmount float64) (bool, error) {
	if client == nil {
		return false, errors.New("nil payment client")
	}
	oid := strconv.FormatInt(orderID, 10)
	detail, _ := json.Marshal(map[string]any{
		"baseAmount": baseAmount,
		"status":     "reserved",
	})
	query, args := buildAffiliateRebateAuditClaimQuery(client, oid, string(detail))
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var claimID int64
	if err := rows.Scan(&claimID); err != nil {
		return false, err
	}
	return true, nil
}

func buildAffiliateRebateAuditClaimQuery(client *dbent.Client, orderID, detail string) (string, []any) {
	nowExpr := paymentAuditCurrentTimestampExpr(client)
	if paymentAuditDialect(client) == dialect.Postgres {
		return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT $1::text, 'AFFILIATE_REBATE_APPLIED', $2::text, 'system', %s
WHERE NOT EXISTS (
	SELECT 1
	FROM payment_audit_logs
	WHERE order_id = $1::text
	  AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail}
	}
	return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
SELECT ?, 'AFFILIATE_REBATE_APPLIED', ?, 'system', %s
WHERE NOT EXISTS (
	SELECT 1
	FROM payment_audit_logs
	WHERE order_id = ?
	  AND action IN ('AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')
)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail, orderID}
}

func paymentAuditCurrentTimestampExpr(client *dbent.Client) string {
	if paymentAuditDialect(client) == dialect.Postgres {
		return "NOW()"
	}
	return "CURRENT_TIMESTAMP"
}

func paymentAuditDialect(client *dbent.Client) string {
	if client == nil || client.Driver() == nil {
		return ""
	}
	return client.Driver().Dialect()
}

func (s *PaymentService) updateClaimedAffiliateRebateAudit(ctx context.Context, client *dbent.Client, orderID int64, action string, detail map[string]any) error {
	if client == nil {
		return errors.New("nil payment client")
	}
	oid := strconv.FormatInt(orderID, 10)
	detailJSON, _ := json.Marshal(detail)
	updated, err := client.PaymentAuditLog.Update().
		Where(
			paymentauditlog.OrderIDEQ(oid),
			paymentauditlog.ActionEQ("AFFILIATE_REBATE_APPLIED"),
		).
		SetAction(action).
		SetDetail(string(detailJSON)).
		SetOperator("system").
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return errors.New("affiliate rebate claim log not found")
	}
	return nil
}

func (s *PaymentService) markFailed(ctx context.Context, oid int64, lease *paymentFulfillmentLease, cause error) {
	if lease == nil {
		slog.Error("mark FAILED without fulfillment lease", "orderID", oid)
		return
	}
	now := time.Now()
	r := psErrMsg(cause)
	// The lease version prevents a stale worker from overwriting a newer owner.
	c, e := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(oid),
			paymentorder.StatusEQ(OrderStatusRecharging),
			paymentorder.UpdatedAtEQ(lease.version),
		).
		SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason(r).Save(ctx)
	if e != nil {
		slog.Error("mark FAILED", "orderID", oid, "error", e)
	}
	if c > 0 {
		s.writeAuditLog(ctx, oid, "FULFILLMENT_FAILED", "system", map[string]any{"reason": r})
	}
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
	if o.Status == OrderStatusCompleted {
		return infraerrors.BadRequest("INVALID_STATUS", "order already completed")
	}
	if o.Status != OrderStatusFailed && o.Status != OrderStatusPaid && o.Status != OrderStatusRecharging {
		return infraerrors.BadRequest("INVALID_STATUS", "only paid, failed, and recoverable recharging orders can retry")
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}
