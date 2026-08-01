package payment

// AmountToMinorUnits is a compatibility wrapper around AmountToMinorUnit.
func AmountToMinorUnits(amountStr, currency string) (int64, error) {
	return AmountToMinorUnit(amountStr, currency)
}

// MinorUnitsToAmount is a compatibility wrapper around MinorUnitToAmount.
func MinorUnitsToAmount(amount int64, currency string) float64 {
	return MinorUnitToAmount(amount, currency)
}

func YuanToFen(yuanStr string) (int64, error) {
	return AmountToMinorUnit(yuanStr, DefaultPaymentCurrency)
}

func FenToYuan(fen int64) float64 {
	return MinorUnitToAmount(fen, DefaultPaymentCurrency)
}
