package service

import (
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestValidateProviderCurrencyAllowsMatchingProviderCurrency(t *testing.T) {
	tests := []struct {
		name        string
		providerKey string
		paymentType string
		currency    string
	}{
		{name: "sepay vnd", providerKey: payment.TypeSepay, paymentType: payment.TypeSepay, currency: "vnd"},
		{name: "wxpay cny", providerKey: payment.TypeWxpay, paymentType: payment.TypeWxpay, currency: "CNY"},
		{name: "alipay direct cny", providerKey: payment.TypeAlipay, paymentType: payment.TypeAlipayDirect, currency: "CNY"},
		{name: "stripe card cny", providerKey: payment.TypeStripe, paymentType: payment.TypeCard, currency: "CNY"},
		{name: "stripe card vnd", providerKey: payment.TypeStripe, paymentType: payment.TypeCard, currency: "VND"},
		{name: "stripe card krw", providerKey: payment.TypeStripe, paymentType: payment.TypeCard, currency: "KRW"},
		{name: "stripe link usd", providerKey: payment.TypeStripe, paymentType: payment.TypeLink, currency: "USD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProviderCurrency(tt.providerKey, tt.paymentType, tt.currency); err != nil {
				t.Fatalf("validateProviderCurrency returned error: %v", err)
			}
		})
	}
}

func TestValidateProviderCurrencyRejectsUnsupportedCurrency(t *testing.T) {
	tests := []struct {
		name        string
		providerKey string
		paymentType string
		currency    string
		supported   string
	}{
		{name: "sepay cny", providerKey: payment.TypeSepay, paymentType: payment.TypeSepay, currency: "CNY", supported: "VND"},
		{name: "wxpay vnd", providerKey: payment.TypeWxpay, paymentType: payment.TypeWxpay, currency: "VND", supported: "CNY"},
		{name: "stripe alipay vnd", providerKey: payment.TypeStripe, paymentType: payment.TypeAlipay, currency: "VND", supported: "CNY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderCurrency(tt.providerKey, tt.paymentType, tt.currency)
			if err == nil {
				t.Fatal("expected unsupported currency error")
			}
			appErr := new(infraerrors.ApplicationError)
			if !errors.As(err, &appErr) {
				t.Fatalf("error = %T, want ApplicationError", err)
			}
			if appErr.Reason != "UNSUPPORTED_PAYMENT_CURRENCY" {
				t.Fatalf("reason = %q, want UNSUPPORTED_PAYMENT_CURRENCY", appErr.Reason)
			}
			if appErr.Metadata["supported_currency"] != tt.supported {
				t.Fatalf("supported_currency = %q, want %q", appErr.Metadata["supported_currency"], tt.supported)
			}
		})
	}
}

func TestValidateProviderCurrencyAllowsUnknownProviderForCompatibility(t *testing.T) {
	if err := validateProviderCurrency(payment.TypePaddle, payment.TypePaddle, "USD"); err != nil {
		t.Fatalf("paddle currency should be delegated to provider for now: %v", err)
	}
}
