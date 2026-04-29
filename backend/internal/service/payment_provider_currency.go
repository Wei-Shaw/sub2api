package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func validateProviderCurrency(providerKey, paymentType, paymentCurrency string, config map[string]string) error {
	currency := normalizeCurrencyCode(paymentCurrency, "")
	if currency == "" {
		return nil
	}
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)

	supported := payment.ProviderDefaultPaymentCurrencies(providerKey, paymentType)
	if len(config) > 0 {
		inst := &dbent.PaymentProviderInstance{ProviderKey: providerKey}
		supported = payment.InstancePaymentCurrencies(inst, paymentType, config)
	}
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
