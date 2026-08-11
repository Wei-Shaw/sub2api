package dto

import (
	"math/big"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

const web3DepositCreditedAmountScale int32 = 8

type UserWeb3DepositAmounts struct {
	TokenAmount    string  `json:"token_amount"`
	CreditedAmount *string `json:"credited_amount"`
}

type AdminWeb3DepositAmounts struct {
	RawAmount      string  `json:"raw_amount"`
	TokenAmount    string  `json:"token_amount"`
	CreditedAmount *string `json:"credited_amount"`
}

func NewUserWeb3DepositAmounts(amounts web3deposit.DepositAmounts, credited bool) UserWeb3DepositAmounts {
	return UserWeb3DepositAmounts{
		TokenAmount:    amounts.TokenAmount.StringFixed(web3deposit.USDT0Decimals),
		CreditedAmount: formatWeb3DepositCreditedAmount(amounts, credited),
	}
}

func NewAdminWeb3DepositAmounts(rawAmount *big.Int, amounts web3deposit.DepositAmounts, credited bool) (AdminWeb3DepositAmounts, error) {
	if rawAmount == nil {
		return AdminWeb3DepositAmounts{}, web3deposit.ErrRawAmountRequired
	}
	return AdminWeb3DepositAmounts{
		RawAmount:      rawAmount.String(),
		TokenAmount:    amounts.TokenAmount.StringFixed(web3deposit.USDT0Decimals),
		CreditedAmount: formatWeb3DepositCreditedAmount(amounts, credited),
	}, nil
}

func formatWeb3DepositCreditedAmount(amounts web3deposit.DepositAmounts, credited bool) *string {
	if !credited {
		return nil
	}
	formatted := amounts.CreditedAmount.StringFixed(web3DepositCreditedAmountScale)
	return &formatted
}
