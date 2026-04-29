package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func validateProviderCurrency(providerKey, paymentType, paymentCurrency string, config map[string]string, capabilities payment.CurrencyCapabilityConfig) error {
	currency := normalizeCurrencyCode(paymentCurrency, "")
	if currency == "" {
		return nil
	}
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)

	inst := &dbent.PaymentProviderInstance{ProviderKey: providerKey}
	supported := payment.InstancePaymentCurrencies(inst, paymentType, config, capabilities)
	if len(supported) == 0 {
		return nil
	}
	for _, supportedCurrency := range supported {
		if currency == supportedCurrency {
			return nil
		}
	}
	return unsupportedPaymentCurrencyError(providerKey, paymentType, currency, supported)
}

func unsupportedPaymentCurrencyError(providerKey, paymentType, paymentCurrency string, supported []string) error {
	return infraerrors.BadRequest("UNSUPPORTED_PAYMENT_CURRENCY", "payment currency is not supported by selected provider").
		WithMetadata(map[string]string{
			"provider":           strings.TrimSpace(providerKey),
			"payment_type":       strings.TrimSpace(paymentType),
			"payment_currency":   normalizeCurrencyCode(paymentCurrency, ""),
			"supported_currency": strings.Join(supported, ","),
		})
}
