package service

import (
	"context"
	"errors"
	"fmt"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// ExecuteSubscriptionFulfillment is the explicit subscription-grant
// entry point. Pre-conditions match ExecuteBalanceFulfillment plus the
// requirement that the order carry subscription_group_id +
// subscription_days.
func (s *PaymentService) ExecuteSubscriptionFulfillment(ctx context.Context, oid int64) error {
	if s == nil || s.entClient == nil {
		return errPaymentServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(oid))
	if err != nil {
		return infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if err := validateFulfillmentPreconditions(o); err != nil {
		if errors.Is(err, errAlreadyCompleted) {
			return nil
		}
		return err
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil {
		return infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
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
	if err := s.doSub(ctx, o); err != nil {
		s.markFailed(ctx, oid, err)
		return err
	}
	return nil
}

// doSub assigns or extends the user's subscription via the host SDK and
// writes the SUBSCRIPTION_SUCCESS audit row. Idempotency is delegated to
// the host's AssignSubscription idempotency-key handling.
func (s *PaymentService) doSub(ctx context.Context, o *pluginent.PaymentOrder) error {
	if s.host == nil {
		return errors.New("host client unavailable")
	}
	groupID := *o.SubscriptionGroupID
	days := *o.SubscriptionDays
	// Belt-and-braces idempotency: if the audit log already records a
	// successful assignment for this order, skip the RPC.
	if s.hasAuditLog(ctx, int64(o.ID), auditActionSubscriptionSuccess) {
		if s.logger != nil {
			s.logger.Info("subscription already assigned for order, skipping",
				"order_id", o.ID, "group_id", groupID)
		}
		return s.markCompleted(ctx, o, auditActionSubscriptionSuccess)
	}
	in := pluginsdk.AssignSubscriptionInput{
		UserID:         o.UserID,
		GroupID:        groupID,
		Days:           days,
		Source:         "payment",
		IdempotencyKey: fmt.Sprintf("order:%d", o.ID),
	}
	if o.PlanID != nil {
		in.PlanID = *o.PlanID
	}
	if _, err := s.host.AssignSubscription(ctx, in); err != nil {
		if errors.Is(err, pluginsdk.ErrHostSubscriptionUnavailable) {
			return infraerrors.ServiceUnavailable("HOST_SUBSCRIPTION_UNAVAILABLE", "host subscription service is unavailable")
		}
		return fmt.Errorf("assign subscription: %w", err)
	}
	return s.markCompleted(ctx, o, auditActionSubscriptionSuccess)
}
