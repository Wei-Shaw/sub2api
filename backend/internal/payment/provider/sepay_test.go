package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func sepayTestConfig() map[string]string {
	return map[string]string{
		"apiToken":          "tok_64_chars_00000000000000000000000000000000000000000000000000000000",
		"bankAccountNumber": "0123456789",
		"bankBin":           "970422",
		"webhookSecret":     "secret",
	}
}

func TestNewSePayConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{"missing apiToken", func(c map[string]string) { delete(c, "apiToken") }, "apiToken"},
		{"missing bankAccountNumber", func(c map[string]string) { delete(c, "bankAccountNumber") }, "bankAccountNumber"},
		{"missing bankBin", func(c map[string]string) { delete(c, "bankBin") }, "bankBin"},
		{"no webhook auth", func(c map[string]string) { delete(c, "webhookSecret") }, "webhook"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sepayTestConfig()
			tc.mutate(cfg)
			_, err := NewSePay("1", cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewSePayApiKeyOnlyConfigIsValid(t *testing.T) {
	cfg := sepayTestConfig()
	delete(cfg, "webhookSecret")
	cfg["webhookApiKey"] = "key"
	if _, err := NewSePay("1", cfg); err != nil {
		t.Fatalf("apikey-only config should be valid: %v", err)
	}
}

func TestSePayCreatePayment(t *testing.T) {
	p, err := NewSePay("1", sepayTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_20260814aB3kX9mQ",
		Amount:  "50000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Currency != "VND" {
		t.Fatalf("currency = %q, want VND", resp.Currency)
	}
	if resp.QRCode == "" || !strings.Contains(resp.QRCode, "6304") {
		t.Fatalf("QRCode = %q, want EMV payload", resp.QRCode)
	}
	// QR amount tag must carry the integer VND amount.
	if !strings.Contains(resp.QRCode, tlv("54", "50000")) {
		t.Fatalf("QR payload missing amount TLV: %s", resp.QRCode)
	}
	if !strings.Contains(resp.QRCode, "sub2_20260814aB3kX9mQ") {
		t.Fatalf("QR payload missing transfer content: %s", resp.QRCode)
	}
}

func TestSePayCreatePaymentRejectsNonIntegerAmount(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	if _, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "x", Amount: "50.5"}); err == nil {
		t.Fatal("expected error for fractional VND amount")
	}
	if _, err := p.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "x", Amount: "0"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestSePayRefundUnsupported(t *testing.T) {
	p, _ := NewSePay("1", sepayTestConfig())
	_, err := p.Refund(context.Background(), payment.RefundRequest{})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v, want not supported", err)
	}
}

func TestSepayCodeMatchesOrder(t *testing.T) {
	const out = "sub2_20260814aB3kX9mQ"
	cases := []struct {
		code string
		want bool
	}{
		{"sub2_20260814aB3kX9mQ", true},
		{"SUB2_20260814AB3KX9MQ", true}, // bank uppercased content
		{"20260814aB3kX9mQ", true},      // SePay stripped the prefix
		{"20260814AB3KX9MQ", true},      // stripped + uppercased
		{"sub2_19990101zzzzzzzz", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := sepayCodeMatchesOrder(tc.code, out); got != tc.want {
			t.Errorf("sepayCodeMatchesOrder(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
