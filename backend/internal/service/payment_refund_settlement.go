package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Refund deduction settlement: what a refund holds, gives back and takes
// again as it moves through execution, the gateway's pending state and its
// final answer. The pending audit is the record these steps share.

// settlePendingRefundDeduction decides what a confirmed pending refund
// deducts. The pending step gave back what execution had deducted, so the
// confirmation takes back exactly that, which already reflects the
// administrator's choice; it never adds a deduction the administrator declined.
// Audits written before the choice was recorded fall back to the amounts they
// rolled back, then to the order itself.
func (s *PaymentService) settlePendingRefundDeduction(ctx context.Context, o *dbent.PaymentOrder, plan *RefundPlan, pending refundPendingAuditDetail) *RefundResult {
	if !pending.DeductionRollbackOK {
		// Compensation failed, so the execution-time deduction is still held.
		plan.BalanceToDeduct = 0
		plan.SubDaysToDeduct = 0
		return nil
	}
	if pending.DeductBalance != nil && !*pending.DeductBalance {
		plan.DeductBalance = false
		plan.DeductionType = payment.DeductionTypeNone
		plan.BalanceToDeduct = 0
		plan.SubDaysToDeduct = 0
		return nil
	}
	if o.OrderType == payment.OrderTypeSubscription {
		if pending.SubDaysRolledBack != nil {
			plan.SubDaysToDeduct = *pending.SubDaysRolledBack
		}
		if pending.SubscriptionAdjustment != nil {
			plan.DeductionType = payment.DeductionTypeSubscription
			plan.SubscriptionID = pending.SubscriptionAdjustment.SubscriptionID
			plan.subscriptionAdjustment = pending.SubscriptionAdjustment
			return nil
		}
		if plan.SubDaysToDeduct <= 0 {
			plan.SubDaysToDeduct = 0
			return nil
		}
		return s.prepDeduct(ctx, o, plan, true)
	}
	if pending.BalanceRolledBack != nil {
		plan.BalanceToDeduct = *pending.BalanceRolledBack
	}
	return nil
}

// pendingRefundHeldDeduction rebuilds the deduction a pending refund still
// holds because its compensation failed. The pending audit describes the order
// only while it is REFUND_PENDING. A subscription deduction recorded before
// adjustments were kept cannot be identified and stays held for a retry.
func pendingRefundHeldDeduction(o *dbent.PaymentOrder, pending refundPendingAuditDetail) *RefundPlan {
	if o == nil || o.Status != OrderStatusRefundPending || !pending.Found || pending.DeductionRollbackOK {
		return nil
	}
	p := &RefundPlan{OrderID: o.ID, Order: o}
	switch {
	case pending.SubscriptionAdjustment != nil:
		p.DeductionType = payment.DeductionTypeSubscription
		p.SubscriptionID = pending.SubscriptionAdjustment.SubscriptionID
		p.SubDaysToDeduct = pending.SubDaysDeducted
		p.subscriptionAdjustment = pending.SubscriptionAdjustment
	case pending.BalanceDeducted > 0:
		p.DeductionType = payment.DeductionTypeBalance
		p.BalanceToDeduct = pending.BalanceDeducted
	default:
		return nil
	}
	return p
}

// refundDeductionStillHeld reports whether an earlier attempt left its
// deduction in place because the compensation failed (REFUND_ROLLBACK_FAILED).
// A later REFUND_ROLLBACK_RECOVERED gave it back, so a retry deducts again.
func (s *PaymentService) refundDeductionStillHeld(ctx context.Context, orderID int64) (bool, error) {
	latest, err := s.entClient.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)),
			paymentauditlog.ActionIn("REFUND_ROLLBACK_FAILED", "REFUND_ROLLBACK_RECOVERED"),
		).
		Order(paymentauditlog.ByCreatedAt(sql.OrderDesc()), paymentauditlog.ByID(sql.OrderDesc())).
		First(ctx)
	if dbent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read refund rollback audit: %w", err)
	}
	return latest.Action == "REFUND_ROLLBACK_FAILED", nil
}

func writeRefundAuditTx(ctx context.Context, client *dbent.Client, orderID int64, action string, detail map[string]any) error {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal %s audit: %w", action, err)
	}
	if _, err := client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(orderID, 10)).
		SetAction(action).
		SetDetail(string(encoded)).
		SetOperator("admin").
		Save(ctx); err != nil {
		return fmt.Errorf("write %s audit: %w", action, err)
	}
	return nil
}

// recordRefundPending moves the order from REFUNDING to REFUND_PENDING and
// writes its audit in one transaction, so a pending order always carries the
// audit that settles it. Without rollbackFailure it also gives back the
// execution-time deduction in that transaction; a compensation failure
// abandons the transaction and is returned as rollbackErr so the caller can
// record the pending state with the deduction still held.
func (s *PaymentService) recordRefundPending(ctx context.Context, p *RefundPlan, detail, rollbackFailure map[string]any) (rollbackErr, err error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refund pending: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if rollbackFailure == nil {
		if compensationErr := s.rollbackRefundDeduction(txCtx, p); compensationErr != nil {
			return compensationErr, nil
		}
	}
	updated, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusEQ(OrderStatusRefunding)).
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
	if updated == 0 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if rollbackFailure != nil {
		if err := writeRefundAuditTx(txCtx, tx.Client(), p.OrderID, "REFUND_ROLLBACK_FAILED", rollbackFailure); err != nil {
			return nil, err
		}
	}
	if err := writeRefundAuditTx(txCtx, tx.Client(), p.OrderID, "REFUND_PENDING", detail); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund pending: %w", err)
	}
	return nil, nil
}

// rollbackRefundDeduction gives back what refund execution deducted. A
// subscription is restored from the recorded adjustment, never by deleting or
// recreating the row.
func (s *PaymentService) rollbackRefundDeduction(ctx context.Context, p *RefundPlan) error {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if err := s.userRepo.UpdateBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			return fmt.Errorf("restore balance %.8f: %w", p.BalanceToDeduct, err)
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.subscriptionAdjustment != nil {
		if err := s.subscriptionSvc.restoreRefundSubscription(ctx, p.subscriptionAdjustment); err != nil {
			return fmt.Errorf("restore subscription %d: %w", p.subscriptionAdjustment.SubscriptionID, err)
		}
	}
	return nil
}

func refundRollbackFailedDetail(p *RefundPlan, gErr, rollbackErr error) map[string]any {
	return map[string]any{
		"gatewayError":           psErrMsg(gErr),
		"rollbackError":          psErrMsg(rollbackErr),
		"balanceDeducted":        p.BalanceToDeduct,
		"subDaysDeducted":        p.SubDaysToDeduct,
		"subscriptionAdjustment": p.subscriptionAdjustment,
	}
}
