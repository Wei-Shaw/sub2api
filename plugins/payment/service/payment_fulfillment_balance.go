package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// Audit action constants used by the fulfillment routines. Centralising
// them keeps the audit-log vocabulary discoverable and prevents the
// classic "AFFILIATE_REBATE_APPLED" typo class of bugs.
const (
	auditActionRechargeSuccess     = "RECHARGE_SUCCESS"
	auditActionSubscriptionSuccess = "SUBSCRIPTION_SUCCESS"
	auditActionAffiliateApplied    = "AFFILIATE_REBATE_APPLIED"
	auditActionAffiliateSkipped    = "AFFILIATE_REBATE_SKIPPED"
	auditActionAffiliateFailed     = "AFFILIATE_REBATE_FAILED"
)

// rebateBaseAmountKey is the field name used in audit-log details to
// record the order amount that drove the rebate calculation.
const rebateBaseAmountKey = "baseAmount"

// ExecuteBalanceFulfillment is the explicit balance-credit entry point.
// Idempotent: a second call on a COMPLETED order is a no-op.
func (s *PaymentService) ExecuteBalanceFulfillment(ctx context.Context, oid int64) error {
	if s == nil || s.entClient == nil {
		return errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if err := validateFulfillmentPreconditions(o); err != nil {
		return err
	}
	c, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(int(oid)), paymentorder.StatusIn(OrderStatusPaid, OrderStatusFailed)).
		SetStatus(OrderStatusRecharging).Save(ctx)
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

// validateFulfillmentPreconditions encapsulates the status guard rails
// shared between balance and subscription fulfillment. Returns nil when
// the order is in a fulfillable state.
func validateFulfillmentPreconditions(o *pluginent.PaymentOrder) error {
	if o.Status == OrderStatusCompleted {
		return errAlreadyCompleted
	}
	if psIsRefundStatus(o.Status) {
		return infraerrors.BadRequest("INVALID_STATUS", "refund-related order cannot fulfill")
	}
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFailed {
		return infraerrors.BadRequest("INVALID_STATUS", "order cannot fulfill in status "+o.Status)
	}
	return nil
}

// errAlreadyCompleted is returned by validateFulfillmentPreconditions to
// short-circuit the fulfillment pipeline without surfacing an error.
var errAlreadyCompleted = errors.New("payment: order already completed")

// doBalance credits the user's balance via the host SDK. Idempotency is
// delegated to the host (keyed by IdempotencyKey=out_trade_no) so the
// plugin no longer needs to mint or look up redemption codes.
func (s *PaymentService) doBalance(ctx context.Context, o *pluginent.PaymentOrder) error {
	if s.host == nil {
		return errors.New("host client unavailable")
	}
	_, err := s.host.CreditBalance(ctx, pluginsdk.CreditBalanceInput{
		UserID:         o.UserID,
		Amount:         o.Amount,
		Reason:         fmt.Sprintf("payment_order:%d", o.ID),
		IdempotencyKey: o.OutTradeNo,
		Source:         "payment",
	})
	if err != nil {
		if errors.Is(err, pluginsdk.ErrHostBalanceUnavailable) {
			return infraerrors.ServiceUnavailable("HOST_BALANCE_UNAVAILABLE", "host balance service is unavailable")
		}
		return fmt.Errorf("credit balance: %w", err)
	}
	if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, auditActionRechargeSuccess)
}

// markCompleted flips a RECHARGING order to COMPLETED and emits the
// payment.order.fulfilled plugin event.
func (s *PaymentService) markCompleted(ctx context.Context, o *pluginent.PaymentOrder, auditAction string) error {
	now := time.Now()
	_, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging)).
		SetStatus(OrderStatusCompleted).
		SetCompletedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	s.writeAuditLog(ctx, int64(o.ID), auditAction, "system", map[string]any{
		"rechargeCode":   o.RechargeCode,
		"creditedAmount": o.Amount,
		"payAmount":      o.PayAmount,
	})
	s.publishOrderFulfilled(ctx, o, auditAction, now)
	return nil
}

func (s *PaymentService) publishOrderFulfilled(ctx context.Context, o *pluginent.PaymentOrder, auditAction string, fulfilledAt time.Time) {
	if s == nil || s.events == nil {
		return
	}
	ev := &pluginsdk.HostEvent{
		EventType: pluginsdk.EventTypePaymentOrderFulfilled,
		Payload: &pb.HostEvent_PaymentOrderFulfilled{
			PaymentOrderFulfilled: &pluginsdk.PaymentOrderFulfilled{
				OrderId:             int64(o.ID),
				UserId:              o.UserID,
				AmountCents:         yuanToFen(o.Amount),
				BizType:             o.OrderType,
				AuditAction:         auditAction,
				FulfilledAtUnixNano: fulfilledAt.UnixNano(),
			},
		},
	}
	if err := s.events.Publish(ctx, ev); err != nil && s.logger != nil {
		s.logger.Warn("publish payment.order.fulfilled failed", "order_id", o.ID, "error", err)
	}
}

// applyAffiliateRebateForOrder fires the host's affiliate rebate accrual
// for balance orders. Idempotency-keyed on the order id; the host
// short-circuits to AlreadyApplied=true on retries. Errors are recorded
// in the audit log but do NOT abort the fulfillment — a missed rebate
// is recoverable, but losing the user's balance credit because the
// invitee table is offline is not.
func (s *PaymentService) applyAffiliateRebateForOrder(ctx context.Context, o *pluginent.PaymentOrder) error {
	if !shouldAccrueAffiliateRebate(o, s.host) {
		return nil
	}
	in := pluginsdk.AccrueRebateInput{
		InviteeUserID:  o.UserID,
		OrderAmount:    o.Amount,
		IdempotencyKey: fmt.Sprintf("rebate:%d", o.ID),
	}
	result, err := s.host.AccrueRebate(ctx, in)
	if err != nil {
		// Host capability not granted is treated as "no inviter" — the
		// payment platform should still complete fulfillment.
		if errors.Is(err, pluginsdk.ErrHostAffiliateUnavailable) {
			return nil
		}
		s.writeAuditLog(ctx, int64(o.ID), auditActionAffiliateFailed, "system", map[string]any{"error": err.Error()})
		return nil
	}
	s.recordRebateOutcome(ctx, o, result)
	return nil
}

func shouldAccrueAffiliateRebate(o *pluginent.PaymentOrder, host pluginsdk.HostPaymentClient) bool {
	if o == nil || host == nil {
		return false
	}
	if o.OrderType != payment.OrderTypeBalance {
		return false
	}
	return o.Amount.IsPositive()
}

func (s *PaymentService) recordRebateOutcome(ctx context.Context, o *pluginent.PaymentOrder, result *pluginsdk.AccrueRebateResult) {
	if result == nil {
		return
	}
	if result.AlreadyApplied {
		// Host already recorded the rebate; do not duplicate the audit row.
		return
	}
	if result.RebateAmount.Sign() <= 0 {
		s.writeAuditLog(ctx, int64(o.ID), auditActionAffiliateSkipped, "system", map[string]any{
			rebateBaseAmountKey: o.Amount,
			"reason":            "no inviter bound or rebate amount <= 0",
		})
		return
	}
	s.writeAuditLog(ctx, int64(o.ID), auditActionAffiliateApplied, "system", map[string]any{
		rebateBaseAmountKey: o.Amount,
		"rebateAmount":      result.RebateAmount,
		"inviterUserId":     result.InviterUserID,
	})
}
