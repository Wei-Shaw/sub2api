package service

import (
	"context"
	"fmt"
	"time"

	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PurchaseSubscriptionWithBalance buys a subscription plan using the user's account balance.
func (s *PaymentService) PurchaseSubscriptionWithBalance(ctx context.Context, userID int64, planID int64) (*BalanceSubscriptionPurchaseResponse, error) {
	if planID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription plan is required")
	}

	plan, err := s.validateSubOrder(ctx, CreateOrderRequest{UserID: userID, OrderType: payment.OrderTypeSubscription, PlanID: planID})
	if err != nil {
		return nil, err
	}
	quote, err := s.quoteSubscriptionPlanPurchaseForPlan(ctx, userID, plan)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}
	if user.Balance+1e-9 < quote.Amount {
		return nil, insufficientBalanceError(user.Balance, quote.Amount)
	}

	now := time.Now()
	validityDays := psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
	// Reuse normal order creation so subscription fulfillment, audit log and order history stay consistent.
	order, err := s.createOrderInTx(ctx, CreateOrderRequest{
		UserID:      userID,
		PaymentType: "balance",
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      planID,
	}, user, plan, &PaymentConfig{OrderTimeoutMin: 24 * 60, MaxPendingOrders: 1000}, quote.Amount, quote.Amount, 0, quote.Amount, nil)
	if err != nil {
		return nil, err
	}
	updated, err := s.entClient.User.Update().
		Where(dbuser.IDEQ(userID), dbuser.BalanceGTE(quote.Amount)).
		AddBalance(-quote.Amount).
		Save(ctx)
	if err != nil {
		_ = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason("deduct balance failed").Exec(ctx)
		return nil, fmt.Errorf("deduct balance: %w", err)
	}
	if updated == 0 {
		_ = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason("insufficient balance").Exec(ctx)
		fresh, freshErr := s.userRepo.GetByID(ctx, userID)
		if freshErr == nil && fresh != nil {
			return nil, insufficientBalanceError(fresh.Balance, quote.Amount)
		}
		return nil, insufficientBalanceError(user.Balance, quote.Amount)
	}

	s.writeAuditLog(ctx, order.ID, "BALANCE_PURCHASE_DEDUCTED", "system", map[string]any{
		"amount":         quote.Amount,
		"balanceBefore":  user.Balance,
		"balanceAfter":   user.Balance - quote.Amount,
		"planID":         plan.ID,
		"groupID":        plan.GroupID,
		"purchaseAction": quote.Action,
	})
	if err := s.toPaid(ctx, order, fmt.Sprintf("BALANCE-%d", order.ID), quote.Amount, "balance"); err != nil {
		// Roll back the balance deduction if subscription fulfillment fails.
		if rollbackErr := s.userRepo.UpdateBalance(ctx, userID, quote.Amount); rollbackErr != nil {
			s.writeAuditLog(ctx, order.ID, "BALANCE_PURCHASE_ROLLBACK_FAILED", "system", map[string]any{"error": rollbackErr.Error(), "amount": quote.Amount})
		}
		return nil, err
	}

	return &BalanceSubscriptionPurchaseResponse{
		OrderID:          order.ID,
		PlanID:           plan.ID,
		GroupID:          plan.GroupID,
		Amount:           quote.Amount,
		BalanceBefore:    user.Balance,
		BalanceAfter:     user.Balance - quote.Amount,
		SubscriptionDays: validityDays,
		Status:           OrderStatusCompleted,
		CompletedAt:      now,
		Action:           quote.Action,
	}, nil
}

func insufficientBalanceError(balance float64, required float64) error {
	return infraerrors.Conflict("INSUFFICIENT_BALANCE", "账户余额不足，请联系 QQ 591719412 充值后再购买").WithMetadata(map[string]string{
		"balance":  fmt.Sprintf("%.2f", balance),
		"required": fmt.Sprintf("%.2f", required),
		"qq":       "591719412",
	})
}
