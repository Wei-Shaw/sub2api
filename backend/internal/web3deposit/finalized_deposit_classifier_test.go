package web3deposit

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestClassifyFinalizedDepositAmountUsesInclusiveAutoCreditBoundaries(t *testing.T) {
	config := ChainConfig{
		MinimumDeposit:  decimal.RequireFromString("1.000000"),
		AutoCreditLimit: decimal.RequireFromString("10000.000000"),
	}
	tests := []struct {
		amount     string
		wantStatus DepositStatus
		wantReason string
	}{
		{amount: "0.999999", wantStatus: DepositStatusBelowMinimum},
		{amount: "1.000000", wantStatus: DepositStatusReadyToCredit},
		{amount: "9999.999999", wantStatus: DepositStatusReadyToCredit},
		{amount: "10000.000000", wantStatus: DepositStatusReadyToCredit},
		{amount: "10000.000001", wantStatus: DepositStatusManualReview, wantReason: ReviewReasonAboveAutoCreditLimit},
	}

	for _, test := range tests {
		classification, err := ClassifyFinalizedDepositAmount(test.amount, config)

		require.NoError(t, err)
		require.Equal(t, test.wantStatus, classification.Status)
		require.Equal(t, test.wantReason, classification.ReviewReason)
	}
}

func TestClassifyFinalizedDepositAmountRejectsInvalidAmount(t *testing.T) {
	_, err := ClassifyFinalizedDepositAmount("not-an-amount", ChainConfig{})

	require.ErrorIs(t, err, ErrFinalizedDepositAmountInvalid)
}

func TestClassifyFinalizedDepositAmountFailsSafeAbovePlatformRange(t *testing.T) {
	classification, err := ClassifyFinalizedDepositAmount(
		"1000000000000.000000",
		ChainConfig{MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000)},
	)

	require.NoError(t, err)
	require.Equal(t, DepositStatusFailed, classification.Status)
	require.Equal(t, FailureReasonAmountExceedsPlatformBalance, classification.FailureReason)
}
