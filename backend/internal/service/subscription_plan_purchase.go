package service

import (
	"context"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SubscriptionPurchaseActionNew              = "new"
	SubscriptionPurchaseActionExtend           = "extend"
	SubscriptionPurchaseActionUpgrade          = "upgrade"
	SubscriptionPurchaseActionBlockedDowngrade = "blocked_downgrade"
)

const subscriptionBillingMonthSeconds = 30 * 24 * 60 * 60

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

	groupIDs := make([]int64, 0, len(subs))
	seen := map[int64]bool{}
	for _, sub := range subs {
		if !seen[sub.GroupID] {
			seen[sub.GroupID] = true
			groupIDs = append(groupIDs, sub.GroupID)
		}
	}
	plans, err := s.entClient.SubscriptionPlan.Query().
		Where(subscriptionplan.GroupIDIn(groupIDs...), subscriptionplan.ForSaleEQ(true)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	planByGroup := make(map[int64]*dbent.SubscriptionPlan, len(plans))
	for _, p := range plans {
		if current, ok := planByGroup[p.GroupID]; !ok || normalizedMonthlyPlanPrice(p) > normalizedMonthlyPlanPrice(current) {
			planByGroup[p.GroupID] = p
		}
	}

	var currentSub *UserSubscription
	var currentPlan *dbent.SubscriptionPlan
	for i := range subs {
		sub := &subs[i]
		p := planByGroup[sub.GroupID]
		if p == nil {
			continue
		}
		if currentSub == nil || normalizedMonthlyPlanPrice(p) > normalizedMonthlyPlanPrice(currentPlan) {
			currentSub = sub
			currentPlan = p
		}
	}
	if currentSub == nil || currentPlan == nil {
		return quote, nil
	}

	now := time.Now()
	remaining := int64(currentSub.ExpiresAt.Sub(now).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	quote.CurrentPlanID = currentPlan.ID
	quote.CurrentPlanName = currentPlan.Name
	quote.CurrentGroupID = currentSub.GroupID
	quote.CurrentExpiresAt = &currentSub.ExpiresAt
	quote.RemainingSeconds = remaining
	quote.CurrentMonthlyPrice = normalizedMonthlyPlanPrice(currentPlan)
	quote.TargetMonthlyPrice = normalizedMonthlyPlanPrice(target)

	if currentSub.GroupID == target.GroupID {
		quote.Action = SubscriptionPurchaseActionExtend
		quote.Amount = roundMoney(target.Price)
		quote.DisplayAmount = quote.Amount
		return quote, nil
	}

	priceDelta := quote.TargetMonthlyPrice - quote.CurrentMonthlyPrice
	if priceDelta <= 0 {
		quote.Action = SubscriptionPurchaseActionBlockedDowngrade
		quote.Blocked = true
		quote.Reason = "当前只支持升档；低档套餐不能直接购买或预约降档。"
		quote.Amount = 0
		quote.DisplayAmount = 0
		return quote, nil
	}

	rawFee := priceDelta * float64(remaining) / subscriptionBillingMonthSeconds
	fee := math.Ceil(rawFee)
	if fee < 1 {
		fee = 1
	}
	quote.Action = SubscriptionPurchaseActionUpgrade
	quote.Amount = fee
	quote.DisplayAmount = fee
	quote.RemainingCredit = math.Max(0, quote.CurrentMonthlyPrice*float64(remaining)/subscriptionBillingMonthSeconds)
	return quote, nil
}

func (s *PaymentService) ApplySubscriptionPlanPurchase(ctx context.Context, userID int64, planID int64, notes string) (*UserSubscription, string, error) {
	quote, plan, err := s.QuoteSubscriptionPlanPurchase(ctx, userID, planID)
	if err != nil {
		return nil, "", err
	}
	if quote.Blocked {
		return nil, quote.Action, infraerrors.Conflict("SUBSCRIPTION_DOWNGRADE_NOT_ALLOWED", quote.Reason)
	}
	if quote.Action != SubscriptionPurchaseActionUpgrade {
		sub, _, err := s.subscriptionSvc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{UserID: userID, GroupID: plan.GroupID, ValidityDays: psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit), AssignedBy: 0, Notes: notes})
		return sub, quote.Action, err
	}

	current, err := s.subscriptionSvc.userSubRepo.GetByUserIDAndGroupID(ctx, userID, quote.CurrentGroupID)
	if err != nil {
		return nil, quote.Action, err
	}
	// If an old row already exists for the target group, remove it first to avoid the soft-delete-aware unique constraint.
	if existingTarget, targetErr := s.subscriptionSvc.userSubRepo.GetByUserIDAndGroupID(ctx, userID, plan.GroupID); targetErr == nil && existingTarget != nil && existingTarget.ID != current.ID {
		if existingTarget.IsActive() {
			return existingTarget, quote.Action, nil
		}
		if err := s.subscriptionSvc.userSubRepo.Delete(ctx, existingTarget.ID); err != nil {
			return nil, quote.Action, fmt.Errorf("remove old target subscription: %w", err)
		}
	}
	oldGroupID := current.GroupID
	current.GroupID = plan.GroupID
	if notes != "" {
		if current.Notes != "" {
			current.Notes += "\n"
		}
		current.Notes += notes
	}
	if err := s.subscriptionSvc.userSubRepo.Update(ctx, current); err != nil {
		return nil, quote.Action, err
	}
	s.subscriptionSvc.InvalidateSubCache(userID, oldGroupID)
	s.subscriptionSvc.InvalidateSubCache(userID, plan.GroupID)
	if s.subscriptionSvc.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.subscriptionSvc.billingCacheService.InvalidateSubscription(cacheCtx, userID, oldGroupID)
			_ = s.subscriptionSvc.billingCacheService.InvalidateSubscription(cacheCtx, userID, plan.GroupID)
		}()
	}
	updated, err := s.subscriptionSvc.userSubRepo.GetByID(ctx, current.ID)
	return updated, quote.Action, err
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
