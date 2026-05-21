package service

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PurchaseSubscriptionWithBalance buys a subscription plan using the user's account balance.
// Plan prices are stored in CNY; the balance recharge multiplier converts CNY to USD balance.
// e.g. plan price ¥9.9 with multiplier 10 → deducts $99 from the user's USD balance.
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

	// Convert plan price (CNY) to balance deduction amount (USD).
	payCfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	multiplier := normalizeBalanceRechargeMultiplier(payCfg.BalanceRechargeMultiplier)
	deductAmount := decimal.NewFromFloat(quote.Amount).
		Mul(decimal.NewFromFloat(multiplier)).
		Round(2).InexactFloat64()

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != payment.EntityStatusActive {
		return nil, infraerrors.Forbidden("USER_INACTIVE", "user account is disabled")
	}
	if user.CashBalance+1e-9 < deductAmount {
		return nil, insufficientBalanceError(user.CashBalance, deductAmount)
	}

	now := time.Now()
	validityDays := psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
	// Reuse normal order creation so subscription fulfillment, audit log and order history stay consistent.
	order, err := s.createOrderInTx(ctx, CreateOrderRequest{
		UserID:      userID,
		PaymentType: "balance",
		OrderType:   payment.OrderTypeSubscription,
		PlanID:      planID,
	}, user, plan, &PaymentConfig{OrderTimeoutMin: 24 * 60, MaxPendingOrders: 1000}, deductAmount, deductAmount, 0, deductAmount, nil)
	if err != nil {
		return nil, err
	}
	updated, err := s.entClient.User.Update().
		Where(dbuser.IDEQ(userID), dbuser.CashBalanceGTE(deductAmount)).
		AddBalance(-deductAmount).
		AddCashBalance(-deductAmount).
		Save(ctx)
	if err != nil {
		_ = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason("deduct balance failed").Exec(ctx)
		return nil, fmt.Errorf("deduct balance: %w", err)
	}
	if updated == 0 {
		_ = s.entClient.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).SetFailedAt(now).SetFailedReason("insufficient balance").Exec(ctx)
		fresh, freshErr := s.userRepo.GetByID(ctx, userID)
		if freshErr == nil && fresh != nil {
			return nil, insufficientBalanceError(fresh.CashBalance, deductAmount)
		}
		return nil, insufficientBalanceError(user.CashBalance, deductAmount)
	}

	s.writeAuditLog(ctx, order.ID, "BALANCE_PURCHASE_DEDUCTED", "system", map[string]any{
		"amount":            deductAmount,
		"planPriceCNY":      quote.Amount,
		"multiplier":        multiplier,
		"cashBalanceBefore": user.CashBalance,
		"cashBalanceAfter":  user.CashBalance - deductAmount,
		"planID":            plan.ID,
		"groupID":           plan.GroupID,
		"purchaseAction":    quote.Action,
	})
	if err := s.toPaid(ctx, order, fmt.Sprintf("BALANCE-%d", order.ID), deductAmount, "balance"); err != nil {
		// Roll back the balance deduction if subscription fulfillment fails.
		if rollbackErr := s.userRepo.UpdateBalance(ctx, userID, deductAmount); rollbackErr != nil {
			s.writeAuditLog(ctx, order.ID, "BALANCE_PURCHASE_ROLLBACK_FAILED", "system", map[string]any{"error": rollbackErr.Error(), "amount": deductAmount})
		}
		return nil, err
	}

	return &BalanceSubscriptionPurchaseResponse{
		OrderID:          order.ID,
		PlanID:           plan.ID,
		GroupID:          plan.GroupID,
		Amount:           deductAmount,
		BalanceBefore:    user.CashBalance,
		BalanceAfter:     user.CashBalance - deductAmount,
		SubscriptionDays: validityDays,
		Status:           OrderStatusCompleted,
		CompletedAt:      now,
		Action:           quote.Action,
	}, nil
}

func insufficientBalanceError(balance float64, required float64) error {
	return infraerrors.Conflict("INSUFFICIENT_BALANCE", "充值余额不足，赠送余额不能购买套餐，请联系 QQ 591719412 充值后再购买").WithMetadata(map[string]string{
		"cash_balance": fmt.Sprintf("%.2f", balance),
		"balance":      fmt.Sprintf("%.2f", balance),
		"required":     fmt.Sprintf("%.2f", required),
		"qq":           "591719412",
	})
}
