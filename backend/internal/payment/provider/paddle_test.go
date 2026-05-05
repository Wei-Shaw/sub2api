//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestPaddleMinorUnitsUseCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
		want     string
	}{
		{name: "usd cents", amount: "12.34", currency: "USD", want: "1234"},
		{name: "vnd zero decimal", amount: "200000", currency: "VND", want: "200000"},
		{name: "krw zero decimal", amount: "12000", currency: "KRW", want: "12000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalAmountToMinorUnits(tt.amount, tt.currency)
			if err != nil {
				t.Fatalf("decimalAmountToMinorUnits returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decimalAmountToMinorUnits(%q, %q) = %q, want %q", tt.amount, tt.currency, got, tt.want)
			}
		})
	}
}

func TestPaddleMinorUnitsToDecimalUseCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name     string
		minor    string
		currency string
		want     float64
	}{
		{name: "usd cents", minor: "1234", currency: "USD", want: 12.34},
		{name: "vnd zero decimal", minor: "200000", currency: "VND", want: 200000},
		{name: "krw zero decimal", minor: "12000", currency: "KRW", want: 12000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := minorUnitsToDecimal(tt.minor, tt.currency)
			if err != nil {
				t.Fatalf("minorUnitsToDecimal returned error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("minorUnitsToDecimal(%q, %q) = %f, want %f", tt.minor, tt.currency, got, tt.want)
			}
		})
	}
}

func TestPaddleUsesSandboxAPIBaseWhenEnvironmentSandbox(t *testing.T) {
	p, err := NewPaddle("paddle-1", map[string]string{
		"apiKey":      "test_key",
		"environment": " sandbox ",
	})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	if got := p.apiBase(); got != paddleSandboxAPIBase {
		t.Fatalf("apiBase() = %q, want %q", got, paddleSandboxAPIBase)
	}
}

func TestPaddleAPIBaseOverrideWinsOverEnvironment(t *testing.T) {
	p, err := NewPaddle("paddle-1", map[string]string{
		"apiKey":      "test_key",
		"environment": "sandbox",
		"apiBase":     "https://example.test/paddle",
	})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	if got := p.apiBase(); got != "https://example.test/paddle" {
		t.Fatalf("apiBase() = %q, want apiBase override", got)
	}
}

func TestPaddleCreatePaymentSendsHostedCheckoutTransactionPayload(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get(paddleHeaderAuth); got != "Bearer test_key" {
			t.Fatalf("authorization header = %q, want bearer key", got)
		}
		var payload paddleTransactionPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("items len = %d, want 1", len(payload.Items))
		}
		item := payload.Items[0]
		if item.Price.UnitPrice.Amount != "5000" || item.Price.UnitPrice.CurrencyCode != "KRW" {
			t.Fatalf("unit_price = %+v, want 5000 KRW", item.Price.UnitPrice)
		}
		if item.Price.Product.Name != "V-Claw top up" || item.Price.Product.TaxCategory != "saas" {
			t.Fatalf("product = %+v, want V-Claw top up saas", item.Price.Product)
		}
		if payload.CustomData["orderId"] != "vclaw_123" {
			t.Fatalf("custom orderId = %v, want vclaw_123", payload.CustomData["orderId"])
		}
		if payload.Checkout == nil {
			t.Fatal("checkout payload is nil, want hosted checkout object")
		}
		if payload.Checkout.URL != nil {
			t.Fatalf("checkout.url = %q, want nil default payment link", *payload.Checkout.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"txn_123","checkout":{"url":"https://buy.paddle.com/checkout/txn_123"}}}`))
	}))
	defer server.Close()

	p, err := NewPaddle("paddle-1", map[string]string{"apiKey": "test_key", "apiBase": server.URL})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "vclaw_123",
		Amount:          "5000",
		PaymentCurrency: "KRW",
		PaymentType:     "paddle",
		Subject:         "V-Claw top up",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if requestedPath != "/transactions" {
		t.Fatalf("requested path = %q, want /transactions", requestedPath)
	}
	if resp.TradeNo != "txn_123" || resp.CheckoutID != "txn_123" {
		t.Fatalf("response = %+v, want txn_123 trade and checkout IDs", resp)
	}
	if resp.CheckoutURL != "https://buy.paddle.com/checkout/txn_123" {
		t.Fatalf("checkout_url = %q, want Paddle hosted URL", resp.CheckoutURL)
	}
	if resp.PayURL != "" {
		t.Fatalf("pay_url = %q, want empty because checkout_url is canonical", resp.PayURL)
	}
}

func TestPaddleCreatePaymentReturnsUpstreamStatusBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"bad_request","detail":"invalid paddle request"}}`))
	}))
	defer server.Close()

	p, err := NewPaddle("paddle-1", map[string]string{"apiKey": "test_key", "apiBase": server.URL})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	_, err = p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "vclaw_123",
		Amount:          "5000",
		PaymentCurrency: "KRW",
		PaymentType:     "paddle",
	})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !strings.Contains(err.Error(), "status=400") || !strings.Contains(err.Error(), "invalid paddle request") {
		t.Fatalf("error = %q, want upstream status and body", err.Error())
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, should not be context canceled", err)
	}
}

func TestPaddleCreatePaymentAllowsMissingHostedCheckoutURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"txn_123"}}`))
	}))
	defer server.Close()

	p, err := NewPaddle("paddle-1", map[string]string{"apiKey": "***", "apiBase": server.URL})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "vclaw_123",
		Amount:          "5000",
		PaymentCurrency: "KRW",
		PaymentType:     "paddle",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if resp.TradeNo != "txn_123" || resp.CheckoutID != "txn_123" {
		t.Fatalf("response = %+v, want txn_123 trade and checkout IDs", resp)
	}
	if resp.CheckoutURL != "" {
		t.Fatalf("checkout_url = %q, want service-layer first-party checkout URL", resp.CheckoutURL)
	}
}

func TestPaddleCreatePaymentDefaultsEmptySubjectToVClawCredit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload paddleTransactionPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("items len = %d, want 1", len(payload.Items))
		}
		item := payload.Items[0]
		if item.Price.Name != "VClaw Credit" || item.Price.Product.Name != "VClaw Credit" {
			t.Fatalf("paddle label = price %q product %q, want VClaw Credit", item.Price.Name, item.Price.Product.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"txn_123","checkout":{"url":"https://buy.paddle.com/checkout/txn_123"}}}`))
	}))
	defer server.Close()

	p, err := NewPaddle("paddle-1", map[string]string{"apiKey": "***", "apiBase": server.URL})
	if err != nil {
		t.Fatalf("NewPaddle() error = %v", err)
	}
	_, err = p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:         "vclaw_123",
		Amount:          "5000",
		PaymentCurrency: "KRW",
		PaymentType:     "paddle",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
}
