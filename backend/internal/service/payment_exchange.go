package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	fxSourceManual   = "manual"
	fxSourceIdentity = "identity"
)

type fxSnapshot struct {
	PaymentCurrency     string
	LedgerCurrency      string
	RatePaymentToLedger float64
	Source              string
	Timestamp           time.Time
}

func resolveFXSnapshot(paymentCurrency string, cfg *PaymentConfig, now time.Time) (fxSnapshot, error) {
	ledgerCurrency := normalizeCurrencyCode(cfg.LedgerCurrency, defaultLedgerCurrency)
	payCurrency := normalizeCurrencyCode(paymentCurrency, "")
	if payCurrency == "" {
		if len(cfg.AllowedPaymentCurrencies) > 0 {
			payCurrency = cfg.AllowedPaymentCurrencies[0]
		} else {
			payCurrency = ledgerCurrency
		}
	}
	if !isAllowedPaymentCurrency(payCurrency, cfg.AllowedPaymentCurrencies) {
		return fxSnapshot{}, fmt.Errorf("payment currency %s is not allowed", payCurrency)
	}
	if payCurrency == ledgerCurrency {
		return fxSnapshot{
			PaymentCurrency:     payCurrency,
			LedgerCurrency:      ledgerCurrency,
			RatePaymentToLedger: 1,
			Source:              fxSourceIdentity,
			Timestamp:           now.UTC(),
		}, nil
	}
	rate := cfg.ManualFXRates[payCurrency]
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return fxSnapshot{}, fmt.Errorf("manual fx rate not found for %s", payCurrency)
	}
	return fxSnapshot{
		PaymentCurrency:     payCurrency,
		LedgerCurrency:      ledgerCurrency,
		RatePaymentToLedger: rate,
		Source:              fxSourceManual,
		Timestamp:           now.UTC(),
	}, nil
}

func isAllowedPaymentCurrency(currency string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	currency = normalizeCurrencyCode(currency, "")
	for _, c := range allowed {
		if normalizeCurrencyCode(c, "") == currency {
			return true
		}
	}
	return false
}

func currencyMinorUnits(currency string) int {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "VND", "KRW", "JPY":
		return 0
	default:
		return 2
	}
}

const floatRoundoffTolerance = 1e-7

func roundPaymentAmountForCollection(amount float64, currency string) float64 {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return amount
	}
	factor := math.Pow10(currencyMinorUnits(currency))
	return math.Ceil((amount-floatRoundoffTolerance)*factor) / factor
}

func roundLedgerAmountForCredit(amount float64, currency string) float64 {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return amount
	}
	factor := math.Pow10(currencyMinorUnits(currency))
	return math.Floor((amount+floatRoundoffTolerance)*factor) / factor
}

func convertPaymentToLedger(paymentAmount float64, snapshot fxSnapshot) (float64, error) {
	if math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) {
		return 0, fmt.Errorf("invalid payment amount")
	}
	if math.IsNaN(snapshot.RatePaymentToLedger) || math.IsInf(snapshot.RatePaymentToLedger, 0) || snapshot.RatePaymentToLedger <= 0 {
		return 0, fmt.Errorf("invalid fx rate")
	}
	ledgerAmount := paymentAmount * snapshot.RatePaymentToLedger
	if math.IsNaN(ledgerAmount) || math.IsInf(ledgerAmount, 0) {
		return 0, fmt.Errorf("invalid converted ledger amount")
	}
	return roundLedgerAmountForCredit(ledgerAmount, snapshot.LedgerCurrency), nil
}

func convertLedgerToPayment(ledgerAmount float64, snapshot fxSnapshot) (float64, error) {
	if math.IsNaN(ledgerAmount) || math.IsInf(ledgerAmount, 0) {
		return 0, fmt.Errorf("invalid ledger amount")
	}
	if math.IsNaN(snapshot.RatePaymentToLedger) || math.IsInf(snapshot.RatePaymentToLedger, 0) || snapshot.RatePaymentToLedger <= 0 {
		return 0, fmt.Errorf("invalid fx rate")
	}
	paymentAmount := ledgerAmount / snapshot.RatePaymentToLedger
	if math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) {
		return 0, fmt.Errorf("invalid converted payment amount")
	}
	return roundPaymentAmountForCollection(paymentAmount, snapshot.PaymentCurrency), nil
}

func currencyAmountTolerance(currency string) float64 {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "VND", "KRW", "JPY":
		return 1
	default:
		return 0.01
	}
}

func currencyAmountMatches(received, expected float64, currency string) bool {
	if math.IsNaN(received) || math.IsInf(received, 0) || math.IsNaN(expected) || math.IsInf(expected, 0) {
		return false
	}
	return math.Abs(received-expected) <= currencyAmountTolerance(currency)
}

func formatCurrencyAmountForProvider(amount float64, currency string) string {
	minorUnits := currencyMinorUnits(currency)
	return strconv.FormatFloat(amount, 'f', minorUnits, 64)
}
