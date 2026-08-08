package web3deposit

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var ErrFinalizedDepositAmountInvalid = errors.New("finalized web3 deposit amount is invalid")

const (
	ReviewReasonAboveAutoCreditLimit = "amount_above_auto_credit_limit"
	ReviewReasonUserMissing          = "user_missing"
	ReviewReasonUserDeleted          = "user_deleted"
	ReviewReasonUserInactive         = "user_inactive"
	ReviewReasonAddressMissing       = "deposit_address_missing"
	ReviewReasonAddressDisabled      = "deposit_address_disabled"
	ReviewReasonAddressUserMismatch  = "deposit_address_user_mismatch"
	ReviewReasonAddressMismatch      = "deposit_address_mismatch"
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

type DepositCreditEligibility struct {
	Eligible     bool
	ReviewReason string
}

type DepositCreditEligibilitySource interface {
	CheckCreditEligibility(ctx context.Context, deposit Deposit) (DepositCreditEligibility, error)
}
