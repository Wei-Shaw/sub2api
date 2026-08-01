//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type captureUsageBillingRepository struct {
	last *UsageBillingCommand
}

func (r *captureUsageBillingRepository) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.last = cmd
	return &UsageBillingApplyResult{Applied: false}, nil
}

func (*captureUsageBillingRepository) ReserveBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func (*captureUsageBillingRepository) CaptureBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func (*captureUsageBillingRepository) ReleaseBatchImageBalance(context.Context, *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return nil, nil
}

func TestBuildUsageBillingCommand_UsesPinnedUsageCostAfterCatalogPublicationChange(t *testing.T) {
	catalogEpoch := int64(7)
	catalogRevisionID := int64(31)
	requestedRevisionID := int64(20)
	effectiveRevisionID := int64(21)
	pricingSource := "catalog-source"

	usageLog := &UsageLog{
		UserID:                   1,
		APIKeyID:                 3,
		AccountID:                2,
		RequestID:                "usage-pinned-publication-change",
		Model:                    "catalog-model",
		BillingType:              BillingTypeBalance,
		InputTokens:              1000,
		OutputTokens:             500,
		TotalCost:                100,
		ActualCost:               200,
		CatalogEpoch:             &catalogEpoch,
		CatalogRevisionID:        &catalogRevisionID,
		RequestedModelRevisionID: &requestedRevisionID,
		EffectiveModelRevisionID: &effectiveRevisionID,
		PricingSource:            &pricingSource,
		PricingSnapshot: map[string]any{
			"input_cost_per_token":  0.000001,
			"output_cost_per_token": 0.000002,
			"_settlement_cost": map[string]any{
				"total_cost":  0.003,
				"actual_cost": 0.006,
			},
		},
	}

	// The current publication is different and a retry caller has already
	// resolved the new price into p.Cost. Settlement must not use it.
	currentPublicationCost := &CostBreakdown{
		TotalCost:  99,
		ActualCost: 199,
	}
	params := &postUsageBillingParams{
		Cost:    currentPublicationCost,
		User:    &User{ID: 1},
		APIKey:  &APIKey{ID: 3},
		Account: &Account{ID: 2},
	}

	first := buildUsageBillingCommand(usageLog.RequestID, usageLog, params)
	require.NotNil(t, first)
	require.Equal(t, 0.006, first.BalanceCost)
	require.NotEqual(t, currentPublicationCost.ActualCost, first.BalanceCost)
	require.Equal(t, int64(7), first.CatalogEpoch)
	require.NotEmpty(t, first.PricingSnapshotHash)

	// A retry after another publication must produce the same billing command,
	// even when its mutable/current pricing input is different again.
	params.Cost = &CostBreakdown{TotalCost: 123, ActualCost: 456}
	retry := buildUsageBillingCommand(usageLog.RequestID, usageLog, params)
	require.NotNil(t, retry)
	require.Equal(t, first.BalanceCost, retry.BalanceCost)
	require.Equal(t, first.RequestFingerprint, retry.RequestFingerprint)
	require.Equal(t, first.PricingSnapshotHash, retry.PricingSnapshotHash)
}

func TestMaterializeUsageSettlementSnapshot_IsValueOwnedAndUsedForRetry(t *testing.T) {
	catalogEpoch := int64(7)
	billingMode := "token"
	pricingSnapshot := map[string]any{"input_cost_per_token": 0.000001}
	usageLog := &UsageLog{
		CatalogEpoch:              &catalogEpoch,
		PricingSnapshot:           pricingSnapshot,
		InputCost:                 0.001,
		OutputCost:                0.002,
		TotalCost:                 0.003,
		ActualCost:                0.006,
		BillingMode:               &billingMode,
		LongContextBillingApplied: true,
	}

	err := materializeUsageSettlementSnapshot(usageLog)
	require.NoError(t, err)
	require.NotContains(t, pricingSnapshot, usageSettlementCostSnapshotKey)
	require.Equal(t, 0.006, pinnedUsageCostBreakdown(usageLog).ActualCost)
	require.Equal(t, 0.003, pinnedUsageCostBreakdown(usageLog).TotalCost)
	require.True(t, pinnedUsageCostBreakdown(usageLog).LongContextBillingApplied)
}

func TestApplyUsageBilling_RetryKeepsPinnedCostAfterCatalogPublicationChange(t *testing.T) {
	catalogEpoch := int64(7)
	pricingSource := "catalog-source"
	usageLog := &UsageLog{
		UserID:        1,
		APIKeyID:      3,
		AccountID:     2,
		RequestID:     "usage-apply-pinned-retry",
		Model:         "catalog-model",
		TotalCost:     100,
		ActualCost:    200,
		CatalogEpoch:  &catalogEpoch,
		PricingSource: &pricingSource,
		PricingSnapshot: map[string]any{
			"input_cost_per_token": 0.000001,
			"_settlement_cost": map[string]any{
				"total_cost":  0.003,
				"actual_cost": 0.006,
			},
		},
	}
	params := &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: 99, ActualCost: 199},
		User:    &User{ID: 1},
		APIKey:  &APIKey{ID: 3},
		Account: &Account{ID: 2},
	}
	repo := &captureUsageBillingRepository{}
	deps := &billingDeps{deferredService: NewDeferredService(nil, nil, 0)}

	applied, err := applyUsageBilling(context.Background(), usageLog.RequestID, usageLog, params, deps, repo)
	require.NoError(t, err)
	require.False(t, applied)
	require.NotNil(t, repo.last)
	require.Equal(t, 0.006, repo.last.BalanceCost)

	params.Cost = &CostBreakdown{TotalCost: 123, ActualCost: 456}
	applied, err = applyUsageBilling(context.Background(), usageLog.RequestID, usageLog, params, deps, repo)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, 0.006, repo.last.BalanceCost)
}
