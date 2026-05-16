package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

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

func PaymentOrderCurrency(order *dbent.PaymentOrder) string {
	// Try provider_snapshot first — it captures the currency at order-creation time
	// and is authoritative for legacy orders that predate the payment_currency column.
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if currency, err := payment.NormalizePaymentCurrency(snapshot.Currency); err == nil {
			return currency
		}
	}
	// Fall back to the explicit payment_currency column (may be DB default "CNY"
	// for legacy rows, but is correct for newer orders where snapshot lacks currency).
	if order.PaymentCurrency != "" {
		if currency, err := payment.NormalizePaymentCurrency(order.PaymentCurrency); err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}
