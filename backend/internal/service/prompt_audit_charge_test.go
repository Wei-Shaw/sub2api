package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditChargeTakesEachFeeOnce(t *testing.T) {
	charge := NewPromptAuditCharge()
	charge.Add(0.0002, nil)
	charge.Add(0.0003, nil)
	charge.Add(0, nil)
	require.InDelta(t, 0.0005, charge.Take(), 1e-12)
	require.Zero(t, charge.Take())

	// A usage record billed on the request context takes the pending fee.
	ctx := WithPromptAuditCharge(context.Background(), charge)
	charge.Add(0.0004, nil)
	require.InDelta(t, 0.0004, promptAuditCostForBilling(ctx), 1e-12)
	require.Zero(t, promptAuditCostForBilling(ctx))

	// A fee fixed at submission wins over anything added to the charge later.
	charge.Add(0.0009, nil)
	fixed := WithPromptAuditCost(ctx, 0.0001)
	require.InDelta(t, 0.0001, promptAuditCostForBilling(fixed), 1e-12)
	require.InDelta(t, 0.0009, charge.Take(), 1e-12)

	var missing *PromptAuditCharge
	missing.Add(1, nil)
	require.Zero(t, missing.Take())
	require.Zero(t, promptAuditCostForBilling(context.Background()))
}

func promptAuditBillingParams(cost CostBreakdown, apiKey *APIKey, subscription *UserSubscription) *postUsageBillingParams {
	return &postUsageBillingParams{
		Cost: &cost, User: &User{ID: 7}, APIKey: apiKey, Account: &Account{ID: 9},
		Subscription:       subscription,
		IsSubscriptionBill: subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType(),
		APIKeyService:      (*APIKeyService)(nil), AccountRateMultiplier: 1,
	}
}

// The fee is billed at list price: it joins ActualCost and the balance charge,
// and stays out of TotalCost, which drives the upstream account's quota.
func TestPromptAuditCostJoinsBalanceChargeAtListPrice(t *testing.T) {
	apiKey := &APIKey{ID: 3, Quota: 10, RateLimit5h: 5, Group: &Group{ID: 10, RateMultiplier: 0.5}}
	p := promptAuditBillingParams(CostBreakdown{TotalCost: 1, ActualCost: 0.5}, apiKey, nil)
	usageLog := &UsageLog{TotalCost: 1, ActualCost: 0.5}
	ctx := WithPromptAuditCost(context.Background(), 0.0003)

	addPromptAuditCost(ctx, usageLog, p)
	cmd := buildUsageBillingCommand("req-1", usageLog, p)

	require.InDelta(t, 0.5003, cmd.BalanceCost, 1e-12)
	require.Zero(t, cmd.SubscriptionCost)
	require.InDelta(t, 0.5003, cmd.APIKeyQuotaCost, 1e-12)
	require.InDelta(t, 0.5003, cmd.APIKeyRateLimitCost, 1e-12)
	require.InDelta(t, 1, p.Cost.TotalCost, 1e-12)
	require.InDelta(t, 0.0003, usageLog.PromptAuditCost, 1e-12)
	require.InDelta(t, 0.5003, usageLog.ActualCost, 1e-12)
	require.InDelta(t, 1, usageLog.TotalCost, 1e-12)
}

// A subscription request whose model costs nothing still bills the audit fee
// to the subscription, never to the balance.
func TestPromptAuditCostOnFreeSubscriptionRequestBillsSubscription(t *testing.T) {
	apiKey := &APIKey{ID: 3, Group: &Group{ID: 19, SubscriptionType: SubscriptionTypeSubscription}}
	subscription := &UserSubscription{ID: 42}
	p := promptAuditBillingParams(CostBreakdown{}, apiKey, subscription)
	usageLog := &UsageLog{}

	addPromptAuditCost(WithPromptAuditCost(context.Background(), 0.0002), usageLog, p)
	cmd := buildUsageBillingCommand("req-2", usageLog, p)

	require.NotNil(t, cmd.SubscriptionID)
	require.Equal(t, int64(42), *cmd.SubscriptionID)
	require.InDelta(t, 0.0002, cmd.SubscriptionCost, 1e-12)
	require.Zero(t, cmd.BalanceCost)
}

func TestPromptAuditCostSkipsSimpleModeAndZeroFee(t *testing.T) {
	apiKey := &APIKey{ID: 3, RateLimit5h: 5}
	p := promptAuditBillingParams(CostBreakdown{TotalCost: 1, ActualCost: 1}, apiKey, nil)
	p.SimpleModeKeyRateLimitOnly = true
	usageLog := &UsageLog{ActualCost: 1}
	addPromptAuditCost(WithPromptAuditCost(context.Background(), 0.0003), usageLog, p)
	require.Zero(t, usageLog.PromptAuditCost)
	require.InDelta(t, 1, p.Cost.ActualCost, 1e-12)

	p = promptAuditBillingParams(CostBreakdown{TotalCost: 1, ActualCost: 1}, apiKey, nil)
	addPromptAuditCost(context.Background(), usageLog, p)
	require.Zero(t, p.PromptAuditCost)
	require.InDelta(t, 1, p.Cost.ActualCost, 1e-12)
}

func TestPromptAuditChargeFinishBillsOnlyUntakenFees(t *testing.T) {
	var billed []float64
	chargeFee := func(_ context.Context, cost float64) error {
		billed = append(billed, cost)
		return nil
	}
	charge := NewPromptAuditCharge()
	charge.Add(0.0002, chargeFee)
	charge.Add(0.0003, chargeFee)
	cost, err := charge.Finish(context.Background())
	require.NoError(t, err)
	require.InDelta(t, 0.0005, cost, 1e-12)
	require.Len(t, billed, 1)
	require.InDelta(t, 0.0005, billed[0], 1e-12)

	cost, err = charge.Finish(context.Background())
	require.NoError(t, err)
	require.Zero(t, cost)
	require.Len(t, billed, 1)

	orphan := NewPromptAuditCharge()
	orphan.Add(0.0004, nil)
	cost, err = orphan.Finish(context.Background())
	require.Error(t, err)
	require.InDelta(t, 0.0004, cost, 1e-12)
}
