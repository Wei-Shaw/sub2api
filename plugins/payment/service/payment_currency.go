package service

import (
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// paymentProviderConfigCurrency extracts the currency from a provider's
// decrypted config. Only providers that support multi-currency (Stripe,
// Airwallex) store a "currency" key; all others default to CNY.
func paymentProviderConfigCurrency(providerKey string, cfg map[string]string) string {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeStripe, payment.TypeAirwallex:
		currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
		if err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}

// PaymentOrderCurrency returns the currency recorded in an order's
// provider snapshot, falling back to the default payment currency.
func PaymentOrderCurrency(order *pluginent.PaymentOrder) string {
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if currency, err := payment.NormalizePaymentCurrency(snapshot.Currency); err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}
