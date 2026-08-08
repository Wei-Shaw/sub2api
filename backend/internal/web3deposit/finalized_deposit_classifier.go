package web3deposit

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var ErrFinalizedDepositAmountInvalid = errors.New("finalized web3 deposit amount is invalid")

const (
	ReviewReasonAboveAutoCreditLimit = "amount_above_auto_credit_limit"
)

type FinalizedDepositClassification struct {
	Status       DepositStatus
	ReviewReason string
}

func ClassifyFinalizedDepositAmount(tokenAmount string, config ChainConfig) (FinalizedDepositClassification, error) {
	amount, err := decimal.NewFromString(tokenAmount)
	if err != nil || !amount.IsPositive() {
		return FinalizedDepositClassification{}, fmt.Errorf("%w: %q", ErrFinalizedDepositAmountInvalid, tokenAmount)
	}
	if amount.LessThan(config.MinimumDeposit) {
		return FinalizedDepositClassification{Status: DepositStatusBelowMinimum}, nil
	}
	if amount.GreaterThan(config.AutoCreditLimit) {
		return FinalizedDepositClassification{
			Status:       DepositStatusManualReview,
			ReviewReason: ReviewReasonAboveAutoCreditLimit,
		}, nil
	}
	return FinalizedDepositClassification{Status: DepositStatusReadyToCredit}, nil
}
