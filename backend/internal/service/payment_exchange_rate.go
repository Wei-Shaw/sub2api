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
	// TierRate is VND per 1 USD for gateways that don't settle in VND. Recharge
	// bonus tiers are priced in VND, so the form needs it to preview the bonus.
	TierRate string `json:"tier_rate,omitempty"`
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
		info := &ExchangeRateInfo{GatewayCurrency: methodCurrency, Rate: "1", Source: "identity"}
		// Best effort: without it the form just shows no bonus; the order path decides.
		if s.exchangeRateService != nil {
			if rate, _, err := s.exchangeRateService.EffectiveRate(ctx); err == nil && rate.IsPositive() {
				info.TierRate = rate.String()
			}
		}
		return info, nil
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
