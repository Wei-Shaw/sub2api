package payment

import (
	"github.com/shopspring/decimal"
)

func CalculatePayAmount(rechargeAmount float64, feeRate float64) string {
	return CalculatePayAmountForCurrency(rechargeAmount, feeRate, DefaultPaymentCurrency)
}

// CalculatePayAmountForCurrency 按币种精度计算应付金额，手续费向上取整到该币种最小支付单位。
func CalculatePayAmountForCurrency(rechargeAmount float64, feeRate float64, currency string) string {
	fractionDigits := int32(CurrencyMaxFractionDigits(currency))
	amount := decimal.NewFromFloat(rechargeAmount)
	if feeRate <= 0 {
		return amount.StringFixed(fractionDigits)
	}
	rate := decimal.NewFromFloat(feeRate)
	fee := amount.Mul(rate).Div(decimal.NewFromInt(100)).RoundUp(fractionDigits)
	return amount.Add(fee).StringFixed(fractionDigits)
}
