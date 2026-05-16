package payment

// AmountToMinorUnits is a compatibility wrapper around AmountToMinorUnit.
func AmountToMinorUnits(amountStr, currency string) (int64, error) {
	return AmountToMinorUnit(amountStr, currency)
}

// MinorUnitsToAmount is a compatibility wrapper around MinorUnitToAmount.
func MinorUnitsToAmount(amount int64, currency string) float64 {
	return MinorUnitToAmount(amount, currency)
}

// YuanToFen converts a CNY amount string (元) to minor units (分).
// This is WxPay-specific and always uses CNY regardless of DefaultPaymentCurrency.
func YuanToFen(yuanStr string) (int64, error) {
	return AmountToMinorUnit(yuanStr, "CNY")
}

// FenToYuan converts CNY minor units (分) back to a major-unit float (元).
// This is WxPay-specific and always uses CNY regardless of DefaultPaymentCurrency.
func FenToYuan(fen int64) float64 {
	return MinorUnitToAmount(fen, "CNY")
}
