package service

import (
	"context"
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/payment/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// validateOrderInput is the entry-point order validator. Subscription orders
// are dispatched to validateSubOrder (which returns the matching plan);
// balance orders only need the amount-range checks.
func (s *PaymentService) validateOrderInput(ctx context.Context, req CreateOrderRequest, cfg *PaymentConfig) (*SubscriptionPlan, error) {
	if req.OrderType == payment.OrderTypeBalance && cfg.BalanceDisabled {
		return nil, infraerrors.Forbidden("BALANCE_PAYMENT_DISABLED", "balance recharge has been disabled")
	}
	if req.OrderType == payment.OrderTypeSubscription {
		return s.validateSubOrder(ctx, req)
	}
	// decimal.Decimal carries no NaN/Inf so the float64 IEEE-754 guard
	// is no longer required; a non-positive value is the only invalid
	// shape left.
	if !req.Amount.IsPositive() {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount must be a positive number")
	}
	if (cfg.MinAmount.IsPositive() && req.Amount.LessThan(cfg.MinAmount)) ||
		(cfg.MaxAmount.IsPositive() && req.Amount.GreaterThan(cfg.MaxAmount)) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{
				"min": cfg.MinAmount.StringFixed(2),
				"max": cfg.MaxAmount.StringFixed(2),
			})
	}
	return nil, nil
}

// validateSubOrder validates a subscription create-order request and returns
// the resolved plan. Group lookup is intentionally skipped: the plugin has
// no host-side groups capability today, so the plan-level checks are the
// only ones we can perform without reaching across the SDK boundary.
func (s *PaymentService) validateSubOrder(ctx context.Context, req CreateOrderRequest) (*SubscriptionPlan, error) {
	if req.PlanID == 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
	}
	plan, err := s.config.GetPlan(ctx, req.PlanID)
	if err != nil {
		// errPlansDBUnavailable is the documented stub error; surface as 503
		// so the user sees a meaningful retryable response instead of 500.
		if errors.Is(err, errPlansDBUnavailable) {
			return nil, infraerrors.ServiceUnavailable("PLANS_DB_UNAVAILABLE", "plan lookup is not available in this build")
		}
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found")
	}
	if plan == nil || !plan.ForSale {
		return nil, infraerrors.NotFound("PLAN_NOT_AVAILABLE", "plan not found or not for sale")
	}
	return plan, nil
}
