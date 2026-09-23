package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetModelPricing_JevPaidBillsNonZero 覆盖 P1：付费 jev-1.13 必须解析出
// 非零输入价（$0.042/MTok），不得落入 pricing_missing_record_zero_cost。
func TestGetModelPricing_JevPaidBillsNonZero(t *testing.T) {
	svc := newTestBillingService()

	paid, err := svc.GetModelPricing("jev-1.13")
	require.NoError(t, err)
	require.InDelta(t, 0.042e-6, paid.InputPricePerToken, 1e-12)
	require.Zero(t, paid.OutputPricePerToken)
	inputCost := paid.InputPricePerToken * 1e6
	require.InDelta(t, 0.042, inputCost, 1e-12)
	require.Greater(t, inputCost, 0.0)

	free, err := svc.GetModelPricing("jev-1.13-free")
	require.NoError(t, err)
	require.Zero(t, free.InputPricePerToken)
	require.Zero(t, free.OutputPricePerToken)
}
