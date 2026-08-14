package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// paymentProviderConfigCurrency reports the currency a provider instance
// settles in. SePay is fixed to dong by the banking rails it reads; NOWPayments
// prices in whatever fiat the admin configured.
func paymentProviderConfigCurrency(providerKey string, cfg map[string]string) string {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeSePay:
		return payment.CurrencySePay
	case payment.TypeNowPayments:
		if currency, err := payment.NormalizePaymentCurrency(cfg["currency"]); err == nil && strings.TrimSpace(cfg["currency"]) != "" {
			return currency
		}
		return payment.CurrencyNowPayments
	}
	return payment.DefaultPaymentCurrency
}

func PaymentOrderCurrency(order *dbent.PaymentOrder) string {
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if currency, err := payment.NormalizePaymentCurrency(snapshot.Currency); err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}
