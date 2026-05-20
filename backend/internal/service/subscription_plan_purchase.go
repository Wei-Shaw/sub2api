package service

import (
	"context"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SubscriptionPurchaseActionNew    = "new"
	SubscriptionPurchaseActionExtend = "extend"
)

type SubscriptionPlanPurchaseQuote struct {
	Action              string     `json:"action"`
	Amount              float64    `json:"amount"`
	DisplayAmount       float64    `json:"display_amount"`
	Blocked             bool       `json:"blocked"`
	Reason              string     `json:"reason,omitempty"`
	CurrentPlanID       int64      `json:"current_plan_id,omitempty"`
	CurrentPlanName     string     `json:"current_plan_name,omitempty"`
	CurrentGroupID      int64      `json:"current_group_id,omitempty"`
	CurrentExpiresAt    *time.Time `json:"current_expires_at,omitempty"`
	RemainingSeconds    int64      `json:"remaining_seconds,omitempty"`
	RemainingCredit     float64    `json:"remaining_credit,omitempty"`
	TargetMonthlyPrice  float64    `json:"target_monthly_price,omitempty"`
	CurrentMonthlyPrice float64    `json:"current_monthly_price,omitempty"`
}

func (s *PaymentService) QuoteSubscriptionPlanPurchase(ctx context.Context, userID int64, planID int64) (*SubscriptionPlanPurchaseQuote, *dbent.SubscriptionPlan, error) {
	plan, err := s.validateSubOrder(ctx, CreateOrderRequest{UserID: userID, PlanID: planID, OrderType: "subscription"})
	if err != nil {
		return nil, nil, err
	}
	quote, err := s.quoteSubscriptionPlanPurchaseForPlan(ctx, userID, plan)
	if err != nil {
		return nil, nil, err
	}
	return quote, plan, nil
}

func (s *PaymentService) quoteSubscriptionPlanPurchaseForPlan(ctx context.Context, userID int64, target *dbent.SubscriptionPlan) (*SubscriptionPlanPurchaseQuote, error) {
	if target == nil {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription plan is required")
	}
	quote := &SubscriptionPlanPurchaseQuote{
		Action:        SubscriptionPurchaseActionNew,
		Amount:        roundMoney(target.Price),
		DisplayAmount: roundMoney(target.Price),
	}

	subs, err := s.subscriptionSvc.ListActiveUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return quote, nil
	}

	for i := range subs {
		if subs[i].GroupID == target.GroupID {
			quote.Action = SubscriptionPurchaseActionExtend
			quote.Amount = roundMoney(target.Price)
			quote.DisplayAmount = quote.Amount
			quote.CurrentPlanID = target.ID
			quote.CurrentPlanName = target.Name
			quote.CurrentGroupID = target.GroupID
			quote.CurrentExpiresAt = &subs[i].ExpiresAt
			remaining := int64(time.Until(subs[i].ExpiresAt).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			quote.RemainingSeconds = remaining
			quote.CurrentMonthlyPrice = normalizedMonthlyPlanPrice(target)
			quote.TargetMonthlyPrice = normalizedMonthlyPlanPrice(target)
			return quote, nil
		}
	}

	return quote, nil
}

func (s *PaymentService) ApplySubscriptionPlanPurchase(ctx context.Context, userID int64, planID int64, notes string) (*UserSubscription, string, error) {
	quote, plan, err := s.QuoteSubscriptionPlanPurchase(ctx, userID, planID)
	if err != nil {
		return nil, "", err
	}
	sub, _, err := s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: userID, GroupID: plan.GroupID, ValidityDays: psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit), AssignedBy: 0, Notes: notes})
	return sub, quote.Action, err
}

func normalizedMonthlyPlanPrice(plan *dbent.SubscriptionPlan) float64 {
	if plan == nil {
		return 0
	}
	days := psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
	if days <= 0 {
		days = 30
	}
	return plan.Price * 30 / float64(days)
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
