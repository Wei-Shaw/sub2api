package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

const (
	auditActionRefundRollbackFailed       = "REFUND_ROLLBACK_FAILED"
	auditActionRefundGatewayFailed        = "REFUND_GATEWAY_FAILED"
	auditActionRefundFailed               = "REFUND_FAILED"
	auditActionRefundSuccess              = "REFUND_SUCCESS"
	auditActionRefundProviderMetadataMiss = "REFUND_PROVIDER_METADATA_MISMATCH"
	auditActionRefundNoTradeNo            = "REFUND_NO_TRADE_NO"
)

// ExecuteRefund executes a previously prepared RefundPlan. The state
// transitions are: COMPLETED → REFUNDING (CAS), then host-side
// deductions, then the upstream gateway refund call, finally
// REFUNDING → REFUNDED / PARTIALLY_REFUNDED on success.
func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	if s == nil || s.entClient == nil {
		return nil, errPaymentServiceUnavailable
	}
	if err := s.lockOrderForRefund(ctx, p); err != nil {
		return nil, err
	}
	if err := s.applyRefundDeductions(ctx, p); err != nil {
		return nil, err
	}
	if err := s.gwRefund(ctx, p); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.markRefundOk(ctx, p)
}

func (s *PaymentService) lockOrderForRefund(ctx context.Context, p *RefundPlan) error {
	c, err := s.entClient.PaymentOrder.Update().
		Where(
			paymentorder.IDEQ(int(p.OrderID)),
			paymentorder.StatusIn(OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed),
		).SetStatus(OrderStatusRefunding).Save(ctx)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	return nil
}

// applyRefundDeductions performs the host-side balance / subscription
// debits decided by PrepareRefund. Orders whose previous attempt
// crashed after the deduction landed (recorded via REFUND_ROLLBACK_FAILED)
// are skipped — the historical deduction is the source of truth.
func (s *PaymentService) applyRefundDeductions(ctx context.Context, p *RefundPlan) error {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct.IsPositive() {
		if s.hasAuditLog(ctx, p.OrderID, auditActionRefundRollbackFailed) {
			if s.logger != nil {
				s.logger.Warn("skipping balance deduction on retry (previous rollback failed)", "order_id", p.OrderID)
			}
			p.BalanceToDeduct = decimal.Zero
		} else if err := s.deductRefundBalance(ctx, p); err != nil {
			s.restoreStatus(ctx, p)
			return err
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 {
		if s.hasAuditLog(ctx, p.OrderID, auditActionRefundRollbackFailed) {
			if s.logger != nil {
				s.logger.Warn("skipping subscription deduction on retry (previous rollback failed)", "order_id", p.OrderID)
			}
			p.SubDaysToDeduct = 0
		} else if err := s.deductRefundSubscriptionDays(ctx, p); err != nil {
			s.restoreStatus(ctx, p)
			return err
		}
	}
	return nil
}

func (s *PaymentService) deductRefundBalance(ctx context.Context, p *RefundPlan) error {
	if s.host == nil {
		return errors.New("host client unavailable for refund deduction")
	}
	in := pluginsdk.DeductBalanceInput{
		UserID:         p.Order.UserID,
		Amount:         p.BalanceToDeduct,
		Reason:         fmt.Sprintf("refund_order:%d", p.Order.ID),
		IdempotencyKey: fmt.Sprintf("refund:%d", p.Order.ID),
		Source:         "payment_refund",
		AllowNegative:  true,
	}
	if _, err := s.host.DeductBalance(ctx, in); err != nil {
		if errors.Is(err, pluginsdk.ErrHostBalanceUnavailable) {
			return infraerrors.ServiceUnavailable("HOST_BALANCE_UNAVAILABLE", "host balance service is unavailable")
		}
		return fmt.Errorf("deduction: %w", err)
	}
	return nil
}

func (s *PaymentService) deductRefundSubscriptionDays(ctx context.Context, p *RefundPlan) error {
	if s.host == nil {
		return errors.New("host client unavailable for refund deduction")
	}
	if p.Order.SubscriptionGroupID == nil {
		return errors.New("refund: order is missing subscription_group_id")
	}
	in := pluginsdk.RevokeSubscriptionDaysInput{
		UserID:         p.Order.UserID,
		GroupID:        *p.Order.SubscriptionGroupID,
		Days:           p.SubDaysToDeduct,
		Reason:         "refund",
		IdempotencyKey: fmt.Sprintf("refund_sub:%d", p.Order.ID),
	}
	if _, err := s.host.RevokeSubscriptionDays(ctx, in); err != nil {
		if errors.Is(err, pluginsdk.ErrHostSubscriptionUnavailable) {
			return infraerrors.ServiceUnavailable("HOST_SUBSCRIPTION_UNAVAILABLE", "host subscription service is unavailable")
		}
		return fmt.Errorf("deduct subscription days: %w", err)
	}
	return nil
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) error {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, int64(p.Order.ID), auditActionRefundNoTradeNo, "admin", map[string]any{"detail": "skipped"})
		return nil
	}
	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, int64(p.Order.ID), auditActionRefundProviderMetadataMiss, "admin", map[string]any{
			"detail": err.Error(),
		})
		return err
	}
	_, err = prov.Refund(ctx, payment.RefundRequest{
		TradeNo: p.Order.PaymentTradeNo,
		OrderID: p.Order.OutTradeNo,
		Amount:  p.GatewayAmount.StringFixed(2),
		Reason:  p.Reason,
	})
	return err
}

// getRefundProvider creates a provider using the order's original instance
// config. Falls through to the same instance lookup the refund flow uses.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *pluginent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s.RollbackRefund(ctx, p, gErr) {
		s.restoreStatus(ctx, p)
		s.writeAuditLog(ctx, p.OrderID, auditActionRefundGatewayFailed, "admin", map[string]any{
			"detail": psErrMsg(gErr),
		})
		return &RefundResult{
			Success: false,
			Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back",
		}, nil
	}
	now := time.Now()
	_, _ = s.entClient.PaymentOrder.UpdateOneID(int(p.OrderID)).
		SetStatus(OrderStatusRefundFailed).
		SetFailedAt(now).
		SetFailedReason(psErrMsg(gErr)).
		Save(ctx)
	s.writeAuditLog(ctx, p.OrderID, auditActionRefundFailed, "admin", map[string]any{
		"detail": psErrMsg(gErr),
	})
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	finalStatus := OrderStatusRefunded
	if p.RefundAmount.LessThan(p.Order.Amount) {
		finalStatus = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	_, err := s.entClient.PaymentOrder.UpdateOneID(int(p.OrderID)).
		SetStatus(finalStatus).
		SetRefundAmount(p.RefundAmount).
		SetRefundReason(p.Reason).
		SetRefundAt(now).
		SetForceRefund(p.Force).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	s.writeAuditLog(ctx, p.OrderID, auditActionRefundSuccess, "admin", map[string]any{
		"refundAmount":    p.RefundAmount,
		"reason":          p.Reason,
		"balanceDeducted": p.BalanceToDeduct,
		"force":           p.Force,
	})
	return &RefundResult{
		Success:         true,
		BalanceDeducted: p.BalanceToDeduct,
		SubDaysDeducted: p.SubDaysToDeduct,
	}, nil
}

// RollbackRefund re-credits the user's balance / restores the
// subscription days when the upstream gateway refund failed after the
// host-side deduction landed. Returns true when every required rollback
// step succeeded; false leaves the order in REFUND_FAILED so an operator
// can investigate.
func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct.IsPositive() {
		if !s.rollbackBalance(ctx, p, gErr) {
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 {
		if !s.rollbackSubscriptionDays(ctx, p, gErr) {
			return false
		}
	}
	return true
}

func (s *PaymentService) rollbackBalance(ctx context.Context, p *RefundPlan, gErr error) bool {
	if s.host == nil {
		return false
	}
	in := pluginsdk.CreditBalanceInput{
		UserID:         p.Order.UserID,
		Amount:         p.BalanceToDeduct,
		Reason:         fmt.Sprintf("refund_rollback:%d", p.Order.ID),
		IdempotencyKey: fmt.Sprintf("refund_rollback:%d", p.Order.ID),
		Source:         "payment_refund_rollback",
	}
	if _, err := s.host.CreditBalance(ctx, in); err != nil {
		if s.logger != nil {
			s.logger.Error("[CRITICAL] balance rollback failed",
				"order_id", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
		}
		s.writeAuditLog(ctx, p.OrderID, auditActionRefundRollbackFailed, "admin", map[string]any{
			"gatewayError":    psErrMsg(gErr),
			"rollbackError":   psErrMsg(err),
			"balanceDeducted": p.BalanceToDeduct,
		})
		return false
	}
	return true
}

func (s *PaymentService) rollbackSubscriptionDays(ctx context.Context, p *RefundPlan, gErr error) bool {
	if s.host == nil || p.Order.SubscriptionGroupID == nil {
		return false
	}
	in := pluginsdk.AssignSubscriptionInput{
		UserID:         p.Order.UserID,
		GroupID:        *p.Order.SubscriptionGroupID,
		Days:           p.SubDaysToDeduct,
		Source:         "payment_refund_rollback",
		IdempotencyKey: fmt.Sprintf("refund_sub_rollback:%d", p.Order.ID),
	}
	if _, err := s.host.AssignSubscription(ctx, in); err != nil {
		if s.logger != nil {
			s.logger.Error("[CRITICAL] subscription rollback failed",
				"order_id", p.OrderID, "days", p.SubDaysToDeduct, "error", err)
		}
		s.writeAuditLog(ctx, p.OrderID, auditActionRefundRollbackFailed, "admin", map[string]any{
			"gatewayError":    psErrMsg(gErr),
			"rollbackError":   psErrMsg(err),
			"subDaysDeducted": p.SubDaysToDeduct,
		})
		return false
	}
	return true
}

func (s *PaymentService) restoreStatus(ctx context.Context, p *RefundPlan) {
	rs := OrderStatusCompleted
	if p.Order.Status == OrderStatusRefundRequested {
		rs = OrderStatusRefundRequested
	}
	_, _ = s.entClient.PaymentOrder.UpdateOneID(int(p.OrderID)).SetStatus(rs).Save(ctx)
}
