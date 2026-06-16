package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// ErrOrderNotFound signals the fulfillment pipeline could not locate the
// order referenced by an inbound webhook. The webhook handler relies on
// errors.Is(err, ErrOrderNotFound) to downgrade unknown-order responses
// to a 200 ack so the upstream provider stops retrying.
var ErrOrderNotFound = errors.New("payment order not found")

// HandlePaymentNotification is the webhook entry-point. It receives a
// parsed notification and the provider key the webhook was received on;
// the order is looked up by its out_trade_no, validated for amount /
// merchant identity / provider mismatch, transitioned to PAID, and
// then handed to the fulfillment pipeline.
func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	if s == nil || s.entClient == nil {
		return errPaymentServiceUnavailable
	}
	if n == nil || n.Status != payment.NotificationStatusSuccess {
		return nil
	}
	order, err := s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		if oid, ok := parseLegacyPaymentOrderID(n.OrderID, err); ok {
			return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk, n.Metadata)
		}
		if pluginent.IsNotFound(err) {
			return fmt.Errorf("%w: out_trade_no=%s", ErrOrderNotFound, n.OrderID)
		}
		return fmt.Errorf("lookup order failed for out_trade_no %s: %w", n.OrderID, err)
	}
	return s.confirmPayment(ctx, int64(order.ID), n.TradeNo, n.Amount, pk, n.Metadata)
}

// parseLegacyPaymentOrderID extracts the numeric order id encoded in a
// legacy "sub2_<id>" out_trade_no. Used as a fallback when the modern
// out_trade_no lookup misses (very old orders generated before the
// random-suffix scheme landed).
func parseLegacyPaymentOrderID(orderID string, lookupErr error) (int64, bool) {
	if !pluginent.IsNotFound(lookupErr) {
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

// confirmPayment validates an inbound notification against the persisted
// order (provider mismatch, merchant identity mismatch, amount mismatch)
// and transitions the order to PAID on success.
func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid decimal.Decimal, pk string, metadata map[string]string) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		if s.logger != nil {
			s.logger.Error("order not found", "order_id", oid)
		}
		return nil
	}
	if err := s.validateNotificationProvider(ctx, o, pk, tradeNo); err != nil {
		return err
	}
	if err := s.validateNotificationMetadata(ctx, o, pk, tradeNo, metadata); err != nil {
		return err
	}
	if err := s.validateNotificationAmount(ctx, o, pk, tradeNo, paid); err != nil {
		return err
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func (s *PaymentService) validateNotificationProvider(ctx context.Context, o *pluginent.PaymentOrder, pk, tradeNo string) error {
	instanceProviderKey := ""
	if inst, instErr := s.getOrderProviderInstance(ctx, o); instErr == nil && inst != nil {
		instanceProviderKey = inst.ProviderKey
	}
	expected := expectedNotificationProviderKeyForOrder(s.registry, o, instanceProviderKey)
	if expected == "" || strings.TrimSpace(pk) == "" || strings.EqualFold(expected, strings.TrimSpace(pk)) {
		return nil
	}
	s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_PROVIDER_MISMATCH", pk, map[string]any{
		"expectedProvider": expected,
		"actualProvider":   pk,
		"tradeNo":          tradeNo,
	})
	return fmt.Errorf("provider mismatch: expected %s, got %s", expected, pk)
}

func (s *PaymentService) validateNotificationMetadata(ctx context.Context, o *pluginent.PaymentOrder, pk, tradeNo string, metadata map[string]string) error {
	if err := validateProviderSnapshotMetadata(o, pk, metadata); err != nil {
		s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_PROVIDER_METADATA_MISMATCH", pk, map[string]any{
			"detail":  err.Error(),
			"tradeNo": tradeNo,
		})
		return err
	}
	return nil
}

func (s *PaymentService) validateNotificationAmount(ctx context.Context, o *pluginent.PaymentOrder, pk, tradeNo string, paid decimal.Decimal) error {
	if !isValidProviderAmount(paid) {
		s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_INVALID_AMOUNT", pk, map[string]any{
			"expected": o.PayAmount,
			"paid":     paid,
			"tradeNo":  tradeNo,
		})
		return fmt.Errorf("invalid paid amount from provider: %s", paid.String())
	}
	// Decimal-precision comparison in fen (integer cents) so a provider
	// rounding drift cannot let an attacker underpay by sub-cent amounts
	// — float64 subtraction with a 0.01 tolerance silently accepted up to
	// 1 cent of underpayment per order.
	if yuanToFen(paid) != yuanToFen(o.PayAmount) {
		s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{
			"expected": o.PayAmount,
			"paid":     paid,
			"tradeNo":  tradeNo,
			"order_id": o.ID,
		})
		return fmt.Errorf("amount mismatch: expected %s, got %s", o.PayAmount.StringFixed(2), paid.StringFixed(2))
	}
	return nil
}

// toPaid promotes the order to PAID inside an idempotent CAS update. The
// allowed source statuses are PENDING / CANCELLED / EXPIRED-within-grace.
// Returns nil after starting fulfillment; alreadyProcessed handles the
// race where another webhook beat us to the same order.
func (s *PaymentService) toPaid(ctx context.Context, o *pluginent.PaymentOrder, tradeNo string, paid decimal.Decimal, pk string) error {
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
	).SetStatus(OrderStatusPaid).
		SetPayAmount(paid).
		SetPaymentTradeNo(tradeNo).
		SetPaidAt(now).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		return s.alreadyProcessed(ctx, o)
	}
	s.recordPaidTransitionAudit(ctx, o, previousStatus, tradeNo, paid, pk)
	return s.executeFulfillment(ctx, int64(o.ID))
}

func (s *PaymentService) recordPaidTransitionAudit(ctx context.Context, o *pluginent.PaymentOrder, previousStatus, tradeNo string, paid decimal.Decimal, pk string) {
	if previousStatus == OrderStatusCancelled || previousStatus == OrderStatusExpired {
		if s.logger != nil {
			s.logger.Info("order recovered from webhook payment success",
				"order_id", o.ID,
				"previous_status", previousStatus,
				"trade_no", tradeNo,
				"provider", pk,
			)
		}
		s.writeAuditLog(ctx, int64(o.ID), "ORDER_RECOVERED", pk, map[string]any{
			"previous_status": previousStatus,
			"tradeNo":         tradeNo,
			"paidAmount":      paid,
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, int64(o.ID), "ORDER_PAID", pk, map[string]any{
		"tradeNo":    tradeNo,
		"paidAmount": paid,
	})
}

// alreadyProcessed handles the path where toPaid's CAS update hit zero
// rows — i.e. another webhook (or our own retry) already moved the
// order forward. The action is dictated by the CURRENT status.
func (s *PaymentService) alreadyProcessed(ctx context.Context, o *pluginent.PaymentOrder) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return nil
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded:
		return nil
	case OrderStatusFailed:
		return s.executeFulfillment(ctx, int64(o.ID))
	case OrderStatusPaid, OrderStatusRecharging:
		return fmt.Errorf("order %d is being processed", o.ID)
	case OrderStatusExpired:
		if s.logger != nil {
			s.logger.Warn("webhook payment success for expired order beyond grace period",
				"order_id", o.ID,
				"status", cur.Status,
				"updated_at", cur.UpdatedAt,
			)
		}
		s.writeAuditLog(ctx, int64(o.ID), "PAYMENT_AFTER_EXPIRY", "system", map[string]any{
			"status":    cur.Status,
			"updatedAt": cur.UpdatedAt,
			"reason":    "payment arrived after expiry grace period",
		})
		return nil
	default:
		return nil
	}
}

// executeFulfillment dispatches PAID orders to the per-biz-type
// fulfillment routine.
func (s *PaymentService) executeFulfillment(ctx context.Context, oid int64) error {
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}
	if o.OrderType == payment.OrderTypeSubscription {
		return s.ExecuteSubscriptionFulfillment(ctx, oid)
	}
	return s.ExecuteBalanceFulfillment(ctx, oid)
}

// RetryFulfillment is the admin-initiated retry path. The order must
// already be PAID (a webhook arrived) and not in a refund state.
func (s *PaymentService) RetryFulfillment(ctx context.Context, oid int64) error {
	if s == nil || s.entClient == nil {
		return errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if err := validateRetryPreconditions(o); err != nil {
		return err
	}
	_, err = s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(int(oid)), paymentorder.StatusIn(OrderStatusFailed, OrderStatusPaid)).
		SetStatus(OrderStatusPaid).
		ClearFailedAt().
		ClearFailedReason().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("reset for retry: %w", err)
	}
	s.writeAuditLog(ctx, oid, "RECHARGE_RETRY", "admin", map[string]any{"detail": "admin manual retry"})
	return s.executeFulfillment(ctx, oid)
}

func validateRetryPreconditions(o *pluginent.PaymentOrder) error {
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
	return nil
}

// hasAuditLog reports whether the audit-log table contains any entry for
// (order_id, action). Used by the fulfillment pipeline to detect
// previous successful runs that crashed before status flipped to
// COMPLETED.
func (s *PaymentService) hasAuditLog(ctx context.Context, orderID int64, action string) bool {
	if s == nil || s.entClient == nil {
		return false
	}
	c, _ := s.entClient.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionEQ(action),
		).
		Limit(1).Count(ctx)
	return c > 0
}

// markFailed flips a RECHARGING order to FAILED with the supplied cause.
// The narrow CAS predicate (RECHARGING-only) prevents overwriting a
// COMPLETED order when markCompleted itself failed but the underlying
// fulfillment had already landed.
func (s *PaymentService) markFailed(ctx context.Context, oid int64, cause error) {
	now := time.Now()
	reason := psErrMsg(cause)
	c, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(int(oid)), paymentorder.StatusEQ(OrderStatusRecharging)).
		SetStatus(OrderStatusFailed).
		SetFailedAt(now).
		SetFailedReason(reason).
		Save(ctx)
	if err != nil && s.logger != nil {
		s.logger.Error("mark FAILED", "order_id", oid, "error", err)
	}
	if c > 0 {
		s.writeAuditLog(ctx, oid, "FULFILLMENT_FAILED", "system", map[string]any{"reason": reason})
	}
}
