package service

import (
	"math"
	"strings"
)

// normalizeCurrencyOverrides sanitizes a currency override map:
// - Uppercases all currency keys
// - Removes entries with non-positive or non-finite values
// - Returns nil if the result is empty (to avoid storing empty JSON objects)
func normalizeCurrencyOverrides(overrides map[string]float64) map[string]float64 {
	if len(overrides) == 0 {
		return nil
	}
	result := make(map[string]float64, len(overrides))
	for currency, amount := range overrides {
		code := strings.ToUpper(strings.TrimSpace(currency))
		if code == "" || len(code) != 3 {
			continue
		}
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			continue
		}
		result[code] = amount
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// resolveCurrencyOverride checks if a currency override exists for the given payment currency.
// Returns the override amount and true if found, or 0 and false if not.
func resolveCurrencyOverride(overrides map[string]float64, paymentCurrency string) (float64, bool) {
	if len(overrides) == 0 {
		return 0, false
	}
	code := strings.ToUpper(strings.TrimSpace(paymentCurrency))
	if code == "" {
		return 0, false
	}
	amount, ok := overrides[code]
	if !ok || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, false
	}
	return amount, true
}
