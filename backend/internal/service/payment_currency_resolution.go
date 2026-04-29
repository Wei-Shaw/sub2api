package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *PaymentService) resolveRequestPaymentCurrency(ctx context.Context, req *CreateOrderRequest, cfg *PaymentConfig) error {
	if req == nil || req.paymentQuote != nil {
		return nil
	}
	requested := normalizeCurrencyCode(req.PaymentCurrency, "")
	methodCurrencies, err := s.methodPaymentCurrencies(ctx, req.PaymentType)
	if err != nil {
		return err
	}
	if requested != "" {
		req.PaymentCurrency = requested
		if len(methodCurrencies) > 0 && !isAllowedPaymentCurrency(requested, methodCurrencies) {
			return unsupportedPaymentCurrencyError("", req.PaymentType, requested, methodCurrencies)
		}
		if len(methodCurrencies) > 0 {
			ensureConfigAllowsPaymentCurrency(cfg, requested)
		}
		return nil
	}
	if len(methodCurrencies) == 1 {
		req.PaymentCurrency = methodCurrencies[0]
		ensureConfigAllowsPaymentCurrency(cfg, req.PaymentCurrency)
		return nil
	}
	if len(methodCurrencies) > 1 {
		return infraerrors.BadRequest("PAYMENT_CURRENCY_REQUIRED", "payment_currency is required for this payment method").
			WithMetadata(map[string]string{
				"payment_type":         strings.TrimSpace(req.PaymentType),
				"supported_currencies": strings.Join(methodCurrencies, ","),
			})
	}
	fallback := defaultConfiguredPaymentCurrency(cfg)
	if fallback == "" {
		return infraerrors.BadRequest("PAYMENT_CURRENCY_REQUIRED", "payment_currency is required")
	}
	req.PaymentCurrency = fallback
	return nil
}

func (s *PaymentService) methodPaymentCurrencies(ctx context.Context, paymentType string) ([]string, error) {
	if s == nil || s.configService == nil {
		return nil, nil
	}
	currencies, err := s.configService.GetPaymentTypeCurrencies(ctx, paymentType)
	if err != nil {
		return nil, err
	}
	return normalizeCurrencyList(currencies), nil
}

func defaultConfiguredPaymentCurrency(cfg *PaymentConfig) string {
	if cfg == nil {
		return ""
	}
	if len(cfg.AllowedPaymentCurrencies) == 1 {
		return cfg.AllowedPaymentCurrencies[0]
	}
	ledgerCurrency := normalizeCurrencyCode(cfg.LedgerCurrency, defaultLedgerCurrency)
	if len(cfg.AllowedPaymentCurrencies) == 0 {
		return ledgerCurrency
	}
	for _, currency := range cfg.AllowedPaymentCurrencies {
		if currency == ledgerCurrency {
			return ledgerCurrency
		}
	}
	return ""
}

func ensureConfigAllowsPaymentCurrency(cfg *PaymentConfig, currency string) {
	if cfg == nil {
		return
	}
	currency = normalizeCurrencyCode(currency, "")
	if currency == "" || isAllowedPaymentCurrency(currency, cfg.AllowedPaymentCurrencies) {
		return
	}
	cfg.AllowedPaymentCurrencies = append(cfg.AllowedPaymentCurrencies, currency)
}

func selectionContextWithPaymentCurrency(ctx context.Context, currency string) context.Context {
	return payment.WithPaymentCurrency(ctx, currency)
}
