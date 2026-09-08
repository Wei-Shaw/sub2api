package web3deposit

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"
)

const USDT0Decimals int32 = 6

var (
	ErrRawAmountZero           = errors.New("web3 deposit raw amount must be positive")
	ErrRawAmountExceedsUint256 = errors.New("web3 deposit raw amount exceeds uint256")
	ErrCreditedAmountOverflow  = errors.New("web3 deposit credited amount exceeds DECIMAL(20,8)")

	maxPlatformBalance = decimal.RequireFromString("999999999999.99999999")
)

type DepositAmounts struct {
	TokenAmount    decimal.Decimal
	CreditedAmount decimal.Decimal
}

// ConvertUSDT0Amount converts an ERC20 uint256 raw amount into the exact
// six-decimal USDT0 token amount and the equal platform credit amount. It
// rejects zero, out-of-range uint256 values, and amounts that cannot fit in
// the platform DECIMAL(20,8) balance range without using floating point.
func ConvertUSDT0Amount(rawAmount *big.Int) (DepositAmounts, error) {
	tokenAmount, err := ConvertUSDT0TokenAmount(rawAmount)
	if err != nil {
		return DepositAmounts{}, err
	}
	if tokenAmount.GreaterThan(maxPlatformBalance) {
		return DepositAmounts{}, ErrCreditedAmountOverflow
	}
	return DepositAmounts{
		TokenAmount:    tokenAmount,
		CreditedAmount: tokenAmount,
	}, nil
}

func ConvertUSDT0TokenAmount(rawAmount *big.Int) (decimal.Decimal, error) {
	if err := validateUint256(rawAmount); err != nil {
		return decimal.Zero, err
	}
	if rawAmount.Sign() == 0 {
		return decimal.Zero, ErrRawAmountZero
	}
	return decimal.NewFromBigInt(rawAmount, -USDT0Decimals), nil
}

func validateUint256(rawAmount *big.Int) error {
	if rawAmount == nil {
		return ErrRawAmountRequired
	}
	if rawAmount.Sign() < 0 {
		return ErrRawAmountNegative
	}
	if rawAmount.BitLen() > 256 {
		return ErrRawAmountExceedsUint256
	}
	return nil
}
