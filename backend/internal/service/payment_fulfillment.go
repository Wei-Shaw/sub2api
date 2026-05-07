package service

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/moneyx"
	"github.com/shopspring/decimal"
)

type paymentSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (stdsql.Result, error)
	QueryContext(context.Context, string, ...any) (*stdsql.Rows, error)
}

// ErrOrderNotFound is returned by HandlePaymentNotification when the webhook
// references an out_trade_no that does not exist in our DB. Callers (webhook
// handlers) should treat this as a terminal, non-retryable condition and still
// respond with a 2xx success to the provider — otherwise the provider will keep
// retrying forever (e.g. when a foreign environment's webhook endpoint is
// misconfigured to point at us, or when our orders table has been wiped).
var ErrOrderNotFound = errors.New("payment order not found")

const (
	customReferralOrderCommissionStatusPending   = "pending"
	customReferralOrderCommissionStatusSucceeded = "succeeded"
	customReferralOrderCommissionStatusSkipped   = "skipped"
	customReferralOrderCommissionStatusFailed    = "failed"
)

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

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string, metadata map[string]string) error {
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
			"paid":     paid,
			"tradeNo":  tradeNo,
		})
		return fmt.Errorf("invalid paid amount from provider: %v", paid)
	}
	if !moneyx.Equal(paid, o.PayAmount, moneyx.ScaleCurrency) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
		return fmt.Errorf("amount mismatch: expected %.2f, got %.2f", o.PayAmount, paid)
	}
	if reused, err := s.paymentTradeNoUsedByAnotherOrder(ctx, o.ID, tradeNo); err != nil {
		return err
	} else if reused {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_TRADE_NO_REUSED", pk, map[string]any{
			"tradeNo": tradeNo,
		})
		return fmt.Errorf("payment trade_no %s is already bound to another order", tradeNo)
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func isValidProviderAmount(amount float64) bool {
	return amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

func (s *PaymentService) paymentTradeNoUsedByAnotherOrder(ctx context.Context, orderID int64, tradeNo string) (bool, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if s == nil || s.entClient == nil || tradeNo == "" {
		return false, nil
	}
	exists, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.PaymentTradeNoEQ(tradeNo),
			paymentorder.IDNEQ(orderID),
		).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check reused payment trade_no %s: %w", tradeNo, err)
	}
	return exists, nil
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

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, pk string) error {
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
	).SetStatus(OrderStatusPaid).SetPayAmount(paid).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o, tradeNo, pk)
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
			"paidAmount":      paid,
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid})
	return s.executeFulfillment(ctx, o.ID)
}

func (s *PaymentService) alreadyProcessed(ctx context.Context, o *dbent.PaymentOrder, tradeNo, providerKey string) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded, OrderStatusPaid, OrderStatusRecharging:
		slog.Info("duplicate payment webhook ignored",
			"orderID", cur.ID,
			"status", cur.Status,
			"tradeNo", tradeNo,
			"provider", providerKey,
		)
		return nil
	case OrderStatusFailed:
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
		_, err := s.applyCustomReferralCommissionForOrder(ctx, o)
		return err
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

func (s *PaymentService) doBalance(ctx context.Context, o *dbent.PaymentOrder) error {
	// Idempotency: check if redeem code already exists (from a previous partial run)
	existing, lookupErr := s.redeemService.GetByCode(ctx, o.RechargeCode)
	action := resolveRedeemAction(existing, lookupErr)

	switch action {
	case redeemActionSkipCompleted:
		// Code already created and redeemed — just mark completed
		return s.markCompletedAndApplyCustomReferralCommission(ctx, o, "RECHARGE_SUCCESS")
	case redeemActionCreate:
		rc := &RedeemCode{Code: o.RechargeCode, Type: RedeemTypeBalance, Value: o.Amount, Status: StatusUnused}
		if err := s.redeemService.CreateCode(ctx, rc); err != nil {
			return fmt.Errorf("create redeem code: %w", err)
		}
	case redeemActionRedeem:
		// Code exists but unused — skip creation, proceed to redeem
	}
	if _, err := s.redeemService.Redeem(ContextSkipRedeemAffiliate(ctx), o.UserID, o.RechargeCode); err != nil {
		return fmt.Errorf("redeem balance: %w", err)
	}
	return s.markCompletedAndApplyCustomReferralCommission(ctx, o, "RECHARGE_SUCCESS")
}

func (s *PaymentService) markCompleted(ctx context.Context, o *dbent.PaymentOrder, auditAction string) error {
	if s == nil || s.entClient == nil || o == nil {
		return fmt.Errorf("payment service is unavailable")
	}
	now := time.Now()
	commissionStatus := customReferralInitialCommissionStatus(o)
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	update := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).
		SetStatus(OrderStatusCompleted).
		SetCompletedAt(now).
		SetCustomReferralCommissionStatus(commissionStatus).
		ClearCustomReferralCommissionError()
	if commissionStatus == customReferralOrderCommissionStatusSkipped {
		update = update.SetCustomReferralCommissionAt(now)
	}
	affected, err := update.Save(txCtx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	if affected > 0 && commissionStatus == customReferralOrderCommissionStatusPending {
		if err := s.enqueueCustomReferralCommissionJobTx(txCtx, tx, o); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit completion transaction: %w", err)
	}
	s.writeAuditLog(ctx, o.ID, auditAction, "system", map[string]any{
		"rechargeCode":   o.RechargeCode,
		"creditedAmount": o.Amount,
		"payAmount":      o.PayAmount,
	})
	return nil
}

func (s *PaymentService) markCompletedAndApplyCustomReferralCommission(ctx context.Context, o *dbent.PaymentOrder, auditAction string) error {
	if err := s.markCompleted(ctx, o, auditAction); err != nil {
		return err
	}
	if amount, err := s.applyCustomReferralCommissionForOrder(ctx, o); err != nil {
		slog.Error("custom referral commission job failed after order completion", "orderID", o.ID, "amount", amount, "error", err)
	}
	return nil
}

func (s *PaymentService) ExecuteSubscriptionFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status == OrderStatusCompleted {
		_, err := s.applyCustomReferralCommissionForOrder(ctx, o)
		return err
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
	if err != nil || g.Status != payment.EntityStatusActive {
		return fmt.Errorf("group %d no longer exists or inactive", gid)
	}
	// Idempotency: check audit log to see if subscription was already assigned.
	// Prevents double-extension on retry after markCompleted fails.
	if s.hasAuditLog(ctx, o.ID, "SUBSCRIPTION_SUCCESS") {
		slog.Info("subscription already assigned for order, skipping", "orderID", o.ID, "groupID", gid)
		return s.markCompletedAndApplyCustomReferralCommission(ctx, o, "SUBSCRIPTION_SUCCESS")
	}
	orderNote := fmt.Sprintf("payment order %d", o.ID)
	_, _, err = s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: o.UserID, GroupID: gid, ValidityDays: days, AssignedBy: 0, Notes: orderNote})
	if err != nil {
		return fmt.Errorf("assign subscription: %w", err)
	}
	return s.markCompletedAndApplyCustomReferralCommission(ctx, o, "SUBSCRIPTION_SUCCESS")
}

func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	oid := strconv.FormatInt(orderID, 10)
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(oid), paymentauditlog.ActionEQ(action)).
		Limit(1).Count(ctx)
	return c > 0
}

func paymentSQLExecutorFromClient(client *dbent.Client) (paymentSQLExecutor, error) {
	if client == nil {
		return nil, fmt.Errorf("payment sql executor is not configured")
	}
	exec := paymentSQLExecutorFromEntClient(client)
	if exec == nil {
		return nil, fmt.Errorf("payment sql executor is not supported")
	}
	return exec, nil
}

func paymentSQLExecutorFromEntClient(client *dbent.Client) paymentSQLExecutor {
	if client == nil {
		return nil
	}
	clientValue := reflect.ValueOf(client).Elem()
	configValue := clientValue.FieldByName("config")
	driverValue := configValue.FieldByName("driver")
	if !driverValue.IsValid() {
		return nil
	}
	driver := reflect.NewAt(driverValue.Type(), unsafe.Pointer(driverValue.UnsafeAddr())).Elem().Interface()
	exec, ok := driver.(paymentSQLExecutor)
	if !ok {
		return nil
	}
	return exec
}

func (s *PaymentService) enqueueCustomReferralCommissionJobTx(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder) error {
	if tx == nil || o == nil {
		return nil
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	affiliateID, _, ok := customReferralOrderSnapshot(o)
	if baseAmount <= 0 || !ok {
		return nil
	}
	exec, err := paymentSQLExecutorFromClient(tx.Client())
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
INSERT INTO custom_referral_commission_jobs (
    order_id, affiliate_id, status, attempt_count, last_error, created_at, updated_at
) VALUES ($1, $2, $3, 0, '', NOW(), NOW())
ON CONFLICT (order_id) DO NOTHING`,
		o.ID,
		affiliateID,
		CustomReferralCommissionJobStatusPending,
	)
	if err != nil {
		return fmt.Errorf("enqueue custom referral commission job: %w", err)
	}
	return nil
}

func (s *PaymentService) ensureCustomReferralCommissionJob(ctx context.Context, o *dbent.PaymentOrder) error {
	if s == nil || s.entClient == nil || o == nil {
		return nil
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	affiliateID, _, ok := customReferralOrderSnapshot(o)
	if baseAmount <= 0 || !ok {
		return nil
	}
	exec, err := paymentSQLExecutorFromClient(s.entClient)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
INSERT INTO custom_referral_commission_jobs (
    order_id, affiliate_id, status, attempt_count, last_error, created_at, updated_at
) VALUES ($1, $2, $3, 0, '', NOW(), NOW())
ON CONFLICT (order_id) DO NOTHING`,
		o.ID,
		affiliateID,
		CustomReferralCommissionJobStatusPending,
	)
	if err != nil {
		return fmt.Errorf("ensure custom referral commission job: %w", err)
	}
	return nil
}

func (s *PaymentService) claimCustomReferralCommissionJob(ctx context.Context, orderID int64) (bool, error) {
	exec, err := paymentSQLExecutorFromClient(s.entClient)
	if err != nil {
		return false, err
	}
	rows, err := exec.QueryContext(ctx, `
UPDATE custom_referral_commission_jobs
SET status = $2,
    attempt_count = attempt_count + 1,
    locked_at = NOW(),
    last_error = '',
    updated_at = NOW()
WHERE order_id = $1
  AND status IN ($3, $4)
RETURNING id`,
		orderID,
		CustomReferralCommissionJobStatusProcessing,
		CustomReferralCommissionJobStatusPending,
		CustomReferralCommissionJobStatusFailed,
	)
	if err != nil {
		return false, fmt.Errorf("claim custom referral commission job: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return rows.Next(), rows.Err()
}

func (s *PaymentService) markCustomReferralCommissionJob(ctx context.Context, orderID int64, status string, cause error) {
	exec, err := paymentSQLExecutorFromClient(s.entClient)
	if err != nil {
		slog.Error("custom referral commission job executor unavailable", "orderID", orderID, "error", err)
		return
	}
	lastErr := ""
	if cause != nil {
		lastErr = cause.Error()
	}
	switch status {
	case CustomReferralCommissionJobStatusSucceeded:
		_, err = exec.ExecContext(ctx, `
UPDATE custom_referral_commission_jobs
SET status = $2,
    succeeded_at = NOW(),
    failed_at = NULL,
    last_error = '',
    updated_at = NOW()
WHERE order_id = $1`,
			orderID,
			status,
		)
	default:
		_, err = exec.ExecContext(ctx, `
UPDATE custom_referral_commission_jobs
SET status = $2,
    failed_at = NOW(),
    last_error = $3,
    updated_at = NOW()
WHERE order_id = $1`,
			orderID,
			status,
			lastErr,
		)
	}
	if err != nil {
		slog.Error("mark custom referral commission job", "orderID", orderID, "status", status, "error", err)
	}
}

func (s *PaymentService) applyCustomReferralCommissionForOrder(ctx context.Context, o *dbent.PaymentOrder) (float64, error) {
	if o != nil && s != nil && s.entClient != nil {
		if current, err := s.entClient.PaymentOrder.Get(ctx, o.ID); err == nil {
			o = current
		}
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	_, _, hasSnapshot := customReferralOrderSnapshot(o)
	if o == nil || baseAmount <= 0 || !hasSnapshot {
		if o != nil {
			s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusSkipped, nil)
		}
		return 0, nil
	}
	if err := s.ensureCustomReferralCommissionJob(ctx, o); err != nil {
		s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusFailed, err)
		return 0, err
	}
	claimed, err := s.claimCustomReferralCommissionJob(ctx, o.ID)
	if err != nil {
		s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusFailed, err)
		return 0, err
	}
	if !claimed {
		return 0, nil
	}
	amount, err := s.createCustomReferralCommissionForOrder(ctx, o)
	if err != nil {
		s.markCustomReferralCommissionJob(ctx, o.ID, CustomReferralCommissionJobStatusFailed, err)
		return amount, err
	}
	s.markCustomReferralCommissionJob(ctx, o.ID, CustomReferralCommissionJobStatusSucceeded, nil)
	return amount, nil
}

func (s *PaymentService) createCustomReferralCommissionForOrder(ctx context.Context, o *dbent.PaymentOrder) (float64, error) {
	if o != nil && s != nil && s.entClient != nil {
		if current, err := s.entClient.PaymentOrder.Get(ctx, o.ID); err == nil {
			o = current
		}
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	baseAmountDec := customReferralCommissionBaseAmountDecimal(o)
	affiliateID, rate, rateDec, hasSnapshot := customReferralOrderSnapshotDecimal(o)
	if o == nil || baseAmount <= 0 || !hasSnapshot {
		if o != nil {
			s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusSkipped, nil)
		}
		return 0, nil
	}
	if s == nil || s.customReferralService == nil {
		err := fmt.Errorf("custom referral service is unavailable")
		s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusFailed, err)
		return 0, err
	}
	if o.CustomReferralCommissionStatus == customReferralOrderCommissionStatusSucceeded {
		return 0, nil
	}
	amount, err := s.customReferralService.CreateCommissionForOrder(ctx, CustomReferralOrderInput{
		OrderID:           o.ID,
		UserID:            o.UserID,
		AffiliateID:       affiliateID,
		OrderType:         strings.TrimSpace(o.OrderType),
		BaseAmount:        baseAmount,
		BaseAmountDecimal: baseAmountDec,
		Rate:              rate,
		RateDecimal:       rateDec,
		PaidAt:            valueOrNow(o.PaidAt),
	})
	if err != nil {
		s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusFailed, err)
		s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_COMMISSION_FAILED", "system", map[string]any{
			"baseAmount":  baseAmount,
			"orderAmount": o.Amount,
			"payAmount":   o.PayAmount,
			"error":       err.Error(),
		})
		return 0, err
	}
	if amount <= 0 {
		s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusSkipped, nil)
		return 0, nil
	}
	s.markCustomReferralCommissionStatus(ctx, o.ID, customReferralOrderCommissionStatusSucceeded, nil)
	s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_COMMISSION_PENDING", "system", map[string]any{
		"baseAmount":  baseAmount,
		"orderAmount": o.Amount,
		"payAmount":   o.PayAmount,
		"commission":  amount,
		"orderType":   o.OrderType,
	})
	return amount, nil
}

func (s *PaymentService) RetryCustomReferralCommission(ctx context.Context, oid int64) (float64, error) {
	if s == nil || s.customReferralService == nil {
		return 0, infraerrors.BadRequest("CUSTOM_REFERRAL_UNAVAILABLE", "custom referral service is unavailable")
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return 0, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusCompleted {
		return 0, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can retry referral commission")
	}
	baseAmount := customReferralCommissionBaseAmount(o)
	if baseAmount <= 0 {
		return 0, infraerrors.BadRequest("INVALID_AMOUNT", "order has no payable amount")
	}
	amount, err := s.applyCustomReferralCommissionForOrder(ctx, o)
	if err != nil {
		s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_COMMISSION_RETRY_FAILED", "admin", map[string]any{"error": err.Error()})
		return 0, err
	}
	if amount > 0 {
		s.writeAuditLog(ctx, o.ID, "CUSTOM_REFERRAL_COMMISSION_RETRIED", "admin", map[string]any{
			"baseAmount":  baseAmount,
			"orderAmount": o.Amount,
			"payAmount":   o.PayAmount,
			"commission":  amount,
			"orderType":   o.OrderType,
		})
	}
	return amount, nil
}

func customReferralCommissionBaseAmount(o *dbent.PaymentOrder) float64 {
	return customReferralCommissionBaseAmountDecimal(o).InexactFloat64()
}

func customReferralCommissionBaseAmountDecimal(o *dbent.PaymentOrder) decimal.Decimal {
	if o == nil {
		return decimal.Zero
	}
	return moneyx.Currency(o.CommissionBaseAmount)
}

func customReferralOrderSnapshot(o *dbent.PaymentOrder) (int64, float64, bool) {
	affiliateID, rate, _, ok := customReferralOrderSnapshotDecimal(o)
	return affiliateID, rate, ok
}

func customReferralOrderSnapshotDecimal(o *dbent.PaymentOrder) (int64, float64, decimal.Decimal, bool) {
	if o == nil || o.CustomReferralAffiliateID == nil || *o.CustomReferralAffiliateID <= 0 || o.CustomReferralRate <= 0 {
		return 0, 0, decimal.Zero, false
	}
	rateDec := moneyx.Rate(o.CustomReferralRate)
	return *o.CustomReferralAffiliateID, rateDec.InexactFloat64(), rateDec, true
}

func customReferralInitialCommissionStatus(o *dbent.PaymentOrder) string {
	baseAmount := customReferralCommissionBaseAmount(o)
	_, _, ok := customReferralOrderSnapshot(o)
	if baseAmount > 0 && ok {
		return customReferralOrderCommissionStatusPending
	}
	return customReferralOrderCommissionStatusSkipped
}

func (s *PaymentService) markCustomReferralCommissionStatus(ctx context.Context, orderID int64, status string, cause error) {
	if s == nil || s.entClient == nil || orderID <= 0 {
		return
	}
	update := s.entClient.PaymentOrder.UpdateOneID(orderID).
		SetCustomReferralCommissionStatus(status)
	switch status {
	case customReferralOrderCommissionStatusSucceeded, customReferralOrderCommissionStatusSkipped:
		update = update.ClearCustomReferralCommissionError().SetCustomReferralCommissionAt(time.Now())
	case customReferralOrderCommissionStatusFailed:
		update = update.SetCustomReferralCommissionError(psErrMsg(cause))
	}
	if _, err := update.Save(ctx); err != nil {
		slog.Warn("mark custom referral commission status failed", "orderID", orderID, "status", status, "error", err)
	}
}

func valueOrNow(ts *time.Time) time.Time {
	if ts == nil || ts.IsZero() {
		return time.Now()
	}
	return *ts
}

func (s *PaymentService) markFailed(ctx context.Context, oid int64, cause error) {
	now := time.Now()
	r := psErrMsg(cause)
	// Only mark FAILED if still in RECHARGING state — prevents overwriting
	// a COMPLETED order when markCompleted failed but fulfillment succeeded.
	c, e := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(oid), paymentorder.StatusEQ(OrderStatusRecharging)).
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
