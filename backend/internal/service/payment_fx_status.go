package service

import (
	"math"
	"sort"
	"strings"
	"time"
)

type PaymentFXStatus struct {
	Source            string     `json:"source"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	StaleAfterSeconds int        `json:"stale_after_seconds"`
	Stale             bool       `json:"stale"`
	MissingCurrencies []string   `json:"missing_currencies"`
}

func buildPaymentFXStatus(cfg *PaymentConfig, vals map[string]string, now time.Time) PaymentFXStatus {
	if cfg == nil {
		return PaymentFXStatus{Source: defaultFXRatesSource, StaleAfterSeconds: defaultFXRatesStaleAfterSeconds, Stale: true}
	}
	staleAfterSeconds := pcParseInt(vals[SettingFXRatesStaleAfterSeconds], defaultFXRatesStaleAfterSeconds)
	if staleAfterSeconds <= 0 {
		staleAfterSeconds = defaultFXRatesStaleAfterSeconds
	}
	updatedAt := parseFXUpdatedAt(vals[SettingFXRatesUpdatedAt])
	status := PaymentFXStatus{
		Source:            normalizeFXSource(vals[SettingFXRatesSource]),
		UpdatedAt:         updatedAt,
		StaleAfterSeconds: staleAfterSeconds,
		MissingCurrencies: missingPaymentFXCurrencies(cfg.LedgerCurrency, cfg.AllowedPaymentCurrencies, cfg.ManualFXRates),
	}
	if status.Source == "" {
		status.Source = defaultFXRatesSource
	}
	if updatedAt == nil {
		status.Stale = true
	} else if now.UTC().After(updatedAt.UTC().Add(time.Duration(staleAfterSeconds) * time.Second)) {
		status.Stale = true
	}
	if len(status.MissingCurrencies) > 0 {
		status.Stale = true
	}
	return status
}

func parseFXUpdatedAt(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func normalizeFXSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "" {
		return defaultFXRatesSource
	}
	return source
}

func paymentFXCurrencies(ledgerCurrency string, paymentCurrencies []string) []string {
	currencies := append([]string{}, paymentCurrencies...)
	currencies = append(currencies, ledgerCurrency)
	return normalizeCurrencyList(currencies)
}

func validatePaymentFXRates(ledgerCurrency string, currencies []string, rates map[string]float64) (map[string]float64, []string) {
	ledgerCurrency = normalizeCurrencyCode(ledgerCurrency, defaultLedgerCurrency)
	out := map[string]float64{ledgerCurrency: 1}
	missing := make([]string, 0)
	for _, currency := range paymentFXCurrencies(ledgerCurrency, currencies) {
		if currency == ledgerCurrency {
			out[currency] = 1
			continue
		}
		rate := rates[normalizeCurrencyCode(currency, "")]
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
			missing = append(missing, currency)
			continue
		}
		out[currency] = rate
	}
	sort.Strings(missing)
	return out, missing
}

func missingPaymentFXCurrencies(ledgerCurrency string, paymentCurrencies []string, rates map[string]float64) []string {
	_, missing := validatePaymentFXRates(ledgerCurrency, paymentCurrencies, rates)
	return missing
}
