package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func validateProviderCurrency(providerKey, paymentType, paymentCurrency string) error {
	currency := normalizeCurrencyCode(paymentCurrency, "")
	if currency == "" {
		return nil
	}
	providerKey = strings.TrimSpace(providerKey)
	paymentType = strings.TrimSpace(paymentType)

	supported := supportedCurrenciesForProvider(providerKey, paymentType)
	if len(supported) == 0 {
		return nil
	}
	for _, supportedCurrency := range supported {
		if currency == supportedCurrency {
			return nil
		}
	}
	return infraerrors.BadRequest("UNSUPPORTED_PAYMENT_CURRENCY", "payment currency is not supported by selected provider").
		WithMetadata(map[string]string{
			"provider":           providerKey,
			"payment_type":       paymentType,
			"payment_currency":   currency,
			"supported_currency": strings.Join(supported, ","),
		})
}

func supportedCurrenciesForProvider(providerKey, paymentType string) []string {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeSepay:
		return []string{"VND"}
	case payment.TypeAlipay, payment.TypeWxpay, payment.TypeEasyPay:
		return []string{"CNY"}
	case payment.TypeStripe:
		switch payment.GetBasePaymentType(paymentType) {
		case payment.TypeAlipay, payment.TypeWxpay:
			return []string{"CNY"}
		case payment.TypeCard, payment.TypeLink, payment.TypeStripe:
			return []string{"USD", "CNY", "VND", "KRW"}
		default:
			return []string{"USD", "CNY", "VND", "KRW"}
		}
	}

	switch payment.GetBasePaymentType(strings.TrimSpace(paymentType)) {
	case payment.TypeAlipay, payment.TypeWxpay:
		return []string{"CNY"}
	case payment.TypeCard, payment.TypeLink, payment.TypeStripe:
		return []string{"USD", "CNY", "VND", "KRW"}
	case payment.TypeSepay:
		return []string{"VND"}
	default:
		return nil
	}
}
