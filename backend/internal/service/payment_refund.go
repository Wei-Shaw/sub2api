package service

import (
	"context"
	"crypto/rand"
	stdsql "database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

// --- Refund Flow ---

var createPaymentProviderFromInstance = provider.CreateProvider

// getOrderProviderInstance looks up the provider instance that processed this order.
// For legacy orders without provider_instance_id, it resolves only when the
// historical instance is uniquely identifiable from the stored order fields.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return s.resolveUniqueLegacyOrderProviderInstance(ctx, o)
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Get(ctx, instID)
}

// getRefundOrderProviderInstance resolves the provider instance for refund paths.
// Refunds must be pinned to an explicit historical binding, so legacy
// "best-effort" provider guessing is intentionally not allowed here.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return nil, nil
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d refund provider instance id is invalid: %s", o.ID, instIDStr)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey != "" {
		instances, err := s.entClient.PaymentProviderInstance.Query().
			Where(paymentproviderinstance.ProviderKeyEQ(providerKey)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
		if len(matched) == 1 {
			return matched[0], nil
		}
		return nil, nil
	}

	if paymentType == "" {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		All(ctx)
	if err != nil {
		return nil, err
	}

	matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
	if len(matched) == 1 {
		return matched[0], nil
	}
	return nil, nil
}

func psFilterLegacyOrderProviderInstances(orderPaymentType string, instances []*dbent.PaymentProviderInstance) []*dbent.PaymentProviderInstance {
	if len(instances) == 0 {
		return nil
	}
	if strings.TrimSpace(orderPaymentType) == "" {
		return instances
	}
	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if psLegacyOrderMatchesInstance(orderPaymentType, inst) {
			matched = append(matched, inst)
		}
	}
	return matched
}

func psLegacyOrderMatchesInstance(orderPaymentType string, inst *dbent.PaymentProviderInstance) bool {
	if inst == nil {
		return false
	}

	baseType := payment.GetBasePaymentType(strings.TrimSpace(orderPaymentType))
	instanceProviderKey := strings.TrimSpace(inst.ProviderKey)
	if baseType == "" {
		return false
	}

	if baseType == payment.TypeStripe {
		return instanceProviderKey == payment.TypeStripe
	}
	if instanceProviderKey == payment.TypeStripe {
		return false
	}
	if instanceProviderKey == baseType {
		return true
	}
	return payment.InstanceSupportsType(inst.SupportedTypes, baseType)
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
	requestIdentity, err := newRefundRequestIdentity()
	if err != nil {
		return infraerrors.InternalServer("REFUND_REQUEST_IDENTITY_FAILED", "failed to create refund request identity")
	}
	err = s.withRefundNotificationTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		c, updateErr := txClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(requestIdentity).SetRefundAmount(o.Amount).Save(txCtx)
		if updateErr != nil {
			return fmt.Errorf("update: %w", updateErr)
		}
		if c == 0 {
			return infraerrors.Conflict("CONFLICT", "order status changed")
		}
		if auditErr := createPaymentAuditLog(txCtx, txClient, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr}); auditErr != nil {
			return fmt.Errorf("record refund request: %w", auditErr)
		}
		if dispatchErr := s.dispatchRefundRequestNotifications(txCtx, o, now, nr); dispatchErr != nil {
			return fmt.Errorf("enqueue refund request notification: %w", dispatchErr)
		}
		return nil
	})
	if err != nil {
		s.recordRefundNotificationError(oid, err)
		return err
	}
	return nil
}

func newRefundRequestIdentity() (string, error) {
	// This legacy field is not consumed as an actor ID anywhere else. A unique
	// per-request value turns it into an ABA-safe generation token without a
	// schema migration; the actual user remains recorded in the audit operator.
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate refund request identity: %w", err)
	}
	return "r:" + base64.RawURLEncoding.EncodeToString(random), nil
}

const refundRequestRejectedStatus = "REFUND_REJECTED"

// RejectRefundRequest rejects a user-submitted refund request without calling
// the payment gateway or changing the user's balance. The conditional update
// races safely with approval: exactly one decision can move REFUND_REQUESTED.
func (s *PaymentService) RejectRefundRequest(ctx context.Context, oid, adminID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return infraerrors.BadRequest("REFUND_REJECTION_REASON_REQUIRED", "refund rejection reason is required")
	}
	if len([]rune(reason)) > 1000 {
		return infraerrors.BadRequest("REFUND_REJECTION_REASON_TOO_LONG", "refund rejection reason must not exceed 1000 characters")
	}

	order, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if order.Status != OrderStatusRefundRequested {
		return infraerrors.Conflict("CONFLICT", "only a pending refund request can be rejected")
	}
	return s.rejectRefundRequestDecision(ctx, order, adminID, reason)
}

func (s *PaymentService) rejectRefundRequestDecision(ctx context.Context, order *dbent.PaymentOrder, adminID int64, reason string) error {
	if order == nil || order.Status != OrderStatusRefundRequested {
		return infraerrors.Conflict("CONFLICT", "only a pending refund request can be rejected")
	}

	requestedAmount := order.RefundAmount
	requestedReason := strings.TrimSpace(psStringValue(order.RefundRequestReason))
	reviewedAt := time.Now()
	operator := "admin"
	if adminID > 0 {
		operator = fmt.Sprintf("admin:%d", adminID)
	}
	auditDetail := map[string]any{
		"requested_amount": requestedAmount,
		"request_reason":   requestedReason,
		"rejection_reason": reason,
		"reviewed_at":      reviewedAt.UTC().Format(time.RFC3339Nano),
	}
	var notificationErr error
	err := s.withRefundNotificationTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		predicates := append([]predicate.PaymentOrder{paymentorder.IDEQ(order.ID)}, refundRequestIdentityPredicates(order)...)
		updated, updateErr := txClient.PaymentOrder.Update().
			Where(predicates...).
			SetStatus(OrderStatusCompleted).
			SetRefundAmount(0).
			ClearRefundRequestedAt().
			ClearRefundRequestReason().
			ClearRefundRequestedBy().
			ClearFailedAt().
			ClearFailedReason().
			Save(txCtx)
		if updateErr != nil {
			return fmt.Errorf("reject refund request: %w", updateErr)
		}
		if updated == 0 {
			return infraerrors.Conflict("CONFLICT", "refund request was already processed")
		}
		if auditErr := createPaymentAuditLog(txCtx, txClient, order.ID, "REFUND_REQUEST_REJECTED", operator, auditDetail); auditErr != nil {
			return fmt.Errorf("record rejected refund request: %w", auditErr)
		}

		gatewayAmount := calculateGatewayRefundAmount(order.Amount, order.PayAmount, requestedAmount, PaymentOrderCurrency(order))
		var fatalErr error
		notificationErr, fatalErr = tryRefundResultNotification(txCtx, txClient, func() error {
			return s.dispatchRefundResultNotification(
				txCtx,
				order,
				NotificationEmailEventRefundRejectedUser,
				refundRequestRejectedStatus,
				gatewayAmount,
				reason,
				reviewedAt,
			)
		})
		return fatalErr
	})
	if err != nil {
		return err
	}

	s.recordRefundNotificationError(order.ID, notificationErr)
	return nil
}

func refundRequestIdentityPredicates(order *dbent.PaymentOrder) []predicate.PaymentOrder {
	predicates := []predicate.PaymentOrder{
		paymentorder.StatusEQ(OrderStatusRefundRequested),
		paymentorder.RefundAmountEQ(order.RefundAmount),
	}
	if order.RefundRequestReason == nil {
		predicates = append(predicates, paymentorder.RefundRequestReasonIsNil())
	} else {
		predicates = append(predicates, paymentorder.RefundRequestReasonEQ(*order.RefundRequestReason))
	}
	if order.RefundRequestedBy == nil {
		predicates = append(predicates, paymentorder.RefundRequestedByIsNil())
	} else {
		predicates = append(predicates, paymentorder.RefundRequestedByEQ(*order.RefundRequestedBy))
	}
	return predicates
}

func (s *PaymentService) dispatchRefundRequestNotifications(ctx context.Context, o *dbent.PaymentOrder, requestedAt time.Time, reason string) error {
	if s == nil || s.notificationEmailDispatcher == nil || s.configService == nil || o == nil {
		return nil
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		slog.Warn("refund request notification setting lookup failed", "order_id", o.ID, "err", err.Error())
		return nil
	}
	baseInput := NotificationEmailSendInput{
		SourceType:  "payment_refund_request",
		SourceID:    strconv.FormatInt(o.ID, 10),
		ReminderKey: requestedAt.UTC().Format(time.RFC3339Nano),
		Variables:   refundNotificationVariables(o, calculateGatewayRefundAmount(o.Amount, o.PayAmount, o.Amount, PaymentOrderCurrency(o)), reason, OrderStatusRefundRequested, requestedAt),
	}
	if cfg.RefundRequestUserEmailEnabled {
		input := baseInput
		input.Event = NotificationEmailEventRefundRequestedUser
		input.RecipientEmail = o.UserEmail
		input.RecipientName = firstNonEmpty(o.UserName, o.UserEmail)
		input.UserID = o.UserID
		if _, err := s.notificationEmailDispatcher.Enqueue(ctx, input); err != nil && !errors.Is(err, ErrNotificationEmailChannelDisabled) {
			return refundNotificationEnqueueError{Event: NotificationEmailEventRefundRequestedUser, Err: err}
		}
	}
	if cfg.RefundRequestAdminEmailEnabled {
		input := baseInput
		input.Event = NotificationEmailEventRefundRequestedAdmin
		input.Variables = cloneNotificationEmailVariables(baseInput.Variables)
		input.Variables["refund_user_id"] = strconv.FormatInt(o.UserID, 10)
		input.Variables["refund_user_email"] = strings.TrimSpace(o.UserEmail)
		input.Variables["refund_user_name"] = strings.TrimSpace(o.UserName)
		if _, err := s.notificationEmailDispatcher.EnqueueGroup(ctx, input); err != nil && !errors.Is(err, ErrNotificationEmailChannelDisabled) {
			return refundNotificationEnqueueError{Event: NotificationEmailEventRefundRequestedAdmin, Err: err}
		}
	}
	return nil
}

func (s *PaymentService) dispatchRefundResultNotification(ctx context.Context, o *dbent.PaymentOrder, event, status string, gatewayAmount float64, reason string, completedAt time.Time) error {
	if s == nil || s.notificationEmailDispatcher == nil || s.configService == nil || o == nil {
		return nil
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil || !cfg.RefundResultUserEmailEnabled {
		if err != nil {
			slog.Warn("refund result notification setting lookup failed", "order_id", o.ID, "err", err.Error())
		}
		return nil
	}
	reminderKey := status
	if event == NotificationEmailEventRefundRejectedUser {
		// A rejected order returns to COMPLETED and the user may submit a later
		// request. Keep retries of this decision idempotent while allowing a
		// future, separately reviewed rejection to notify the user again.
		reminderKey = status + ":" + completedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.notificationEmailDispatcher.Enqueue(ctx, NotificationEmailSendInput{
		Event: event, RecipientEmail: o.UserEmail, RecipientName: firstNonEmpty(o.UserName, o.UserEmail), UserID: o.UserID,
		SourceType: "payment_refund_result", SourceID: strconv.FormatInt(o.ID, 10), ReminderKey: reminderKey,
		Variables: refundNotificationVariables(o, gatewayAmount, reason, status, completedAt),
	})
	if err != nil && !errors.Is(err, ErrNotificationEmailChannelDisabled) {
		return refundNotificationEnqueueError{Event: event, Err: err}
	}
	return nil
}

type refundNotificationEnqueueError struct {
	Event string
	Err   error
}

type refundNotificationSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (stdsql.Result, error)
}

func (e refundNotificationEnqueueError) Error() string { return e.Err.Error() }
func (e refundNotificationEnqueueError) Unwrap() error { return e.Err }

func (s *PaymentService) recordRefundNotificationError(orderID int64, err error) {
	var enqueueErr refundNotificationEnqueueError
	if errors.As(err, &enqueueErr) {
		s.recordRefundNotificationEnqueueFailure(orderID, enqueueErr.Event, enqueueErr.Err)
	}
}

func (s *PaymentService) withRefundNotificationTx(ctx context.Context, fn func(context.Context, *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund notification transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit refund notification transaction: %w", err)
	}
	return nil
}

// tryRefundResultNotification isolates the outbox insert behind a savepoint.
// A provider result is an external side effect and must still commit when only
// notification persistence fails.
func tryRefundResultNotification(ctx context.Context, txClient *dbent.Client, fn func() error) (notificationErr, fatalErr error) {
	exec, ok := txClient.Driver().(refundNotificationSQLExecutor)
	if !ok {
		return nil, errors.New("refund notification transaction does not support savepoints")
	}
	if _, err := exec.ExecContext(ctx, "SAVEPOINT refund_notification_outbox"); err != nil {
		return nil, fmt.Errorf("create refund notification savepoint: %w", err)
	}
	if err := fn(); err != nil {
		if _, rollbackErr := exec.ExecContext(ctx, "ROLLBACK TO SAVEPOINT refund_notification_outbox"); rollbackErr != nil {
			return nil, fmt.Errorf("rollback refund notification savepoint after %v: %w", err, rollbackErr)
		}
		if _, releaseErr := exec.ExecContext(ctx, "RELEASE SAVEPOINT refund_notification_outbox"); releaseErr != nil {
			return nil, fmt.Errorf("release failed refund notification savepoint after %v: %w", err, releaseErr)
		}
		return err, nil
	}
	if _, err := exec.ExecContext(ctx, "RELEASE SAVEPOINT refund_notification_outbox"); err != nil {
		return nil, fmt.Errorf("release refund notification savepoint: %w", err)
	}
	return nil, nil
}

func refundNotificationVariables(o *dbent.PaymentOrder, gatewayAmount float64, reason, status string, at time.Time) map[string]string {
	currency := PaymentOrderCurrency(o)
	variables := map[string]string{
		"order_id": strconv.FormatInt(o.ID, 10), "refund_amount": payment.FormatAmountForCurrency(gatewayAmount, currency),
		"refund_currency": currency, "refund_reason": strings.TrimSpace(reason), "refund_status": status,
	}
	if status == OrderStatusRefundRequested {
		variables["requested_at"] = at.UTC().Format(time.RFC3339)
	} else {
		variables["completed_at"] = at.UTC().Format(time.RFC3339)
	}
	return variables
}

func (s *PaymentService) recordRefundNotificationEnqueueFailure(orderID int64, event string, err error) {
	if err == nil {
		return
	}
	slog.Warn("refund notification enqueue failed", "order_id", orderID, "event", event, "err", boundedNotificationEmailError(err))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.writeAuditLog(ctx, orderID, "REFUND_EMAIL_ENQUEUE_FAILED", "system", map[string]any{"event": event, "detail": boundedNotificationEmailError(err)})
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	// Check provider instance allows user refund
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	ok := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundPending, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	// Check provider instance allows admin refund
	inst, instErr := s.getRefundOrderProviderInstance(ctx, o)
	if instErr != nil {
		slog.Warn("refund: provider instance lookup failed", "orderID", oid, "error", instErr)
		return nil, nil, infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		// Legacy order without provider_instance_id — block refund
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	if math.IsNaN(amt) || math.IsInf(amt, 0) {
		return nil, nil, infraerrors.BadRequest("INVALID_AMOUNT", "invalid refund amount")
	}
	if amt <= 0 {
		amt = o.Amount
	}
	orderCurrency := PaymentOrderCurrency(o)
	if amt-o.Amount > paymentAmountToleranceForCurrency(orderCurrency) {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	fullRefund := math.Abs(amt-o.Amount) <= paymentAmountToleranceForCurrency(orderCurrency)
	if fullRefund {
		amt = o.Amount
	}
	if (o.Status == OrderStatusRefundPending || !fullRefund) && inst.ProviderKey == payment.TypeEasyPay {
		a5Mode, modeErr := s.isA5EasyPayOrder(ctx, o, inst)
		if modeErr != nil {
			return nil, nil, infraerrors.InternalServer("PROVIDER_CONFIG_FAILED", "failed to resolve EasyPay compatibility mode")
		}
		if a5Mode && o.Status == OrderStatusRefundPending {
			return nil, nil, infraerrors.Conflict("A5_REFUND_PENDING_RETRY_BLOCKED", "A5 pending refunds must be queried or reconciled before another refund attempt")
		}
		if a5Mode && !fullRefund {
			return nil, nil, infraerrors.BadRequest("A5_PARTIAL_REFUND_UNSUPPORTED", "A5 compatibility mode only supports full refunds")
		}
	}
	ga := calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt, orderCurrency)
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	p := &RefundPlan{OrderID: oid, Order: o, RefundAmount: amt, GatewayAmount: ga, Reason: rr, Force: force, DeductBalance: deduct, DeductionType: payment.DeductionTypeNone}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) isA5EasyPayOrder(ctx context.Context, order *dbent.PaymentOrder, inst *dbent.PaymentProviderInstance) (bool, error) {
	if inst == nil || inst.ProviderKey != payment.TypeEasyPay {
		return false, nil
	}
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.SchemaVersion >= 3 {
		mode := strings.ToLower(strings.TrimSpace(snapshot.CompatibilityMode))
		if mode != provider.EasyPayCompatibilityStandard && mode != provider.EasyPayCompatibilityA5 {
			return false, fmt.Errorf("order %d EasyPay compatibility mode is missing or invalid", order.ID)
		}
		return mode == provider.EasyPayCompatibilityA5, nil
	}
	if s.loadBalancer == nil {
		return false, errors.New("payment load balancer is unavailable")
	}
	cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return false, err
	}
	pinned, err := pinEasyPayCompatibilityModeToOrder(inst, order, cfg)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(pinned[provider.EasyPayCompatibilityModeKey]), provider.EasyPayCompatibilityA5), nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		p.DeductionType = payment.DeductionTypeSubscription
		if o.SubscriptionGroupID != nil && o.SubscriptionDays != nil {
			p.SubDaysToDeduct = *o.SubscriptionDays
			sub, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
			if err == nil && sub != nil {
				p.SubscriptionID = sub.ID
			} else if !force {
				return &RefundResult{Success: false, Warning: "cannot find active subscription for deduction, use force", RequireForce: true}
			}
		}
		return nil
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	p.BalanceToDeduct = math.Min(p.RefundAmount, u.Balance)
	return nil
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	// PrepareRefund records the exact state the administrator reviewed. Requiring
	// that same state here prevents a stale approval from winning after another
	// administrator has rejected or otherwise changed the refund request.
	predicates := []predicate.PaymentOrder{paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(p.Order.Status)}
	if p.Order.Status == OrderStatusRefundRequested {
		predicates = append([]predicate.PaymentOrder{paymentorder.IDEQ(p.OrderID)}, refundRequestIdentityPredicates(p.Order)...)
	}
	c, err := s.entClient.PaymentOrder.Update().
		Where(predicates...).
		SetStatus(OrderStatusRefunding).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		// Skip balance deduction on retry if previous attempt already deducted
		// but failed to roll back (REFUND_ROLLBACK_FAILED in audit log).
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			if err := s.userRepo.DeductBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
				s.restoreStatus(ctx, p)
				return nil, fmt.Errorf("deduction: %w", err)
			}
		} else {
			slog.Warn("skipping balance deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.BalanceToDeduct = 0
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			_, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct)
			if err != nil {
				if errors.Is(err, ErrAdjustWouldExpire) {
					// Deduction would expire the subscription — revoke it entirely
					slog.Info("subscription deduction would expire, revoking", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct)
					if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
						s.restoreStatus(ctx, p)
						return nil, fmt.Errorf("revoke subscription: %w", revokeErr)
					}
				} else {
					// Other errors (DB failure, not found) — abort refund
					s.restoreStatus(ctx, p)
					return nil, fmt.Errorf("deduct subscription days: %w", err)
				}
			}
		} else {
			slog.Warn("skipping subscription deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.SubDaysToDeduct = 0
		}
	}
	resp, err := s.gwRefund(ctx, p)
	if err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.finishRefund(ctx, p, resp)
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) (*payment.RefundResponse, error) {
	if p.Order.PaymentTradeNo == "" {
		inst, err := s.getRefundOrderProviderInstance(ctx, p.Order)
		if err != nil {
			return nil, fmt.Errorf("get refund provider instance: %w", err)
		}
		a5Mode, modeErr := s.isA5EasyPayOrder(ctx, p.Order, inst)
		if modeErr != nil {
			return nil, fmt.Errorf("resolve EasyPay compatibility mode: %w", modeErr)
		}
		if !a5Mode {
			s.writeAuditLog(ctx, p.Order.ID, "REFUND_NO_TRADE_NO", "admin", map[string]any{"detail": "skipped"})
			return &payment.RefundResponse{Status: payment.ProviderStatusSuccess}, nil
		}
	}

	// Use the exact provider instance that created this order, not a random one
	// from the registry. Each instance has its own merchant credentials.
	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_PROVIDER_METADATA_MISMATCH", "admin", map[string]any{
			"detail": err.Error(),
		})
		return nil, err
	}
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := prov.Refund(ctx, payment.RefundRequest{
		TradeNo:    p.Order.PaymentTradeNo,
		OrderID:    p.Order.OutTradeNo,
		Amount:     formatGatewayRefundAmount(p.GatewayAmount, p.Order),
		Reason:     p.Reason,
		FullRefund: refundPlanIsFull(p),
	})
	finishProviderCall()
	if err != nil {
		if resp != nil && strings.TrimSpace(resp.Status) == payment.ProviderStatusPending {
			return resp, nil
		}
		return nil, err
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func refundPlanIsFull(p *RefundPlan) bool {
	if p == nil || p.Order == nil {
		return false
	}
	return math.Abs(p.RefundAmount-p.Order.Amount) <= paymentAmountToleranceForCurrency(PaymentOrderCurrency(p.Order))
}

func formatGatewayRefundAmount(amount float64, order *dbent.PaymentOrder) string {
	return payment.FormatAmountForCurrency(amount, PaymentOrderCurrency(order))
}

func validateRefundProviderResponse(resp *payment.RefundResponse) error {
	if resp == nil {
		return fmt.Errorf("payment refund response missing")
	}
	status := strings.TrimSpace(resp.Status)
	switch status {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded, payment.ProviderStatusPending:
		return nil
	case payment.ProviderStatusFailed:
		return fmt.Errorf("payment refund failed: status %s", status)
	default:
		return fmt.Errorf("payment refund returned unknown status: %s", status)
	}
}

func (s *PaymentService) finishRefund(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	if err := validateRefundProviderResponse(resp); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	switch strings.TrimSpace(resp.Status) {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		return s.markRefundOk(ctx, p, resp)
	case payment.ProviderStatusPending:
		return s.markRefundPending(ctx, p, resp)
	default:
		return s.handleGwFail(ctx, p, fmt.Errorf("payment refund returned unknown status: %s", strings.TrimSpace(resp.Status)))
	}
}

func (s *PaymentService) QueryAndFinalizeRefund(ctx context.Context, oid int64) (*RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusRefundPending {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only refund pending orders can be finalized")
	}
	pendingDetail, err := s.latestRefundPendingDetail(ctx, oid)
	if err != nil {
		s.writeAuditLogOnce(ctx, oid, "REFUND_PENDING_STATE_INVALID", "admin", map[string]any{"detail": psErrMsg(err)})
		return nil, infraerrors.Conflict("REFUND_PENDING_STATE_INVALID", "refund pending deduction state is missing or invalid; manual reconciliation is required")
	}
	plan, err := s.refundFinalizePlan(ctx, o, pendingDetail)
	if err != nil {
		s.writeAuditLogOnce(ctx, oid, "REFUND_PENDING_STATE_INVALID", "admin", map[string]any{"detail": psErrMsg(err)})
		return nil, infraerrors.Conflict("REFUND_PENDING_STATE_INVALID", "refund pending deduction state is inconsistent; manual reconciliation is required")
	}

	prov, err := s.getRefundProvider(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("get refund provider: %w", err)
	}
	queryProvider, ok := prov.(payment.RefundQueryProvider)
	if !ok {
		return nil, infraerrors.BadRequest("REFUND_QUERY_UNSUPPORTED", "this payment provider does not support refund status query; please verify manually")
	}

	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	resp, err := queryProvider.QueryRefund(ctx, payment.RefundQueryRequest{
		TradeNo:    o.PaymentTradeNo,
		OrderID:    o.OutTradeNo,
		RefundID:   pendingDetail.RefundID,
		Amount:     formatGatewayRefundAmount(calculateGatewayRefundAmount(o.Amount, o.PayAmount, o.RefundAmount, PaymentOrderCurrency(o)), o),
		FullRefund: math.Abs(o.RefundAmount-o.Amount) <= paymentAmountToleranceForCurrency(PaymentOrderCurrency(o)),
	})
	finishProviderCall()
	if err != nil {
		return nil, fmt.Errorf("query refund: %w", err)
	}
	if err := validateRefundProviderResponse(resp); err != nil {
		return s.finalizeRefundFailed(ctx, o, err)
	}

	switch strings.TrimSpace(resp.Status) {
	case payment.ProviderStatusSuccess, payment.ProviderStatusRefunded:
		return s.finalizeQueriedRefund(ctx, o, plan, resp)
	case payment.ProviderStatusPending:
		detail := map[string]any{"refundID": resp.RefundID}
		if message := boundedRefundProviderMessage(resp.Message); message != "" {
			detail["providerMessage"] = message
		}
		s.writeAuditLogOnce(ctx, oid, "REFUND_QUERY_PENDING", "admin", detail)
		return &RefundResult{Success: false, Warning: "gateway refund is still pending confirmation"}, nil
	default:
		return s.finalizeRefundFailed(ctx, o, fmt.Errorf("payment refund returned unknown status: %s", strings.TrimSpace(resp.Status)))
	}
}

func (s *PaymentService) finalizeQueriedRefund(ctx context.Context, order *dbent.PaymentOrder, plan *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund finalization transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	claimed, err := tx.Client().PaymentOrder.Update().
		Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(OrderStatusRefundPending)).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return nil, fmt.Errorf("claim pending refund: %w", err)
	}
	if claimed == 0 {
		return nil, infraerrors.Conflict("REFUND_FINALIZE_CONFLICT", "refund status changed while finalizing")
	}
	if err := s.applyRefundFinalDeduction(txCtx, plan); err != nil {
		return nil, err
	}
	result, err := s.markRefundOkRequiredAudit(txCtx, plan, resp)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund finalization transaction: %w", err)
	}
	return result, nil
}

func (s *PaymentService) refundFinalizePlan(ctx context.Context, o *dbent.PaymentOrder, detail refundPendingAuditDetail) (*RefundPlan, error) {
	if o == nil || detail.DeductionRollbackOK == nil {
		return nil, errors.New("refund pending deduction state is incomplete")
	}
	if detail.Legacy {
		return s.legacyRefundFinalizePlan(ctx, o, detail)
	}
	if detail.DeductBalance == nil || detail.BalanceToDeduct == nil || detail.SubDaysToDeduct == nil || detail.SubscriptionID == nil {
		return nil, errors.New("refund pending deduction state is incomplete")
	}
	if *detail.BalanceToDeduct < 0 || *detail.SubDaysToDeduct < 0 || *detail.SubscriptionID < 0 {
		return nil, errors.New("refund pending deduction state contains negative values")
	}
	switch detail.DeductionType {
	case payment.DeductionTypeNone:
		if *detail.BalanceToDeduct != 0 || *detail.SubDaysToDeduct != 0 || *detail.SubscriptionID != 0 {
			return nil, errors.New("refund pending none deduction state contains deduction values")
		}
	case payment.DeductionTypeBalance:
		if o.OrderType != payment.OrderTypeBalance || *detail.SubDaysToDeduct != 0 || *detail.SubscriptionID != 0 {
			return nil, errors.New("refund pending balance deduction state is inconsistent")
		}
	case payment.DeductionTypeSubscription:
		if o.OrderType != payment.OrderTypeSubscription || *detail.BalanceToDeduct != 0 {
			return nil, errors.New("refund pending subscription deduction state is inconsistent")
		}
	default:
		return nil, errors.New("refund pending deduction type is invalid")
	}
	refundAmount := o.RefundAmount
	reason := strings.TrimSpace(psStringValue(o.RefundReason))
	if reason == "" {
		reason = fmt.Sprintf("refund order:%d", o.ID)
	}
	plan := &RefundPlan{
		OrderID:       o.ID,
		Order:         o,
		RefundAmount:  refundAmount,
		GatewayAmount: calculateGatewayRefundAmount(o.Amount, o.PayAmount, refundAmount, PaymentOrderCurrency(o)),
		Reason:        reason,
		Force:         o.ForceRefund,
		DeductBalance: *detail.DeductBalance,
		DeductionType: detail.DeductionType,
	}
	if *detail.DeductBalance && *detail.DeductionRollbackOK {
		plan.BalanceToDeduct = *detail.BalanceToDeduct
		plan.SubDaysToDeduct = *detail.SubDaysToDeduct
		plan.SubscriptionID = *detail.SubscriptionID
	}
	return plan, nil
}

func (s *PaymentService) legacyRefundFinalizePlan(ctx context.Context, o *dbent.PaymentOrder, detail refundPendingAuditDetail) (*RefundPlan, error) {
	if detail.BalanceRolledBack == nil || detail.SubDaysRolledBack == nil || detail.DeductionRollbackOK == nil ||
		*detail.BalanceRolledBack < 0 || *detail.SubDaysRolledBack < 0 {
		return nil, errors.New("legacy refund pending deduction state is invalid")
	}
	if *detail.BalanceRolledBack > 0 && *detail.SubDaysRolledBack > 0 {
		return nil, errors.New("legacy refund pending state contains multiple deduction types")
	}

	refundAmount := o.RefundAmount
	reason := strings.TrimSpace(psStringValue(o.RefundReason))
	if reason == "" {
		reason = fmt.Sprintf("refund order:%d", o.ID)
	}
	plan := &RefundPlan{
		OrderID:       o.ID,
		Order:         o,
		RefundAmount:  refundAmount,
		GatewayAmount: calculateGatewayRefundAmount(o.Amount, o.PayAmount, refundAmount, PaymentOrderCurrency(o)),
		Reason:        reason,
		Force:         o.ForceRefund,
		DeductionType: payment.DeductionTypeNone,
	}
	// A failed rollback means the original deduction is still applied and must
	// not be repeated after the gateway confirms the refund.
	if !*detail.DeductionRollbackOK {
		return plan, nil
	}
	if *detail.BalanceRolledBack > 0 {
		if o.OrderType != payment.OrderTypeBalance {
			return nil, errors.New("legacy refund pending balance state is inconsistent")
		}
		plan.DeductBalance = true
		plan.DeductionType = payment.DeductionTypeBalance
		plan.BalanceToDeduct = *detail.BalanceRolledBack
		return plan, nil
	}
	if *detail.SubDaysRolledBack > 0 {
		if o.OrderType != payment.OrderTypeSubscription || o.SubscriptionGroupID == nil || s.subscriptionSvc == nil {
			return nil, errors.New("legacy refund pending subscription state is incomplete")
		}
		subscription, err := s.subscriptionSvc.GetActiveSubscription(ctx, o.UserID, *o.SubscriptionGroupID)
		if err != nil || subscription == nil {
			return nil, errors.New("legacy refund pending subscription is unavailable")
		}
		plan.DeductBalance = true
		plan.DeductionType = payment.DeductionTypeSubscription
		plan.SubDaysToDeduct = *detail.SubDaysRolledBack
		plan.SubscriptionID = subscription.ID
	}
	return plan, nil
}

func (s *PaymentService) applyRefundFinalDeduction(ctx context.Context, p *RefundPlan) error {
	if s.hasAuditLog(ctx, p.OrderID, "REFUND_SUCCESS") {
		p.BalanceToDeduct = 0
		p.SubDaysToDeduct = 0
		return nil
	}
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if err := s.userRepo.DeductBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			return fmt.Errorf("deduction: %w", err)
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct); err != nil {
			if errors.Is(err, ErrAdjustWouldExpire) {
				if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
					return fmt.Errorf("revoke subscription: %w", revokeErr)
				}
			} else {
				return fmt.Errorf("deduct subscription days: %w", err)
			}
		}
	}
	return nil
}

func (s *PaymentService) finalizeRefundFailed(ctx context.Context, o *dbent.PaymentOrder, gErr error) (*RefundResult, error) {
	now := time.Now()
	var notificationErr error
	err := s.withRefundNotificationTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, updateErr := txClient.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusRefundFailed).SetFailedAt(now).SetFailedReason(psErrMsg(gErr)).Save(txCtx); updateErr != nil {
			return updateErr
		}
		gatewayAmount := calculateGatewayRefundAmount(o.Amount, o.PayAmount, o.RefundAmount, PaymentOrderCurrency(o))
		var fatalErr error
		notificationErr, fatalErr = tryRefundResultNotification(txCtx, txClient, func() error {
			return s.dispatchRefundResultNotification(txCtx, o, NotificationEmailEventRefundFailedUser, OrderStatusRefundFailed, gatewayAmount, psStringValue(o.RefundReason), now)
		})
		return fatalErr
	})
	if err != nil {
		return nil, fmt.Errorf("mark refund failed: %w", err)
	}
	s.recordRefundNotificationError(o.ID, notificationErr)
	s.writeAuditLog(ctx, o.ID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	return &RefundResult{Success: false, Warning: "gateway refund failed: " + psErrMsg(gErr)}, nil
}

type refundPendingAuditDetail struct {
	RefundID            string   `json:"refundID"`
	DeductBalance       *bool    `json:"deductBalance"`
	DeductionType       string   `json:"deductionType"`
	BalanceToDeduct     *float64 `json:"balanceToDeduct"`
	SubDaysToDeduct     *int     `json:"subDaysToDeduct"`
	SubscriptionID      *int64   `json:"subscriptionID"`
	DeductionRollbackOK *bool    `json:"deductionRollbackOK"`
	BalanceRolledBack   *float64 `json:"balanceRolledBack"`
	SubDaysRolledBack   *int     `json:"subDaysRolledBack"`
	Legacy              bool     `json:"-"`
}

func (s *PaymentService) latestRefundPendingDetail(ctx context.Context, oid int64) (refundPendingAuditDetail, error) {
	logEntry, err := s.entClient.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Order(paymentauditlog.ByCreatedAt(sql.OrderDesc())).
		First(ctx)
	if err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("load refund pending state: %w", err)
	}
	if logEntry == nil {
		return refundPendingAuditDetail{}, errors.New("refund pending state is missing")
	}
	detail := refundPendingAuditDetail{}
	if err := json.Unmarshal([]byte(logEntry.Detail), &detail); err != nil {
		return refundPendingAuditDetail{}, fmt.Errorf("parse refund pending state: %w", err)
	}
	detail.RefundID = strings.TrimSpace(detail.RefundID)
	detail.DeductionType = strings.TrimSpace(detail.DeductionType)
	hasNormalizedState := detail.DeductBalance != nil || detail.BalanceToDeduct != nil ||
		detail.SubDaysToDeduct != nil || detail.SubscriptionID != nil || detail.DeductionType != ""
	if hasNormalizedState {
		if detail.DeductBalance == nil || detail.DeductionRollbackOK == nil ||
			detail.BalanceToDeduct == nil || detail.SubDaysToDeduct == nil || detail.SubscriptionID == nil || detail.DeductionType == "" {
			return refundPendingAuditDetail{}, errors.New("refund pending normalized state is incomplete")
		}
		return detail, nil
	}
	// ts.3 wrote the original rollback amounts but not the normalized deduction
	// fields. Accept only that complete legacy shape; partial/malformed state must
	// still fail closed.
	if detail.DeductionRollbackOK != nil && detail.BalanceRolledBack != nil && detail.SubDaysRolledBack != nil {
		detail.Legacy = true
		return detail, nil
	}
	return refundPendingAuditDetail{}, errors.New("refund pending state is incomplete")
}

// getRefundProvider creates a provider using the order's original instance config.
// Delegates to getOrderProvider which handles instance lookup and fallback.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createOrderBoundProvider(ctx, inst, o)
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s.RollbackRefund(ctx, p, gErr) {
		s.restoreStatus(ctx, p)
		s.writeAuditLog(ctx, p.OrderID, "REFUND_GATEWAY_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
		return &RefundResult{Success: false, Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back"}, nil
	}
	now := time.Now()
	var notificationErr error
	err := s.withRefundNotificationTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, updateErr := txClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundFailed).SetFailedAt(now).SetFailedReason(psErrMsg(gErr)).Save(txCtx); updateErr != nil {
			return updateErr
		}
		var fatalErr error
		notificationErr, fatalErr = tryRefundResultNotification(txCtx, txClient, func() error {
			return s.dispatchRefundResultNotification(txCtx, p.Order, NotificationEmailEventRefundFailedUser, OrderStatusRefundFailed, p.GatewayAmount, p.Reason, now)
		})
		return fatalErr
	})
	if err != nil {
		return nil, fmt.Errorf("mark refund failed: %w", err)
	}
	s.recordRefundNotificationError(p.OrderID, notificationErr)
	s.writeAuditLog(ctx, p.OrderID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan, responses ...*payment.RefundResponse) (*RefundResult, error) {
	var resp *payment.RefundResponse
	if len(responses) > 0 {
		resp = responses[0]
	}
	return s.markRefundOkInternal(ctx, p, resp, false)
}

func (s *PaymentService) markRefundOkRequiredAudit(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	return s.markRefundOkInternal(ctx, p, resp, true)
}

func (s *PaymentService) markRefundOkInternal(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse, auditRequired bool) (*RefundResult, error) {
	fs := OrderStatusRefunded
	if p.RefundAmount < p.Order.Amount {
		fs = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	var notificationErr error
	err := s.withRefundNotificationTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, updateErr := txClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(fs).SetRefundAmount(p.RefundAmount).SetRefundReason(p.Reason).SetRefundAt(now).SetForceRefund(p.Force).Save(txCtx); updateErr != nil {
			return updateErr
		}
		var fatalErr error
		notificationErr, fatalErr = tryRefundResultNotification(txCtx, txClient, func() error {
			return s.dispatchRefundResultNotification(txCtx, p.Order, NotificationEmailEventRefundSucceededUser, fs, p.GatewayAmount, p.Reason, now)
		})
		return fatalErr
	})
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	s.recordRefundNotificationError(p.OrderID, notificationErr)
	detail := map[string]any{"refundAmount": p.RefundAmount, "reason": p.Reason, "balanceDeducted": p.BalanceToDeduct, "force": p.Force}
	if refundID := refundResponseID(resp); refundID != "" && resp.RefundIDProviderIssued {
		detail["refundID"] = refundID
	}
	if resp != nil {
		if message := boundedRefundProviderMessage(resp.Message); message != "" {
			detail["providerMessage"] = message
		}
	}
	if err := createPaymentAuditLog(ctx, paymentAuditClient(ctx, s.entClient), p.OrderID, "REFUND_SUCCESS", "admin", detail); err != nil {
		if auditRequired {
			return nil, fmt.Errorf("write refund success audit: %w", err)
		}
		slog.Error("audit log failed", "orderID", p.OrderID, "action", "REFUND_SUCCESS", "error", err)
	}
	return &RefundResult{Success: true, BalanceDeducted: p.BalanceToDeduct, SubDaysDeducted: p.SubDaysToDeduct}, nil
}

func (s *PaymentService) markRefundPending(ctx context.Context, p *RefundPlan, resp *payment.RefundResponse) (*RefundResult, error) {
	balanceDeducted := p.BalanceToDeduct
	subDaysDeducted := p.SubDaysToDeduct

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund pending transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	rollbackOK := s.RollbackRefund(txCtx, p, nil)

	detail := map[string]any{
		"refundID":            refundResponseID(resp),
		"refundAmount":        p.RefundAmount,
		"reason":              p.Reason,
		"force":               p.Force,
		"deductBalance":       p.DeductBalance,
		"deductionType":       p.DeductionType,
		"balanceToDeduct":     balanceDeducted,
		"subDaysToDeduct":     subDaysDeducted,
		"subscriptionID":      p.SubscriptionID,
		"balanceRolledBack":   balanceDeducted,
		"subDaysRolledBack":   subDaysDeducted,
		"deductionRollbackOK": rollbackOK,
	}
	if message := boundedRefundProviderMessage(resp.Message); message != "" {
		detail["providerMessage"] = message
	}

	_, err = tx.Client().PaymentOrder.UpdateOneID(p.OrderID).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		ClearRefundAt().
		SetForceRefund(p.Force).
		ClearFailedAt().
		ClearFailedReason().
		Save(txCtx)
	if err != nil {
		return nil, fmt.Errorf("mark refund pending: %w", err)
	}
	if err := createPaymentAuditLog(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", "admin", detail); err != nil {
		return nil, fmt.Errorf("persist refund pending state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund pending state: %w", err)
	}
	if rollbackOK {
		p.BalanceToDeduct = 0
		p.SubDaysToDeduct = 0
	}

	warning := "gateway refund is pending confirmation"
	if !rollbackOK {
		warning += "; refund deduction rollback failed"
	}
	return &RefundResult{Success: false, Warning: warning}, nil
}

func boundedRefundProviderMessage(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	const maxProviderMessageLength = 512
	if len(message) > maxProviderMessageLength {
		return message[:maxProviderMessageLength] + "..."
	}
	return message
}

func refundResponseID(resp *payment.RefundResponse) string {
	if resp == nil {
		return ""
	}
	return strings.TrimSpace(resp.RefundID)
}

func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if _, err := s.userRepo.AdjustBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "balanceDeducted": p.BalanceToDeduct})
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, p.SubDaysToDeduct); err != nil {
			slog.Error("[CRITICAL] subscription rollback failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "subDaysDeducted": p.SubDaysToDeduct})
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
