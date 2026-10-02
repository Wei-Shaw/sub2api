package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ExchangeRateInfo tells the frontend what an amount typed in one currency
// costs in the other, so the top-up form can preview the conversion with the
// same number the order will be priced at.
type ExchangeRateInfo struct {
	// GatewayCurrency is the currency the gateway actually settles in.
	GatewayCurrency string `json:"gateway_currency"`
	// Rate is how many units of GatewayCurrency one USD buys, markup included.
	Rate      string    `json:"rate"`
	FetchedAt time.Time `json:"fetched_at"`
	Source    string    `json:"source"`
}

// ExchangeRate returns the rate used to price orders for a payment type.
//
// It returns the same error the order path would, so a stale rate surfaces on
// the form rather than only at submit time.
func (s *PaymentService) ExchangeRate(ctx context.Context, paymentType string) (*ExchangeRateInfo, error) {
	methodCurrency := payment.DefaultPaymentCurrency
	if s.configService != nil {
		resolved, err := s.configService.ValidateMethodCurrencyConsistency(ctx, paymentType)
		if err != nil {
			return nil, err
		}
		methodCurrency = resolved
	}
	if strings.EqualFold(methodCurrency, "USD") {
		return &ExchangeRateInfo{GatewayCurrency: methodCurrency, Rate: "1", Source: "identity"}, nil
	}
	if s.exchangeRateService == nil {
		return nil, infraerrors.ServiceUnavailable("EXCHANGE_RATE_UNAVAILABLE",
			"exchange rate service is not configured")
	}
	rate, snapshot, err := s.exchangeRateService.EffectiveRate(ctx)
	if err != nil {
		return nil, err
	}
	return &ExchangeRateInfo{
		GatewayCurrency: methodCurrency,
		Rate:            rate.String(),
		FetchedAt:       snapshot.FetchedAt,
		Source:          snapshot.Source,
	}, nil
}
