package service

import (
	"context"
	"fmt"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
)

// CancelOrder is the user-initiated cancel path. Validates ownership and
// the PENDING precondition; the rest of the work — including upstream
// reconciliation — happens inside cancelCore.
func (s *PaymentService) CancelOrder(ctx context.Context, orderID, userID int64) (string, error) {
	if s == nil || s.entClient == nil {
		return "", errLifecycleServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
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

// AdminCancelOrder is the operator path; identical guarantees minus
// the ownership check.
func (s *PaymentService) AdminCancelOrder(ctx context.Context, orderID int64) (string, error) {
	if s == nil || s.entClient == nil {
		return "", errLifecycleServiceUnavailable
	}
	o, err := s.entClient.PaymentOrder.Get(ctx, int(orderID))
	if err != nil {
		return "", infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.Status != OrderStatusPending {
		return "", infraerrors.BadRequest("INVALID_STATUS", "order cannot be cancelled in current status")
	}
	return s.cancelCore(ctx, o, OrderStatusCancelled, "admin", "admin cancelled order")
}

// cancelCore is the merged implementation behind CancelOrder /
// AdminCancelOrder / ExpireTimedOutOrders. Before transitioning the
// status it asks the upstream provider whether the user actually paid
// — a successful upstream response promotes the order to PAID via the
// fulfillment pipeline and the cancel is suppressed.
func (s *PaymentService) cancelCore(ctx context.Context, o *pluginent.PaymentOrder, fs, op, ad string) (string, error) {
	if o.PaymentTradeNo != "" || o.PaymentType != "" {
		if s.checkPaid(ctx, o) == checkPaidResultAlreadyPaid {
			return checkPaidResultAlreadyPaid, nil
		}
	}
	c, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusPending)).
		SetStatus(fs).Save(ctx)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
	}
	if c > 0 {
		auditAction := "ORDER_CANCELLED"
		if fs == OrderStatusExpired {
			auditAction = "ORDER_EXPIRED"
		}
		s.writeAuditLog(ctx, int64(o.ID), auditAction, op, map[string]any{"detail": ad})
		if fs == OrderStatusExpired {
			return checkPaidResultExpired, nil
		}
	}
	return checkPaidResultCancelled, nil
}
