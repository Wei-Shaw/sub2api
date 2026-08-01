//go:build unit

package provider

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestStripeNormalizeCurrencyDefaultsToCNY(t *testing.T) {
	if got := normalizeStripeCurrency(""); got != "CNY" {
		t.Fatalf("normalizeStripeCurrency empty = %q, want CNY", got)
	}
	if got := normalizeStripeCurrency(" vnd "); got != "VND" {
		t.Fatalf("normalizeStripeCurrency vnd = %q, want VND", got)
	}
}

func TestStripeParsePaymentIntentUsesEventCurrencyMinorUnits(t *testing.T) {
	intent := stripe.PaymentIntent{
		ID:       "pi_123",
		Amount:   12000,
		Currency: stripe.Currency("krw"),
		Metadata: map[string]string{"orderId": "order_123"},
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		t.Fatalf("marshal payment intent: %v", err)
	}
	event := &stripe.Event{Data: &stripe.EventData{Raw: raw}}

	notification, err := parseStripePaymentIntent(event, payment.ProviderStatusSuccess, string(raw))
	if err != nil {
		t.Fatalf("parseStripePaymentIntent returned error: %v", err)
	}
	if notification.Currency != "KRW" {
		t.Fatalf("currency = %q, want KRW", notification.Currency)
	}
	if math.Abs(notification.Amount-12000) > 1e-9 {
		t.Fatalf("amount = %f, want 12000", notification.Amount)
	}
}

func TestStripeMinorUnitConversionSupportsLocalCurrencies(t *testing.T) {
	tests := []struct {
		currency string
		amount   string
		want     int64
	}{
		{currency: "CNY", amount: "12.34", want: 1234},
		{currency: "USD", amount: "12.34", want: 1234},
		{currency: "VND", amount: "200000", want: 200000},
		{currency: "KRW", amount: "12000", want: 12000},
	}
	for _, tt := range tests {
		t.Run(tt.currency, func(t *testing.T) {
			got, err := payment.AmountToMinorUnits(tt.amount, tt.currency)
			if err != nil {
				t.Fatalf("AmountToMinorUnits returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("minor units = %d, want %d", got, tt.want)
			}
		})
	}
}
